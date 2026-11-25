package handlers

import (
	"errors"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type ModelInput struct {
	Name                 string                `json:"name" binding:"required"`
	ImageURL             string                `json:"image_url"`
	MaterialWeight       *float64              `json:"material_weight"`
	PrintTime            int                   `json:"print_time" binding:"gt=0"`
	WasteMaterial        float64               `json:"waste_material"`
	IsMulticolor         bool                  `json:"is_multicolor"`
	MaterialRequirements *[]modelMaterialInput `json:"material_requirements"`
}

type modelMaterialInput struct {
	ID                  *uuid.UUID `json:"id"`
	MaterialType        string     `json:"material_type"`
	Color               string     `json:"color"`
	PartWeightG         *float64   `json:"part_weight_g"`
	WasteWeightG        *float64   `json:"waste_weight_g"`
	PreferredFilamentID *uuid.UUID `json:"preferred_filament_id"`
	SourceTool          *int       `json:"source_tool"`
}

// GetModels godoc
// @Summary      List all models
// @Description  Retrieve all models with their print jobs
// @Tags         models
// @Produce      json
// @Success      200  {array}  dto.ModelDTO
// @Router       /models [get]
func GetModels(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, valid := parseListParams(c, map[string]string{"name": "name", "print_time": "print_time", "created_at": "created_at"}, "name")
		if !valid {
			c.JSON(400, gin.H{"error": "Invalid list parameters"})
			return
		}
		query := db.WithContext(c.Request.Context()).Model(&models.Model{})
		if params.Enabled {
			if search := strings.TrimSpace(c.Query("search")); search != "" {
				query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(search)+"%")
			}
			if multi := c.Query("multicolor"); multi != "" {
				if multi != "true" && multi != "false" {
					c.JSON(400, gin.H{"error": "Invalid multicolor filter"})
					return
				}
				query = query.Where("is_multicolor = ?", multi == "true")
			}
			if jobs := c.Query("print_jobs"); jobs != "" {
				sub := db.Model(&models.PrintJob{}).Select("1").Where("print_jobs.model_id = models.id")
				if jobs == "has" {
					query = query.Where("EXISTS (?)", sub)
				} else if jobs == "none" {
					query = query.Where("NOT EXISTS (?)", sub)
				} else {
					c.JSON(400, gin.H{"error": "Invalid print_jobs filter"})
					return
				}
			}
			for key, filter := range map[string]struct{ column, op string }{
				"min_print_time": {"print_time", ">="}, "max_print_time": {"print_time", "<="},
				"min_material": {"material_weight", ">="}, "max_material": {"material_weight", "<="},
				"min_waste": {"waste_material", ">="}, "max_waste": {"waste_material", "<="},
			} {
				if raw := c.Query(key); raw != "" {
					value, err := strconv.ParseFloat(raw, 64)
					if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (filter.column == "print_time" && value != float64(int(value))) {
						c.JSON(400, gin.H{"error": "Invalid numeric filter"})
						return
					}
					query = query.Where(filter.column+" "+filter.op+" ?", value)
				}
			}
		}
		var modelsList []models.Model
		var total int64
		if params.Enabled {
			if err := query.Count(&total).Error; err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve models"})
				return
			}
			query = query.Order(params.Sort + " " + params.Direction).Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
		}
		if err := withModelMaterials(query).Preload("PrintJobs").Find(&modelsList).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve models"})
			return
		}

		dtoList := make([]dto.ModelDTO, 0, len(modelsList))
		for _, m := range modelsList {
			dtoList = append(dtoList, toModelDTO(m))
		}

		if params.Enabled {
			c.JSON(200, PageResponse[dto.ModelDTO]{Items: dtoList, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, dtoList)
	}
}

func GetModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var model models.Model
		err = withModelMaterials(db.WithContext(c.Request.Context())).Preload("PrintJobs").First(&model, "id = ?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"error": "Model not found"})
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve model"})
			return
		}
		c.JSON(200, toModelDTO(model))
	}
}

func CreateModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		saveModel(c, db, nil)
	}
}

func UpdateModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		saveModel(c, db, &id)
	}
}

func saveModel(c *gin.Context, db *gorm.DB, id *uuid.UUID) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var input ModelInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Invalid model input"})
		return
	}
	var model models.Model
	err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if id != nil {
			if err := withModelMaterials(tx).First(&model, "id = ?", *id).Error; err != nil {
				return err
			}
		}
		previous := model.MaterialRequirements
		if input.MaterialRequirements != nil {
			rows, err := prepareModelMaterials(tx, model, *input.MaterialRequirements)
			if err != nil {
				return err
			}
			model.MaterialRequirements = rows
		}
		summary, err := models.SummarizeModelMaterials(model.MaterialRequirements)
		if err != nil {
			return err
		}
		model.Name, model.ImageURL, model.PrintTime = input.Name, input.ImageURL, input.PrintTime
		if len(model.MaterialRequirements) == 0 {
			if input.MaterialWeight == nil || !models.ValidModelMaterialGrams(*input.MaterialWeight) ||
				!models.ValidModelMaterialGrams(input.WasteMaterial) {
				return models.ErrInvalidModelMaterials
			}
			model.MaterialWeight, model.WasteMaterial, model.IsMulticolor = *input.MaterialWeight, input.WasteMaterial, input.IsMulticolor
		} else {
			model.MaterialWeight, model.WasteMaterial = summary.PartWeightG, summary.KnownWasteG
			model.IsMulticolor = summary.ColorStatus == "multiple"
		}

		record := tx.Omit("MaterialRequirements")
		if id == nil {
			if err := record.Create(&model).Error; err != nil {
				return err
			}
		} else if err := record.Select("Name", "ImageURL", "PrintTime", "MaterialWeight", "WasteMaterial", "IsMulticolor", "UpdatedAt").Updates(&model).Error; err != nil {
			return err
		}
		if input.MaterialRequirements != nil {
			return syncModelMaterials(tx, &model, previous)
		}
		return nil
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(404, gin.H{"error": "Model not found"})
	case errors.Is(err, models.ErrInvalidModelMaterials):
		c.JSON(400, gin.H{"error": models.ErrInvalidModelMaterials.Error()})
	case err != nil:
		c.JSON(500, gin.H{"error": "Failed to save model"})
	case id == nil:
		c.JSON(201, toModelDTO(model))
	default:
		c.JSON(200, toModelDTO(model))
	}
}

