package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type ModelInput struct {
	Name           string  `json:"name" binding:"required"`
	ImageURL       string  `json:"image_url"`
	MaterialWeight float64 `json:"material_weight" binding:"required"`
	PrintTime      int     `json:"print_time" binding:"required"`
	WasteMaterial  float64 `json:"waste_material" binding:"gte=0"`
	IsMulticolor   bool    `json:"is_multicolor"`
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
		query := db.Model(&models.Model{})
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
					if err != nil || value < 0 || (filter.column == "print_time" && value != float64(int(value))) {
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
		if err := query.Preload("PrintJobs").Find(&modelsList).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve models"})
			return
		}

		dtoList := make([]dto.ModelDTO, 0, len(modelsList))
		for _, m := range modelsList {
			var jobs []dto.PrintJobDTO
			for _, pj := range m.PrintJobs {
				jobs = append(jobs, dto.PrintJobDTO{
					ID:           pj.ID,
					ModelID:      &m.ID,
					PrintDate:    pj.PrintDate,
					UnitsPrinted: pj.UnitsPrinted,
					LaborHours:   pj.LaborHours,
					CreatedAt:    pj.CreatedAt,
					UpdatedAt:    pj.UpdatedAt,
				})
			}
			dtoList = append(dtoList, dto.ModelDTO{
				ID:             m.ID,
				Name:           m.Name,
				ImageURL:       m.ImageURL,
				MaterialWeight: m.MaterialWeight,
				PrintTime:      m.PrintTime,
				WasteMaterial:  m.WasteMaterial,
				IsMulticolor:   m.IsMulticolor,
				CreatedAt:      m.CreatedAt,
				UpdatedAt:      m.UpdatedAt,
				PrintJobs:      jobs,
				PrintJobCount:  len(m.PrintJobs),
			})
		}

		if params.Enabled {
			c.JSON(200, PageResponse[dto.ModelDTO]{Items: dtoList, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, dtoList)
	}
}

// GetModel godoc
// @Summary      Get model by ID
// @Description  Retrieve a specific model by ID with its print jobs
// @Tags         models
// @Param        id  path      string  true  "Model ID"
// @Produce      json
// @Success      200  {object}  dto.ModelDTO
// @Router       /models/{id} [get]
func GetModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var model models.Model
		if err := db.Preload("PrintJobs").Where("id = ?", id).First(&model).Error; err != nil {
			c.JSON(404, gin.H{"error": "Model not found"})
			return
		}

		var jobs []dto.PrintJobDTO
		for _, pj := range model.PrintJobs {
			jobs = append(jobs, dto.PrintJobDTO{
				ID:           pj.ID,
				ModelID:      &model.ID,
				PrintDate:    pj.PrintDate,
				UnitsPrinted: pj.UnitsPrinted,
				LaborHours:   pj.LaborHours,
				CreatedAt:    pj.CreatedAt,
				UpdatedAt:    pj.UpdatedAt,
			})
		}

		dto := dto.ModelDTO{
			ID:             model.ID,
			Name:           model.Name,
			ImageURL:       model.ImageURL,
			MaterialWeight: model.MaterialWeight,
			PrintTime:      model.PrintTime,
			WasteMaterial:  model.WasteMaterial,
			IsMulticolor:   model.IsMulticolor,
			CreatedAt:      model.CreatedAt,
			UpdatedAt:      model.UpdatedAt,
			PrintJobs:      jobs,
			PrintJobCount:  len(model.PrintJobs),
		}

		c.JSON(200, dto)
	}
}

// CreateModel godoc
// @Summary      Create a new model
// @Description  Create a new model entry
// @Tags         models
// @Param        input  body      ModelInput  true  "Model Input"
// @Produce      json
// @Success      201  {object}  dto.ModelDTO
// @Router       /models [post]
func CreateModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ModelInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		model := models.Model{
			Name:           input.Name,
			ImageURL:       input.ImageURL,
			MaterialWeight: input.MaterialWeight,
			PrintTime:      input.PrintTime,
			WasteMaterial:  input.WasteMaterial,
			IsMulticolor:   input.IsMulticolor,
		}
		if err := db.Create(&model).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to create model"})
			return
		}
		dtoObj := dto.ModelDTO{
			ID:             model.ID,
			Name:           model.Name,
			ImageURL:       model.ImageURL,
			MaterialWeight: model.MaterialWeight,
			PrintTime:      model.PrintTime,
			WasteMaterial:  model.WasteMaterial,
			IsMulticolor:   model.IsMulticolor,
			CreatedAt:      model.CreatedAt,
			UpdatedAt:      model.UpdatedAt,
			PrintJobs:      nil, // or preload if you want
			PrintJobCount:  0,
		}
		c.JSON(201, dtoObj)
	}
}

// UpdateModel godoc
// @Summary      Update an existing model
// @Description  Update an existing model entry
// @Tags         models
// @Param        id    path      string      true  "Model ID"
// @Param        input body      ModelInput  true  "Model Input"
// @Produce      json
// @Success      200  {object}  dto.ModelDTO
// @Router       /models/{id} [put]
func UpdateModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var model models.Model
		if err := db.Where("id = ?", id).First(&model).Error; err != nil {
			c.JSON(404, gin.H{"error": "Model not found"})
			return
		}
		var input ModelInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		model.Name = input.Name
		model.ImageURL = input.ImageURL
		model.MaterialWeight = input.MaterialWeight
		model.PrintTime = input.PrintTime
		model.WasteMaterial = input.WasteMaterial
		model.IsMulticolor = input.IsMulticolor
		model.UpdatedAt = time.Now()

		if err := db.Save(&model).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to update model"})
			return
		}
		dtoObj := dto.ModelDTO{
			ID:             model.ID,
			Name:           model.Name,
			ImageURL:       model.ImageURL,
			MaterialWeight: model.MaterialWeight,
			PrintTime:      model.PrintTime,
			WasteMaterial:  model.WasteMaterial,
			IsMulticolor:   model.IsMulticolor,
			CreatedAt:      model.CreatedAt,
			UpdatedAt:      model.UpdatedAt,
			PrintJobs:      nil, // or preload if you want
			PrintJobCount:  0,
		}
		c.JSON(200, dtoObj)
	}
}

// DeleteModel godoc
// @Summary      Delete model
// @Description  Delete a model entry by ID
// @Tags         models
// @Param        id  path      string  true  "Model ID"
// @Produce      json
// @Success      204
// @Router       /models/{id} [delete]
func DeleteModel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		if err := db.Where("id = ?", id).Delete(&models.Model{}).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to delete model"})
			return
		}
		c.Status(204)
	}
}
