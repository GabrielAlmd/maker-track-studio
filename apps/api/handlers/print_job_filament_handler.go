package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type PrintJobFilamentInput struct {
	PrintJobID uuid.UUID `json:"print_job_id" binding:"required"`
	FilamentID uuid.UUID `json:"filament_id" binding:"required"`
	WeightUsed float64   `json:"weight_used" binding:"required"`
}

// GetPrintJobsFilament godoc
// @Summary      List all print job filaments
// @Description  Retrieve all print job filament relations
// @Tags         print_job_filaments
// @Produce      json
// @Success      200  {array}  dto.PrintJobFilamentDTO
// @Router       /print-job-filaments [get]
func GetPrintJobsFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var printJobs []models.PrintJobFilament
		if err := db.Preload("Filament").Preload("PrintJob").Find(&printJobs).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print jobs with filament"})
			return
		}
		var dtoList []dto.PrintJobFilamentDTO
		for _, pjf := range printJobs {
			dtoList = append(dtoList, printJobFilamentDTO(pjf))
		}
		c.JSON(200, dtoList)
	}
}

// GetPrintJobFilament godoc
// @Summary      Get print job filament by ID
// @Description  Retrieve a specific print job filament by ID
// @Tags         print_job_filaments
// @Param        id  path      string  true  "Print Job Filament ID"
// @Produce      json
// @Success      200  {object}  dto.PrintJobFilamentDTO
// @Router       /print-job-filaments/{id} [get]
func GetPrintJobFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var pjf models.PrintJobFilament
		if err := db.Preload("Filament").Preload("PrintJob").Where("id = ?", id).First(&pjf).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job filament not found"})
			return
		}
		c.JSON(200, printJobFilamentDTO(pjf))
	}
}

// CreatePrintJobFilament godoc
// @Summary      Create a new print job filament
// @Description  Create a new print job filament entry
// @Tags         print_job_filaments
// @Param        input  body      PrintJobFilamentInput  true  "Print Job Filament Input"
// @Produce      json
// @Success      201  {object}  dto.PrintJobFilamentDTO
// @Router       /print-job-filaments [post]
func CreatePrintJobFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input PrintJobFilamentInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if !printJobFilamentTargetsExist(db, input.PrintJobID, input.FilamentID) {
			c.JSON(400, gin.H{"error": "Print job or filament not found"})
			return
		}
		printJobFilament := models.PrintJobFilament{
			PrintJobID: input.PrintJobID,
			FilamentID: input.FilamentID,
			WeightUsed: input.WeightUsed,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := db.Create(&printJobFilament).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to create print job filament"})
			return
		}
		if err := db.Preload("Filament").First(&printJobFilament, "id = ?", printJobFilament.ID).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print job filament"})
			return
		}
		c.JSON(201, printJobFilamentDTO(printJobFilament))
	}
}

// UpdatePrintJobFilament godoc
// @Summary      Update an existing print job filament
// @Description  Update an existing print job filament entry
// @Tags         print_job_filaments
// @Param        id    path      string                  true  "Print Job Filament ID"
// @Param        input body      PrintJobFilamentInput   true  "Print Job Filament Input"
// @Produce      json
// @Success      200  {object}  dto.PrintJobFilamentDTO
// @Router       /print-job-filaments/{id} [put]
func UpdatePrintJobFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var printJobFilament models.PrintJobFilament
		if err := db.Where("id = ?", id).First(&printJobFilament).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job filament not found"})
			return
		}
		var input PrintJobFilamentInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if !printJobFilamentTargetsExist(db, input.PrintJobID, input.FilamentID) {
			c.JSON(400, gin.H{"error": "Print job or filament not found"})
			return
		}
		printJobFilament.PrintJobID = input.PrintJobID
		printJobFilament.FilamentID = input.FilamentID
		printJobFilament.WeightUsed = input.WeightUsed
		printJobFilament.UpdatedAt = time.Now()

		if err := db.Save(&printJobFilament).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to update print job filament"})
			return
		}
		if err := db.Preload("Filament").First(&printJobFilament, "id = ?", printJobFilament.ID).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve print job filament"})
			return
		}
		c.JSON(200, printJobFilamentDTO(printJobFilament))
	}
}

func printJobFilamentTargetsExist(db *gorm.DB, printJobID, filamentID uuid.UUID) bool {
	var jobs, filaments int64
	if db.Model(&models.PrintJob{}).Where("id = ?", printJobID).Count(&jobs).Error != nil || jobs != 1 {
		return false
	}
	return db.Model(&models.Filament{}).Where("id = ?", filamentID).Count(&filaments).Error == nil && filaments == 1
}

func printJobFilamentDTO(pjf models.PrintJobFilament) dto.PrintJobFilamentDTO {
	return dto.PrintJobFilamentDTO{ID: pjf.ID, PrintJobID: pjf.PrintJobID, FilamentID: pjf.FilamentID, WeightUsed: pjf.WeightUsed, Filament: filamentReference(pjf)}
}

// DeletePrintJobFilament godoc
// @Summary      Delete print job filament
// @Description  Delete a print job filament entry by ID
// @Tags         print_job_filaments
// @Param        id  path      string  true  "Print Job Filament ID"
// @Produce      json
// @Success      204
// @Router       /print-job-filaments/{id} [delete]
func DeletePrintJobFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		if err := db.Where("id = ?", id).Delete(&models.PrintJobFilament{}).Error; err != nil {
			c.JSON(404, gin.H{"error": "Print job filament not found"})
			return
		}
		c.Status(204)
	}
}
