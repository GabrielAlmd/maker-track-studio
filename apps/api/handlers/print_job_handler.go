package handlers

import (
	"errors"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type PrintJobInput struct {
	PrinterID        *uuid.UUID `json:"printer_id"`
	ModelID          *uuid.UUID `json:"model_id"`
	PrintDate        time.Time  `json:"print_date" binding:"required"`
	UnitsPrinted     int        `json:"units_printed" binding:"required"`
	LaborHours       *float32   `json:"labor_hours" binding:"required,gte=0"`
	PrintTimeMinutes *int       `json:"print_time_minutes"`
}

// GetPrintJobs godoc
// @Summary      List all print jobs
// @Description  Retrieve all print jobs with their filaments
// @Tags         print_jobs
// @Produce      json
// @Success      200  {array}  dto.PrintJobDTO
// @Router       /print-jobs [get]
func GetPrintJobs(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, valid := parseListParams(c, map[string]string{"date": "print_date", "created_at": "created_at", "units": "units_printed"}, "print_date")
		if !valid {
			c.JSON(400, gin.H{"error": "Invalid list parameters"})
			return
		}
		query := db.Model(&models.PrintJob{})
		if params.Enabled {
			if search := strings.TrimSpace(c.Query("search")); search != "" {
				value := "%" + strings.ToLower(search) + "%"
				matchingModel := db.Model(&models.Model{}).Select("1").Where("models.id = print_jobs.model_id AND LOWER(models.name) LIKE ?", value)
				matchingFilament := db.Table("print_job_filaments").Select("1").Joins("JOIN filaments ON filaments.id = print_job_filaments.filament_id").Where("print_job_filaments.print_job_id = print_jobs.id AND (LOWER(filaments.brand) LIKE ? OR LOWER(filaments.type) LIKE ? OR LOWER(filaments.color) LIKE ?)", value, value, value)
				query = query.Where("EXISTS (?) OR EXISTS (?)", matchingModel, matchingFilament)
			}
			for key, op := range map[string]string{"date_from": ">=", "date_to": "<"} {
				if raw := c.Query(key); raw != "" {
					value, err := time.Parse("2006-01-02", raw)
					if err != nil {
						c.JSON(400, gin.H{"error": "Invalid date filter"})
						return
					}
					if key == "date_to" {
						value = value.Add(24 * time.Hour)
					}
					query = query.Where("print_date "+op+" ?", value)
				}
			}
			for key, column := range map[string]string{"model_id": "model_id", "printer_id": "printer_id", "filament_id": ""} {
				if raw := c.Query(key); raw != "" {
					id, err := uuid.Parse(raw)
					if err != nil {
						c.JSON(400, gin.H{"error": "Invalid reference filter"})
						return
					}
					if column != "" {
						query = query.Where(column+" = ?", id)
					} else {
						query = query.Where("EXISTS (SELECT 1 FROM print_job_filaments WHERE print_job_filaments.print_job_id = print_jobs.id AND print_job_filaments.filament_id = ?)", id)
					}
				}
			}
		}
		var printJobs []models.PrintJob
		var total int64
		if params.Enabled {
			if err := query.Count(&total).Error; err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve print jobs"})
				return
			}
			query = query.Order(params.Sort + " " + params.Direction).Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
		}
		if err := query.Preload("Filaments.Filament").Preload("Model").Find(&printJobs).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print jobs"})
			return
		}

		cfg, err := loadCostConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to load app config"})
			return
		}
		dtoList := make([]dto.PrintJobDTO, 0, len(printJobs))
		for _, pj := range printJobs {
			dtoList = append(dtoList, printJobDTO(pj, cfg))
		}
		if dtoList == nil {
			dtoList = []dto.PrintJobDTO{}
		}
		if params.Enabled {
			c.JSON(200, PageResponse[dto.PrintJobDTO]{Items: dtoList, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, dtoList)
	}
}

