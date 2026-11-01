package handlers

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PurchaseFilamentInput struct {
	Brand string `json:"brand" binding:"required,max=200"`
	Type  string `json:"type" binding:"required,max=200"`
	Color string `json:"color" binding:"required,max=200"`
}

type PurchaseLineInput struct {
	FilamentID  uuid.UUID              `json:"filament_id"`
	NewFilament *PurchaseFilamentInput `json:"new_filament"`
	Price       float64                `json:"price" binding:"required,gt=0"`
	Amount      float64                `json:"amount" binding:"required,gt=0"`
}

type FilamentPurchaseBatchInput struct {
	PurchaseDate time.Time           `json:"purchase_date" binding:"required"`
	Items        []PurchaseLineInput `json:"items" binding:"required,min=1,max=100,dive"`
}

// CreateFilamentPurchaseBatch saves all lines and new filaments atomically using
// the existing schema, preserving compatibility with backups and older records.
func CreateFilamentPurchaseBatch(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input FilamentPurchaseBatchInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid purchase input"})
			return
		}
		for _, item := range input.Items {
			if (item.FilamentID == uuid.Nil) == (item.NewFilament == nil) || math.IsInf(item.Price/item.Amount, 0) {
				c.JSON(400, gin.H{"error": "Each line requires an existing or new filament and valid costs"})
				return
			}
			if f := item.NewFilament; f != nil {
				f.Brand, f.Type, f.Color = strings.TrimSpace(f.Brand), strings.TrimSpace(f.Type), strings.TrimSpace(f.Color)
				if f.Brand == "" || f.Type == "" || f.Color == "" {
					c.JSON(400, gin.H{"error": "Brand, type and color are required"})
					return
				}
			}
		}
		result := make([]dto.FilamentPurchaseDTO, 0, len(input.Items))
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, item := range input.Items {
				id := item.FilamentID
				if f := item.NewFilament; f != nil {
					brand, err := canonicalFilamentBrand(tx, f.Brand)
					if err != nil {
						return err
					}
					filament := models.Filament{Brand: brand, Type: f.Type, Color: f.Color, AvgPrice: item.Price / item.Amount, PersonalScore: 3}
					if err := tx.Create(&filament).Error; err != nil {
						return err
					}
					id = filament.ID
				} else {
					var filament models.Filament
					if err := tx.Where("id = ?", id).First(&filament).Error; err != nil {
						return err
					}
				}
				p := models.FilamentPurchase{FilamentID: id, Price: item.Price, Amount: item.Amount, PurchaseDate: input.PurchaseDate}
				if err := tx.Create(&p).Error; err != nil {
					return err
				}
				result = append(result, dto.FilamentPurchaseDTO{ID: p.ID, FilamentID: p.FilamentID, Price: p.Price, Amount: p.Amount, PurchaseDate: p.PurchaseDate, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
			}
			return nil
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(404, gin.H{"error": "Filament not found"})
			return
		}
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to record purchase"})
			return
		}
		c.JSON(201, result)
	}
}
