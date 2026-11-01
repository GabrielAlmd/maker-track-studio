package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Metadata updates cannot alter IDs, timestamps, or default/archive state.
type PrinterInput struct {
	Name         string `json:"name" binding:"required,max=100"`
	Manufacturer string `json:"manufacturer" binding:"required,max=100"`
	Model        string `json:"model" binding:"required,max=100"`
	Notes        string `json:"notes" binding:"max=2000"`
}

var errPrinterConflict = errors.New("printer state conflict")
var errInvalidPrinterCosts = errors.New("invalid printer cost configuration")

func printerID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		c.JSON(400, gin.H{"error": "Invalid printer UUID"})
		return uuid.Nil, false
	}
	return id, true
}

func printerError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(404, gin.H{"error": "Printer not found"})
	case errors.Is(err, errPrinterConflict):
		c.JSON(409, gin.H{"error": "Select another active default printer first, or restore the printer before selecting it"})
	default:
		c.JSON(500, gin.H{"error": "Failed to save or retrieve printer"})
	}
}

func bindPrinter(c *gin.Context, id uuid.UUID) (models.Printer, bool) {
	// Bound allocation even when clients bypass the desktop form.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input PrinterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Invalid printer input"})
		return models.Printer{}, false
	}
	p := models.Printer{
		ID: id, Name: strings.TrimSpace(input.Name),
		Manufacturer: strings.TrimSpace(input.Manufacturer),
		Model:        strings.TrimSpace(input.Model), Notes: strings.TrimSpace(input.Notes),
	}
	if err := models.ValidatePrinter(p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return models.Printer{}, false
	}
	return p, true
}

// Take SQLite's writer lock before reading state. The partial unique index is
// the final guard against two defaults, including writes outside these handlers.
func printerTransaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE printers SET is_default = is_default WHERE is_default = 1").Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

func GetPrinters(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := db.Model(&models.Printer{})
		switch c.DefaultQuery("state", "active") {
		case "active":
			query = query.Where("is_archived = ?", false)
		case "archived":
			query = query.Where("is_archived = ?", true)
		case "all":
		default:
			c.JSON(400, gin.H{"error": "Invalid printer state filter"})
			return
		}
		printers := []models.Printer{}
		if err := query.Order("is_default DESC, name ASC, id ASC").Find(&printers).Error; err != nil {
			printerError(c, err)
			return
		}
		c.JSON(200, printers)
	}
}

func GetPrinter(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := printerID(c)
		if !ok {
			return
		}
		var p models.Printer
		if err := db.Where("id = ?", id).First(&p).Error; err != nil {
			printerError(c, err)
			return
		}
		c.JSON(200, p)
	}
}

func CreatePrinter(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var input struct {
			Name         string          `json:"name" binding:"required,max=100"`
			Manufacturer string          `json:"manufacturer" binding:"required,max=100"`
			Model        string          `json:"model" binding:"required,max=100"`
			Notes        string          `json:"notes" binding:"max=2000"`
			CostConfig   json.RawMessage `json:"cost_config"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid printer input"})
			return
		}
		p := models.Printer{ID: uuid.New(), Name: strings.TrimSpace(input.Name), Manufacturer: strings.TrimSpace(input.Manufacturer), Model: strings.TrimSpace(input.Model), Notes: strings.TrimSpace(input.Notes)}
		if err := models.ValidatePrinter(p); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var suppliedCostConfig map[string]*float64
		if input.CostConfig != nil {
			if err := json.Unmarshal(input.CostConfig, &suppliedCostConfig); err != nil || suppliedCostConfig == nil {
				c.JSON(400, gin.H{"error": "Invalid printer cost configuration"})
				return
			}
		}
		err := printerTransaction(db, func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&models.Printer{}).Where("is_archived = ?", false).Count(&count).Error; err != nil {
				return err
			}
			p.IsDefault = count == 0
			config, err := loadRawCostConfig(tx)
			if err != nil {
				return err
			}
			if suppliedCostConfig != nil {
				for key, value := range suppliedCostConfig {
					if value == nil {
						return errInvalidPrinterCosts
					}
					config[key] = *value
				}
			}
			if err := models.ValidatePrinterCostConfig(config); err != nil {
				return errors.Join(errInvalidPrinterCosts, err)
			}
			p.CostConfig = config
			return tx.Create(&p).Error
		})
		if err != nil {
			if errors.Is(err, errInvalidPrinterCosts) {
				c.JSON(400, gin.H{"error": "Invalid printer cost configuration"})
				return
			}
			printerError(c, err)
			return
		}
		c.JSON(201, p)
	}
}

func UpdatePrinter(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := printerID(c)
		if !ok {
			return
		}
		input, ok := bindPrinter(c, id)
		if !ok {
			return
		}
		var p models.Printer
		err := printerTransaction(db, func(tx *gorm.DB) error {
			if err := tx.Where("id = ?", id).First(&p).Error; err != nil {
				return err
			}
			p.Name, p.Manufacturer, p.Model, p.Notes = input.Name, input.Manufacturer, input.Model, input.Notes
			return tx.Save(&p).Error
		})
		if err != nil {
			printerError(c, err)
			return
		}
		c.JSON(200, p)
	}
}

// SetPrinterDefault atomically replaces the previous default.
func SetPrinterDefault(db *gorm.DB) gin.HandlerFunc {
	return changePrinterState(db, "default")
}

func ArchivePrinter(db *gorm.DB) gin.HandlerFunc {
	return changePrinterState(db, "archive")
}

func RestorePrinter(db *gorm.DB) gin.HandlerFunc {
	return changePrinterState(db, "restore")
}

func changePrinterState(db *gorm.DB, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := printerID(c)
		if !ok {
			return
		}
		var p models.Printer
		err := printerTransaction(db, func(tx *gorm.DB) error {
			if err := tx.Where("id = ?", id).First(&p).Error; err != nil {
				return err
			}
			switch action {
			case "default":
				if p.IsArchived {
					return errPrinterConflict
				}
				if err := tx.Model(&models.Printer{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
					return err
				}
				p.IsDefault = true
			case "archive":
				if p.IsDefault {
					var others int64
					if err := tx.Model(&models.Printer{}).Where("is_archived = ? AND id <> ?", false, id).Count(&others).Error; err != nil {
						return err
					}
					if others > 0 {
						return errPrinterConflict
					}
				}
				p.IsDefault, p.IsArchived = false, true
			case "restore":
				if p.IsArchived {
					var active int64
					if err := tx.Model(&models.Printer{}).Where("is_archived = ?", false).Count(&active).Error; err != nil {
						return err
					}
					p.IsDefault, p.IsArchived = active == 0, false
				}
			}
			return tx.Save(&p).Error
		})
		if err != nil {
			printerError(c, err)
			return
		}
		c.JSON(200, p)
	}
}