// GetPrintJob godoc
// @Summary      Get print job by ID
// @Description  Retrieve a specific print job by ID with its filaments
// @Tags         print_jobs
// @Param        id  path      string  true  "Print Job ID"
// @Produce      json
// @Success      200  {object}  dto.PrintJobDTO
// @Router       /print-jobs/{id} [get]
func GetPrintJob(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var pj models.PrintJob
		if err := db.Preload("Filaments.Filament").Preload("Model").Where("id = ?", id).First(&pj).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job not found"})
			return
		}

		cfg, err := loadCostConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to load app config"})
			return
		}
		c.JSON(200, printJobDTO(pj, cfg))
	}
}

// CreatePrintJob godoc
// @Summary      Create a new print job
// @Description  Create a new print job entry
// @Tags         print_jobs
// @Param        input  body      PrintJobInput  true  "Print Job Input"
// @Produce      json
// @Success      201  {object}  dto.PrintJobDTO
// @Router       /print-jobs [post]
func CreatePrintJob(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input PrintJobInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if !modelIDExists(db, input.ModelID) {
			c.JSON(400, gin.H{"error": "Model not found"})
			return
		}
		printJob := models.PrintJob{
			ModelID:          input.ModelID,
			PrintDate:        input.PrintDate,
			UnitsPrinted:     input.UnitsPrinted,
			LaborHours:       *input.LaborHours,
			PrintTimeMinutes: input.PrintTimeMinutes,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		err := printerTransaction(db, func(tx *gorm.DB) error {
			if err := assignPrintJobPrinter(tx, &printJob, input.PrinterID, true); err != nil {
				return err
			}
			return tx.Create(&printJob).Error
		})
		if err != nil {
			printJobPrinterError(c, err)
			return
		}
		result, err := loadPrintJobDTO(db, printJob.ID)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve created print job"})
			return
		}
		c.JSON(201, result)
	}
}

// UpdatePrintJob godoc
// @Summary      Update an existing print job
// @Description  Update an existing print job entry
// @Tags         print_jobs
// @Param        id    path      string         true  "Print Job ID"
// @Param        input body      PrintJobInput  true  "Print Job Input"
// @Produce      json
// @Success      200  {object}  dto.PrintJobDTO
// @Router       /print-jobs/{id} [put]
func UpdatePrintJob(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var printJob models.PrintJob
		if err := db.Where("id = ?", id).First(&printJob).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job not found"})
			return
		}
		var input PrintJobInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if !modelIDExists(db, input.ModelID) {
			c.JSON(400, gin.H{"error": "Model not found"})
			return
		}
		printJob.ModelID = input.ModelID
		printJob.PrintDate = input.PrintDate
		printJob.UnitsPrinted = input.UnitsPrinted
		printJob.LaborHours = *input.LaborHours
		printJob.PrintTimeMinutes = input.PrintTimeMinutes
		printJob.UpdatedAt = time.Now()

		err := printerTransaction(db, func(tx *gorm.DB) error {
			// Reload inside the write transaction so a concurrent assignment
			// cannot be overwritten by metadata read before acquiring the lock.
			var current models.PrintJob
			if err := tx.Where("id = ?", printJob.ID).First(&current).Error; err != nil {
				return err
			}
			printJob.PrinterID, printJob.PrinterCost = current.PrinterID, current.PrinterCost
			if err := assignPrintJobPrinter(tx, &printJob, input.PrinterID, false); err != nil {
				return err
			}
			return tx.Save(&printJob).Error
		})
		if err != nil {
			printJobPrinterError(c, err)
			return
		}
		result, err := loadPrintJobDTO(db, printJob.ID)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve updated print job"})
			return
		}
		c.JSON(200, result)
	}
}

// DeletePrintJob godoc
// @Summary      Delete print job
// @Description  Delete a print job entry by ID
// @Tags         print_jobs
// @Param        id  path      string  true  "Print Job ID"
// @Produce      json
// @Success      204
// @Router       /print-jobs/{id} [delete]
func DeletePrintJob(db *gorm.DB, installed *modules.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		if err := deletePrintJob(db, installed, id); err != nil {
			if errors.Is(err, modules.ErrPrintJobDeleteConflict) {
				c.JSON(409, gin.H{"error": err.Error()})
				return
			}
			c.JSON(500, gin.H{"error": "Failed to delete print job"})
			return
		}
		c.Status(204) // No content response
	}
}

