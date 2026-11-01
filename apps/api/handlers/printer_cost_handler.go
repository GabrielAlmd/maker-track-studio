package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errJobPrinter = errors.New("select an existing active printer")

func UpdatePrinterCostConfig(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := printerID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var input struct {
			CostConfig map[string]*float64 `json:"cost_config" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid cost configuration"})
			return
		}
		config := make(map[string]float64, len(input.CostConfig))
		for key, value := range input.CostConfig {
			if value == nil {
				c.JSON(400, gin.H{"error": "Cost inputs must be numbers, not null"})
				return
			}
			config[key] = *value
		}
		if err := models.ValidatePrinterCostConfig(config); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var printer models.Printer
		err := printerTransaction(db, func(tx *gorm.DB) error {
			if err := tx.Where("id = ?", id).First(&printer).Error; err != nil {
				return err
			}
			printer.CostConfig = config
			return tx.Save(&printer).Error
		})
		if err != nil {
			printerError(c, err)
			return
		}
		c.JSON(200, printer)
	}
}

func printJobPrinterError(c *gin.Context, err error) {
	if errors.Is(err, errJobPrinter) {
		c.JSON(400, gin.H{"error": "Select an existing active printer"})
		return
	}
	printerError(c, err)
}

// Called only inside printerTransaction. Omitted/null on edit preserves the
// assignment. Re-selecting the same printer preserves its historical snapshot,
// even after archival. An explicit change captures the new printer's inputs.
func assignPrintJobPrinter(tx *gorm.DB, job *models.PrintJob, requested *uuid.UUID, creating bool) error {
	if !creating && (requested == nil || (job.PrinterID != nil && *requested == *job.PrinterID)) {
		return nil
	}
	var printer models.Printer
	query := tx.Where("is_archived = ?", false)
	if requested == nil {
		query = query.Where("is_default = ?", true)
	} else {
		if *requested == uuid.Nil {
			return errJobPrinter
		}
		query = query.Where("id = ?", *requested)
	}
	err := query.First(&printer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errJobPrinter
	}
	if err != nil {
		return err
	}
	if err := models.ValidatePrinterCostConfig(printer.CostConfig); err != nil {
		return err
	}
	job.PrinterID = &printer.ID
	job.PrinterCost = &models.PrinterCostSnapshot{
		Version: 1, PrinterName: printer.Name, CapturedAt: time.Now().UTC(),
		Config: printer.CostConfig,
	}
	return nil
}

func printJobEffectiveConfig(job models.PrintJob, legacyConfig map[string]float64) map[string]float64 {
	if job.PrinterCost == nil {
		return legacyConfig
	}
	return normalizeCostConfig(job.PrinterCost.Config)
}
