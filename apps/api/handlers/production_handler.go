package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"reflect"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errProductionConflict = errors.New("production revision, identity or lifecycle conflict")

type ProductionRunInput struct {
	ID               *uuid.UUID              `json:"id"`
	Status           string                  `json:"status"`
	PrintTimeMinutes *int                    `json:"print_time_minutes"`
	LaborHours       float32                 `json:"labor_hours"`
	Items            []models.PrintRunItem   `json:"items"`
	Materials        []PrintJobMaterialInput `json:"materials"`
}

type ProductionInput struct {
	OperationID      uuid.UUID            `json:"operation_id"`
	ExpectedRevision int64                `json:"expected_revision"`
	PrinterID        *uuid.UUID           `json:"printer_id"`
	PrintDate        time.Time            `json:"print_date"`
	Runs             []ProductionRunInput `json:"runs"`
}

type ProductionRunResponse struct {
	models.PrintRun
	Materials []PrintJobMaterialInput `json:"materials"`
}

type ProductionResponse struct {
	ID          uuid.UUID               `json:"id"`
	Revision    int64                   `json:"revision"`
	OperationID uuid.UUID               `json:"operation_id,omitempty"`
	Runs        []ProductionRunResponse `json:"runs"`
}

func loadProduction(db *gorm.DB, id uuid.UUID) (ProductionResponse, error) {
	var job models.PrintJob
	if err := db.First(&job, "id = ?", id).Error; err != nil {
		return ProductionResponse{}, err
	}
	var runs []models.PrintRun
	if err := db.Preload("Items").Preload("Materials").Where("print_job_id = ?", id).Order("id").Find(&runs).Error; err != nil {
		return ProductionResponse{}, err
	}
	result := ProductionResponse{ID: id, Revision: job.Revision, Runs: []ProductionRunResponse{}}
	for _, run := range runs {
		row := ProductionRunResponse{PrintRun: run, Materials: []PrintJobMaterialInput{}}
		for _, m := range run.Materials {
			id := m.ID
			row.Materials = append(row.Materials, PrintJobMaterialInput{ID: &id, FilamentID: m.FilamentID, WeightUsed: m.WeightUsed, EstimatedWeightG: m.EstimatedWeightG, WasteWeightG: m.WasteWeightG, MeasurementState: m.MeasurementState})
		}
		result.Runs = append(result.Runs, row)
	}
	return result, nil
}

func productionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, modules.ErrRunItemPlanConflict):
		c.JSON(409, gin.H{"error": modules.ErrRunItemPlanConflict.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(404, gin.H{"error": "Production reference not found"})
	case errors.Is(err, inventory.ErrConflict), errors.Is(err, errProductionConflict), errors.Is(err, errPrintJobMaterialConflict):
		c.JSON(409, gin.H{"error": errProductionConflict.Error()})
	default:
		c.JSON(500, gin.H{"error": "Unable to persist production"})
	}
}

func GetProduction(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			c.JSON(400, gin.H{"error": "Invalid job ID"})
			return
		}
		result, err := loadProduction(db, id)
		if err != nil {
			productionError(c, err)
			return
		}
		c.JSON(200, result)
	}
}