func deletePrintJob(db *gorm.DB, installed *modules.Registry, id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Take SQLite's writer lock before module reference checks. Allocation
		// writes use the same lock, preventing a delete/allocation race.
		if err := tx.Exec(
			"UPDATE print_jobs SET updated_at = updated_at WHERE id = ?",
			id,
		).Error; err != nil {
			return err
		}

		if err := installed.CheckPrintJobDeletion(tx, id); err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&models.PrintJob{}).Error
	})
}

// GetPrintJobCost godoc
// @Summary      Get cost breakdown for a print job
// @Description  Calculate the cost breakdown (filament, electricity, wear, labor) for a print job
// @Tags         print_jobs
// @Param        id  path      string  true  "Print Job ID"
// @Produce      json
// @Success      200  {object}  dto.PrintJobCostDTO
// @Router       /print-jobs/{id}/cost [get]
func GetPrintJobCost(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}

		var pj models.PrintJob
		if err := db.Preload("Filaments.Filament").Preload("Model").Where("id = ?", id).First(&pj).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job not found"})
			return
		}

		cfg, err := loadCostConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to load app config"})
			return
		}
		costState := calculatePrintJobCostState(pj, cfg)
		if costState.State != "calculated" {
			c.JSON(200, dto.PrintJobCostDTO{PrintJobID: pj.ID, State: costState.State, Reason: costState.Reason})
			return
		}

		cfg = printJobEffectiveConfig(pj, cfg)
		// Defaults if not found
		electricityCostKwh := cfg["electricity_cost_kwh"]
		printerWearCostHour := cfg["printer_wear_cost_hour"]
		laborCostHour := cfg["labor_cost_hour"]
		filamentWasteFactor := cfg["filament_waste_factor"]
		if filamentWasteFactor == 0 {
			filamentWasteFactor = 1.0
		}
		filamentCost, electricityCost, wearCost, laborCost, totalCost := calculatePrintJobCostParts(
			pj,
			electricityCostKwh,
			printerWearCostHour,
			laborCostHour,
			filamentWasteFactor,
		)

		filamentCost, electricityCost, wearCost, laborCost, totalCost = round2(filamentCost), round2(electricityCost), round2(wearCost), round2(laborCost), round2(totalCost)
		c.JSON(200, dto.PrintJobCostDTO{PrintJobID: pj.ID, State: "calculated", FilamentCost: &filamentCost, ElectricityCost: &electricityCost, WearCost: &wearCost, LaborCost: &laborCost, TotalCost: &totalCost})
	}
}

func loadCostConfig(db *gorm.DB) (map[string]float64, error) {
	cfg, err := loadRawCostConfig(db)
	if err != nil {
		return nil, err
	}
	return normalizeCostConfig(cfg), nil
}

// LoadProductionCostConfig exposes the production cost inputs to installed modules.
func LoadProductionCostConfig(db *gorm.DB) (map[string]float64, error) {
	return loadCostConfig(db)
}

func loadRawCostConfig(db *gorm.DB) (map[string]float64, error) {
	var configRows []models.AppConfig
	if err := db.Find(&configRows).Error; err != nil {
		return nil, err
	}
	return models.PrinterCostConfigFromAppConfig(configRows), nil
}

// Work on a copy; neither persisted tariff nor captured inputs are mutated.
func normalizeCostConfig(raw map[string]float64) map[string]float64 {
	cfg := make(map[string]float64, len(raw))
	for key, value := range raw {
		cfg[key] = value
	}
	if cfg["filament_waste_factor"] == 0 {
		cfg["filament_waste_factor"] = 1
	}
	// Normalize to effective hourly rates once for every cost consumer.
	// The persisted kWh tariff and manual wear rate are never changed here.
	if cfg["machine_cost_mode"] == 1 {
		years, uptime := cfg["printer_lifetime_years"], cfg["printer_uptime_percent"]
		if years < 0.001 {
			years = 3
		}
		if uptime < 0.001 || uptime > 100 {
			uptime = 50
		}
		buffer := cfg["printer_buffer_factor"]
		if buffer < 1 {
			buffer = 1.3
		}
		lifetime := cfg["printer_purchase_cost"] + cfg["printer_upfront_cost"] + cfg["printer_annual_maintenance"]*years
		cfg["printer_wear_cost_hour"] = lifetime / (8760 * uptime / 100 * years) * buffer
		cfg["electricity_cost_kwh"] *= cfg["printer_power_watts"] / 1000 * buffer
	}
	return cfg
}