func prepareModelMaterials(tx *gorm.DB, model models.Model, inputs []modelMaterialInput) ([]models.ModelMaterialRequirement, error) {
	if len(inputs) > models.MaxModelMaterialRequirements {
		return nil, models.ErrInvalidModelMaterials
	}
	existing := make(map[uuid.UUID]bool, len(model.MaterialRequirements))
	for _, row := range model.MaterialRequirements {
		existing[row.ID] = true
	}
	preferences := make(map[uuid.UUID]bool)
	rows := make([]models.ModelMaterialRequirement, len(inputs))
	for i, input := range inputs {
		if input.PartWeightG == nil {
			return nil, models.ErrInvalidModelMaterials
		}
		row := models.ModelMaterialRequirement{
			Position: i, MaterialType: strings.TrimSpace(input.MaterialType), Color: strings.TrimSpace(input.Color),
			PartWeightG: *input.PartWeightG, WasteWeightG: input.WasteWeightG,
			PreferredFilamentID: input.PreferredFilamentID, SourceTool: input.SourceTool,
		}
		if input.ID != nil {
			if !existing[*input.ID] {
				return nil, models.ErrInvalidModelMaterials
			}
			row.ID = *input.ID
			delete(existing, row.ID)
		}
		if input.PreferredFilamentID != nil {
			preferences[*input.PreferredFilamentID] = true
		}
		rows[i] = row
	}
	// A retry of the same PUT may still omit a newly issued row ID.
	for i, previous := range model.MaterialRequirements {
		if i >= len(rows) || rows[i].ID != uuid.Nil || !existing[previous.ID] {
			continue
		}
		candidate := rows[i]
		candidate.ID, candidate.ModelID = previous.ID, model.ID
		if reflect.DeepEqual(candidate, previous) {
			rows[i] = candidate
		}
	}
	if len(preferences) > 0 {
		ids := make([]uuid.UUID, 0, len(preferences))
		for id := range preferences {
			ids = append(ids, id)
		}
		var count int64
		if err := tx.Model(&models.Filament{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return nil, err
		}
		if count != int64(len(ids)) {
			return nil, models.ErrInvalidModelMaterials
		}
	}
	return rows, nil
}

func syncModelMaterials(tx *gorm.DB, model *models.Model, previous []models.ModelMaterialRequirement) error {
	kept := make(map[uuid.UUID]bool, len(previous))
	for i := range model.MaterialRequirements {
		row := &model.MaterialRequirements[i]
		row.ModelID = model.ID
		if row.ID == uuid.Nil {
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		} else if err := tx.Where("model_id = ?", model.ID).
			Select("Position", "MaterialType", "Color", "PartWeightG", "WasteWeightG", "PreferredFilamentID", "SourceTool").
			Updates(row).Error; err != nil {
			return err
		}
		kept[row.ID] = true
	}
	for _, row := range previous {
		if !kept[row.ID] {
			if err := tx.Where("model_id = ? AND id = ?", model.ID, row.ID).Delete(&models.ModelMaterialRequirement{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func withModelMaterials(db *gorm.DB) *gorm.DB {
	return db.Preload("MaterialRequirements", func(q *gorm.DB) *gorm.DB { return q.Order("position") })
}

func toModelDTO(model models.Model) dto.ModelDTO {
	var jobs []dto.PrintJobDTO
	for _, job := range model.PrintJobs {
		jobs = append(jobs, dto.PrintJobDTO{
			ID: job.ID, ModelID: &model.ID, PrintDate: job.PrintDate, UnitsPrinted: job.UnitsPrinted,
			LaborHours: job.LaborHours, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		})
	}
	rows := model.MaterialRequirements
	if rows == nil {
		rows = []models.ModelMaterialRequirement{}
	}
	summary, _ := models.SummarizeModelMaterials(rows)
	return dto.ModelDTO{
		ID: model.ID, Name: model.Name, ImageURL: model.ImageURL, PrintTime: model.PrintTime,
		MaterialWeight: model.MaterialWeight, WasteMaterial: model.WasteMaterial, IsMulticolor: model.IsMulticolor,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, PrintJobs: jobs, PrintJobCount: len(jobs),
		MaterialRequirements: rows, MaterialSummary: summary,
	}
}

func DeleteModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		deleteInventoryItem(c, db, "model", &models.Model{})
	}
}