// WriteProduction persists the complete aggregate and replay result under the
// same SQLite writer lock. Terminal runs are immutable; a reprint is a new run.
func WriteProduction(db *gorm.DB, installed *modules.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id uuid.UUID
		creating := c.Param("id") == ""
		if !creating {
			var err error
			id, err = uuid.Parse(c.Param("id"))
			if err != nil || id == uuid.Nil {
				c.JSON(400, gin.H{"error": "Invalid job ID"})
				return
			}
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		var input ProductionInput
		if err := c.ShouldBindJSON(&input); err != nil || input.OperationID == uuid.Nil || input.ExpectedRevision < 0 || input.PrintDate.IsZero() || len(input.Runs) == 0 || len(input.Runs) > 100 {
			c.JSON(400, gin.H{"error": "Invalid production command"})
			return
		}
		for _, run := range input.Runs {
			candidate := models.PrintRun{Status: run.Status, Provenance: "verified", Items: run.Items}
			if run.Status == "planned" && (run.LaborHours != 0 || run.PrintTimeMinutes != nil) {
				c.JSON(400, gin.H{"error": "Planned runs cannot contain actual time or labor"})
				return
			}
			if models.ValidatePrintRun(candidate) != nil || run.LaborHours < 0 || run.LaborHours > 1000000 || math.IsNaN(float64(run.LaborHours)) || math.IsInf(float64(run.LaborHours), 0) || run.PrintTimeMinutes != nil && (*run.PrintTimeMinutes < 0 || *run.PrintTimeMinutes > 1000000000) {
				c.JSON(400, gin.H{"error": "Invalid run quantities, status or duration"})
				return
			}
			if err := validatePrintJobMaterials(db, &run.Materials); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			for _, m := range run.Materials {
				if _, err := inventory.Milligrams(m.WeightUsed, 1000); err != nil {
					c.JSON(400, gin.H{"error": "Material grams exceed inventory range"})
					return
				}
				if m.MeasurementState != "verified" && m.MeasurementState != "estimated" || run.Status == "planned" && m.MeasurementState != "estimated" {
					c.JSON(400, gin.H{"error": "Planned runs require estimates; executed consumption must have explicit provenance"})
					return
				}
			}
		}
		encoded, _ := json.Marshal(struct {
			ID    uuid.UUID
			Input ProductionInput
		}{id, input})
		sum := sha256.Sum256(encoded)
		hash := hex.EncodeToString(sum[:])
		var response string
		err := printerTransaction(db, func(tx *gorm.DB) error {
			var previous models.ProductionOperation
			err := tx.First(&previous, "id = ?", input.OperationID).Error
			if err == nil {
				if previous.PayloadHash != hash {
					return errProductionConflict
				}
				response = previous.Response
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var job models.PrintJob
			if creating {
				if input.ExpectedRevision != 0 {
					return errProductionConflict
				}
				job = models.PrintJob{WorkflowVersion: 2, PrintDate: input.PrintDate}
			} else {
				if err := tx.First(&job, "id = ?", id).Error; err != nil {
					return err
				}
				if job.WorkflowVersion != 2 || job.Revision != input.ExpectedRevision {
					return errProductionConflict
				}
			}
			if !creating && !reflect.DeepEqual(job.PrinterID, input.PrinterID) {
				return errProductionConflict
			}
			if err := assignPrintJobPrinter(tx, &job, input.PrinterID, creating); err != nil {
				return err
			}
			if creating {
				if err := tx.Create(&job).Error; err != nil {
					return err
				}
			}
			var oldRuns []models.PrintRun
			if err := tx.Preload("Items").Preload("Materials").Where("print_job_id = ?", job.ID).Find(&oldRuns).Error; err != nil {
				return err
			}
			old := map[uuid.UUID]models.PrintRun{}
			for _, run := range oldRuns {
				old[run.ID] = run
			}
			seen := map[uuid.UUID]bool{}
			job.UnitsPrinted = 0
			job.LaborHours = 0
			totalTime := 0
			unknownTime := false
			job.PrintTimeMinutes = &totalTime
			job.ModelID = nil
			job.PrintDate = input.PrintDate
			for _, command := range input.Runs {
				run := models.PrintRun{PrintJobID: job.ID, Status: command.Status, Provenance: "verified", PrintTimeMinutes: command.PrintTimeMinutes, LaborHours: command.LaborHours, Items: command.Items, Revision: 1}
				var prior models.PrintRun
				if command.ID != nil {
					var ok bool
					prior, ok = old[*command.ID]
					if !ok || seen[*command.ID] || !models.ValidPrintRunTransition(prior.Status, run.Status) {
						return errProductionConflict
					}
					run.ID = *command.ID
					run.Revision = prior.Revision + 1
				} else {
					if command.Status != "planned" {
						return errProductionConflict
					}
					if err := run.BeforeCreate(tx); err != nil {
						return err
					}
				}
				seen[run.ID] = true
				priorItems := map[uuid.UUID]models.PrintRunItem{}
				for _, item := range prior.Items {
					priorItems[item.ID] = item
				}
				itemSeen := map[uuid.UUID]bool{}
				for n := range run.Items {
					item := &run.Items[n]
					if item.ID == uuid.Nil {
						if prior.ID != uuid.Nil && prior.Status != "planned" {
							return errProductionConflict
						}
						if err := item.BeforeCreate(tx); err != nil {
							return err
						}
					} else {
						if _, ok := priorItems[item.ID]; !ok || itemSeen[item.ID] {
							return errProductionConflict
						}
					}
					itemSeen[item.ID] = true
					item.PrintRunID = run.ID
					if previous, ok := priorItems[item.ID]; ok && prior.Status == "planned" &&
						(item.Planned < previous.Planned || !reflect.DeepEqual(item.ModelID, previous.ModelID)) {
						if err := installed.CheckRunItemPlanChange(tx, previous, *item); err != nil {
							return err
						}
					}
					if previous, ok := priorItems[item.ID]; ok && prior.Status != "planned" {
						item.ModelName = previous.ModelName
					} else if item.ModelID != nil {
						var model models.Model
						if err := tx.First(&model, "id = ?", item.ModelID).Error; err != nil {
							return err
						}
						item.ModelName = model.Name
					}
					if run.Status == "completed" {
						job.UnitsPrinted += int(item.Accepted)
					}
				}
				for key, previous := range priorItems {
					if !itemSeen[key] {
						return errProductionConflict
					}
					if prior.Status != "planned" {
						for _, item := range run.Items {
							if item.ID == key && (item.Planned != previous.Planned || !reflect.DeepEqual(item.ModelID, previous.ModelID)) {
								return errProductionConflict
							}
						}
					}
				}
				if prior.Status == "completed" || prior.Status == "failed" || prior.Status == "cancelled" {
					run.Revision = prior.Revision
					if prior.Status != run.Status || prior.LaborHours != run.LaborHours || !reflect.DeepEqual(prior.PrintTimeMinutes, run.PrintTimeMinutes) || !reflect.DeepEqual(prior.Items, run.Items) {
						return errProductionConflict
					}
				}
				if run.PrintTimeMinutes == nil {
					unknownTime = true
				}
				job.LaborHours += run.LaborHours
				if run.PrintTimeMinutes != nil {
					totalTime += *run.PrintTimeMinutes
				}
				if err := tx.Omit("Items", "Materials").Save(&run).Error; err != nil {
					return err
				}
				for _, item := range run.Items {
					if err := tx.Omit("Model").Save(&item).Error; err != nil {
						return err
					}
				}
				if run.Status == "completed" && prior.Status != "completed" {
					for _, item := range run.Items {
						if item.ModelID != nil && item.Accepted > 0 {
							if err := inventory.RecordOutput(tx, item); err != nil {
								return err
							}
						}
					}
				}
				if prior.ID != uuid.Nil && prior.Status != run.Status {
					if err := installed.RecordRunTransition(tx, run); err != nil {
						return err
					}
				}
				if err := saveRunMaterials(tx, job.ID, run, prior, command.Materials); err != nil {
					return err
				}
			}
			if len(old) > 0 { // New runs may be appended, existing executions never disappear.
				for key := range old {
					if !seen[key] {
						return errProductionConflict
					}
				}
			}
			if unknownTime {
				job.PrintTimeMinutes = nil
			}
			job.Revision++
			if err := tx.Omit("Model", "Filaments").Save(&job).Error; err != nil {
				return err
			}
			result, err := loadProduction(tx, job.ID)
			if err != nil {
				return err
			}
			result.OperationID = input.OperationID
			body, err := json.Marshal(result)
			if err != nil {
				return err
			}
			response = string(body)
			return tx.Create(&models.ProductionOperation{ID: input.OperationID, PrintJobID: job.ID, PayloadHash: hash, Response: response}).Error
		})
		if err != nil {
			productionError(c, err)
			return
		}
		status := 200
		if creating {
			status = 201
		}
		c.Data(status, "application/json", []byte(response))
	}
}

func saveRunMaterials(tx *gorm.DB, jobID uuid.UUID, run, prior models.PrintRun, rows []PrintJobMaterialInput) error {
	if err := validatePrintJobMaterials(tx, &rows); err != nil {
		return err
	}
	old := map[uuid.UUID]models.PrintJobFilament{}
	for _, m := range prior.Materials {
		old[m.ID] = m
	}
	seen := map[uuid.UUID]bool{}
	for _, row := range rows {
		m := models.PrintJobFilament{PrintJobID: jobID, PrintRunID: &run.ID, FilamentID: row.FilamentID, WeightUsed: row.WeightUsed, EstimatedWeightG: row.EstimatedWeightG, WasteWeightG: row.WasteWeightG, MeasurementState: row.MeasurementState}
		if row.ID != nil {
			previous, ok := old[*row.ID]
			if !ok || seen[*row.ID] {
				return errProductionConflict
			}
			m.ID = *row.ID
			m.CreatedAt = previous.CreatedAt
			if prior.Status != "planned" && previous.MeasurementState == "verified" && (m.MeasurementState != "verified" || m.FilamentID != previous.FilamentID || m.WeightUsed < previous.WeightUsed) {
				return errProductionConflict
			}
			if prior.Status == "completed" || prior.Status == "failed" || prior.Status == "cancelled" {
				if m.FilamentID != previous.FilamentID || m.WeightUsed != previous.WeightUsed || m.MeasurementState != previous.MeasurementState || !reflect.DeepEqual(m.EstimatedWeightG, previous.EstimatedWeightG) || !reflect.DeepEqual(m.WasteWeightG, previous.WasteWeightG) {
					return errProductionConflict
				}
			}
		} else {
			if prior.Status == "completed" || prior.Status == "failed" || prior.Status == "cancelled" {
				return errProductionConflict
			}
			if err := m.BeforeCreate(tx); err != nil {
				return err
			}
		}
		seen[m.ID] = true
		if prior.Status == "completed" || prior.Status == "failed" || prior.Status == "cancelled" {
			continue
		}
		if (run.Status == "completed" || run.Status == "failed") && m.MeasurementState == "verified" {
			quantity, err := inventory.Milligrams(m.WeightUsed, 1000)
			if err != nil {
				return err
			}
			if err := inventory.SetSource(tx, "material:"+m.ID.String(), map[uuid.UUID]int64{m.FilamentID: -quantity}, "consumption"); err != nil {
				return err
			}
		}
		if err := tx.Save(&m).Error; err != nil {
			return err
		}
	}
	for key := range old {
		if !seen[key] {
			return errProductionConflict
		}
	}
	return nil
}