func calculatePrintJobTotalCost(pj models.PrintJob, cfg map[string]float64) float64 {
	_, _, _, _, total := calculatePrintJobCostParts(
		pj,
		cfg["electricity_cost_kwh"],
		cfg["printer_wear_cost_hour"],
		cfg["labor_cost_hour"],
		cfg["filament_waste_factor"],
	)
	rounded := round2(total)
	return rounded
}

func calculatePrintJobCostState(pj models.PrintJob, cfg map[string]float64) dto.PrintJobCostStateDTO {
	// A nil snapshot identifies a legacy job. Those rows remain calculable
	// from AppConfig; modern jobs must keep using their captured snapshot.
	if pj.PrinterCost != nil && models.ValidatePrinterCostSnapshot(pj.PrinterCost) != nil {
		return dto.PrintJobCostStateDTO{State: "not_calculated", Reason: "Printer cost snapshot is unavailable or invalid"}
	}
	if pj.PrintTimeMinutes == nil && (pj.Model == nil || pj.Model.ID == uuid.Nil || pj.Model.PrintTime <= 0) {
		return dto.PrintJobCostStateDTO{State: "not_calculated", Reason: "Print time is not available"}
	}
	for _, usage := range pj.Filaments {
		if usage.Filament.ID == uuid.Nil {
			return dto.PrintJobCostStateDTO{State: "not_calculated", Reason: "A referenced filament is unavailable"}
		}
	}
	total := calculatePrintJobTotalCost(pj, cfg)
	return dto.PrintJobCostStateDTO{State: "calculated", Total: &total}
}

// CalculateProductionPrintJobCostState uses the same cost rules as the core API.
func CalculateProductionPrintJobCostState(pj models.PrintJob, cfg map[string]float64) dto.PrintJobCostStateDTO {
	return calculatePrintJobCostState(pj, cfg)
}

func modelIDExists(db *gorm.DB, id *uuid.UUID) bool {
	if id == nil {
		return true
	}
	var count int64
	return db.Model(&models.Model{}).Where("id = ?", *id).Count(&count).Error == nil && count == 1
}

func calculatePrintJobCostParts(
	pj models.PrintJob,
	electricityCostKwh float64,
	printerWearCostHour float64,
	laborCostHour float64,
	filamentWasteFactor float64,
) (float64, float64, float64, float64, float64) {
	if pj.PrinterCost != nil {
		cfg := normalizeCostConfig(pj.PrinterCost.Config)
		electricityCostKwh = cfg["electricity_cost_kwh"]
		printerWearCostHour = cfg["printer_wear_cost_hour"]
		laborCostHour = cfg["labor_cost_hour"]
		filamentWasteFactor = cfg["filament_waste_factor"]
	}
	var filamentCost float64
	for _, pjf := range pj.Filaments {
		filamentCost += (pjf.WeightUsed / 1000.0) * pjf.Filament.AvgPrice * filamentWasteFactor
	}

	var printTimeMinutes int
	if pj.PrintTimeMinutes != nil {
		printTimeMinutes = *pj.PrintTimeMinutes
	} else if pj.Model != nil {
		printTimeMinutes = pj.Model.PrintTime
	}
	printTimeHours := float64(printTimeMinutes) / 60.0

	electricityCost := printTimeHours * electricityCostKwh
	wearCost := printTimeHours * printerWearCostHour
	laborCost := float64(pj.LaborHours) * laborCostHour
	totalCost := filamentCost + electricityCost + wearCost + laborCost

	return filamentCost, electricityCost, wearCost, laborCost, totalCost
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// GetPrintJobStats godoc
// @Summary      Get aggregate stats for all print jobs
// @Description  Returns totals for jobs, units, labor, filament weight, and cost breakdown across all print jobs
// @Tags         print_jobs
// @Produce      json
// @Success      200  {object}  dto.PrintJobStatsDTO
// @Router       /print-jobs/stats [get]
func GetPrintJobStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var printJobs []models.PrintJob
		if err := db.Preload("Filaments.Filament").Preload("Model").Find(&printJobs).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print jobs"})
			return
		}

		cfg, err := loadCostConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to load app config"})
			return
		}

		stats := buildPrintJobStats(printJobs, cfg)
		c.JSON(200, stats)
	}
}

// GetDashboardSummary godoc
// @Summary      Get dashboard summary
// @Description  Returns dashboard totals, cost stats, and recent print jobs precomputed by the API
// @Tags         print_jobs
// @Produce      json
// @Success      200  {object}  dto.DashboardSummaryDTO
// @Router       /print-jobs/dashboard-summary [get]
func GetDashboardSummary(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var totalModels int64
		var totalFilaments int64
		var totalPrintJobs int64
		var totalPurchases int64

		if err := db.Model(&models.Model{}).Count(&totalModels).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve dashboard summary"})
			return
		}
		if err := db.Model(&models.Filament{}).Count(&totalFilaments).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve dashboard summary"})
			return
		}
		if err := db.Model(&models.PrintJob{}).Count(&totalPrintJobs).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve dashboard summary"})
			return
		}
		if err := db.Model(&models.FilamentPurchase{}).Count(&totalPurchases).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve dashboard summary"})
			return
		}

		cfg, err := loadCostConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to load app config"})
			return
		}

		var allPrintJobs []models.PrintJob
		if err := db.Preload("Filaments.Filament").Preload("Model").Find(&allPrintJobs).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print jobs"})
			return
		}

		costStats := buildPrintJobStats(allPrintJobs, cfg)
		expenses, err := buildDashboardExpenses(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve dashboard expenses"})
			return
		}

		var recentRows []models.PrintJob
		if err := db.Preload("Filaments.Filament").Preload("Model").Order("print_date DESC").Limit(5).Find(&recentRows).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve recent print jobs"})
			return
		}

		recent := make([]dto.DashboardRecentPrintJobDTO, 0, len(recentRows))
		for _, pj := range recentRows {
			model := modelReference(pj)
			cost := calculatePrintJobCostState(pj, cfg)

			recent = append(recent, dto.DashboardRecentPrintJobDTO{
				ID:            pj.ID,
				PrinterID:     pj.PrinterID,
				PrinterName:   printJobPrinterName(pj),
				ModelID:       modelReferenceID(pj),
				ModelName:     model.Name,
				PrintDate:     pj.PrintDate,
				UnitsPrinted:  pj.UnitsPrinted,
				LaborHours:    pj.LaborHours,
				EstimatedCost: cost.Total,
				Model:         model,
				Cost:          cost,
			})
		}

		c.JSON(200, dto.DashboardSummaryDTO{
			TotalModels:     int(totalModels),
			TotalFilaments:  int(totalFilaments),
			TotalPrintJobs:  int(totalPrintJobs),
			TotalPurchases:  int(totalPurchases),
			Expenses:        expenses,
			CostStats:       costStats,
			RecentPrintJobs: recent,
		})
	}
}

// buildDashboardExpenses uses transaction sources only: physical-printer
// acquisition values, filament purchase totals, and explicitly categorized
// equipment expense records. It never adds printer_upfront_cost or Print Job
// costs, because those are calculation inputs that would double-count spend.
func buildDashboardExpenses(db *gorm.DB) (dto.DashboardExpensesDTO, error) {
	var printers []models.Printer
	if err := db.Find(&printers).Error; err != nil {
		return dto.DashboardExpensesDTO{}, err
	}
	var result dto.DashboardExpensesDTO
	for _, printer := range printers {
		result.PrinterCost += printer.CostConfig["printer_purchase_cost"]
	}
	if err := db.Model(&models.FilamentPurchase{}).Select("COALESCE(SUM(price), 0)").Scan(&result.FilamentCost).Error; err != nil {
		return dto.DashboardExpensesDTO{}, err
	}

	var configs []models.AppConfig
	if err := db.Find(&configs).Error; err != nil {
		return dto.DashboardExpensesDTO{}, err
	}
	var expenseConfig *models.AppConfig
	for i := range configs {
		if configs[i].Key == equipmentCostsKey {
			expenseConfig = &configs[i]
			break
		}
	}
	if expenseConfig == nil {
		// Virtual cost defaults are calculation examples, not recorded spend.
		// Only migrate an explicitly persisted legacy upfront-cost row.
		for _, config := range configs {
			if config.Key == "printer_upfront_cost" {
				legacy := legacyEquipmentConfig(configs)
				expenseConfig = &legacy
				break
			}
		}
		if expenseConfig == nil {
			empty := models.AppConfig{Value: "[]", Type: "json"}
			expenseConfig = &empty
		}
	}
	entries, _, err := parseEquipment(AppConfigInput{Value: expenseConfig.Value, Type: expenseConfig.Type})
	if err != nil {
		return dto.DashboardExpensesDTO{}, err
	}
	for _, entry := range entries {
		switch entry.Category {
		case equipmentCategorySpareParts:
			result.SparePartsCost += entry.Amount
		case equipmentCategoryAdditional:
			result.AdditionalCost += entry.Amount
		case equipmentCategoryOther:
			result.OtherCost += entry.Amount
		}
	}
	result.TotalExpenses = result.PrinterCost + result.FilamentCost + result.SparePartsCost + result.AdditionalCost + result.OtherCost
	return result, nil
}

func printJobPrinterName(job models.PrintJob) string {
	if job.PrinterCost == nil {
		return "Not assigned"
	}
	name := strings.TrimSpace(job.PrinterCost.PrinterName)
	if name == "" {
		return "Printer unavailable"
	}
	return name
}
func buildPrintJobStats(printJobs []models.PrintJob, cfg map[string]float64) dto.PrintJobStatsDTO {
	electricityCostKwh := cfg["electricity_cost_kwh"]
	printerWearCostHour := cfg["printer_wear_cost_hour"]
	laborCostHour := cfg["labor_cost_hour"]
	filamentWasteFactor := cfg["filament_waste_factor"]
	if filamentWasteFactor == 0 {
		filamentWasteFactor = 1.0
	}

	var stats dto.PrintJobStatsDTO
	stats.TotalJobs = len(printJobs)

	for _, pj := range printJobs {
		stats.TotalUnitsPrinted += pj.UnitsPrinted
		stats.TotalLaborHours += float64(pj.LaborHours)

		costState := calculatePrintJobCostState(pj, cfg)
		if costState.State != "calculated" {
			continue
		}
		stats.CalculatedJobs++
		filamentCost, electricityCost, wearCost, laborCost, _ := calculatePrintJobCostParts(
			pj,
			electricityCostKwh,
			printerWearCostHour,
			laborCostHour,
			filamentWasteFactor,
		)

		for _, pjf := range pj.Filaments {
			stats.TotalFilamentWeightG += pjf.WeightUsed
		}

		stats.TotalFilamentCost += filamentCost
		stats.TotalElectricityCost += electricityCost
		stats.TotalWearCost += wearCost
		stats.TotalLaborCost += laborCost
	}

	stats.TotalCost = stats.TotalFilamentCost + stats.TotalElectricityCost + stats.TotalWearCost + stats.TotalLaborCost

	stats.TotalLaborHours = round2(stats.TotalLaborHours)
	stats.TotalFilamentWeightG = round2(stats.TotalFilamentWeightG)
	stats.TotalFilamentCost = round2(stats.TotalFilamentCost)
	stats.TotalElectricityCost = round2(stats.TotalElectricityCost)
	stats.TotalWearCost = round2(stats.TotalWearCost)
	stats.TotalLaborCost = round2(stats.TotalLaborCost)
	stats.TotalCost = round2(stats.TotalCost)

	return stats
}
