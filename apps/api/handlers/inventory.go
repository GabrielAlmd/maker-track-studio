package handlers

import (
	"errors"
	"strconv"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func inventoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, inventory.ErrInvalid):
		c.JSON(400, gin.H{"error": err.Error()})
	case errors.Is(err, inventory.ErrConflict):
		c.JSON(409, gin.H{"error": err.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(404, gin.H{"error": "Inventory item not found"})
	default:
		c.JSON(500, gin.H{"error": "Inventory operation failed"})
	}
}

func AdjustInventory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.ContentType() != gin.MIMEJSON {
			c.JSON(415, gin.H{"error": "Expected application/json"})
			return
		}
		var input struct {
			inventory.Posting
			Quantity         *int64 `json:"quantity"`
			CountedQuantity  *int64 `json:"counted_quantity"`
			ExpectedRevision *int64 `json:"expected_revision"`
		}
		if c.ShouldBindJSON(&input) != nil || input.SourceKey != "" {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		switch input.Kind {
		case "count":
			if input.Quantity != nil || input.CountedQuantity == nil || input.ExpectedRevision == nil {
				inventoryError(c, inventory.ErrInvalid)
				return
			}
			input.Posting.CountedQuantity = *input.CountedQuantity
			input.Posting.ExpectedRevision = *input.ExpectedRevision
		case "opening", "adjustment":
			if input.Quantity == nil || input.CountedQuantity != nil || input.ExpectedRevision != nil {
				inventoryError(c, inventory.ErrInvalid)
				return
			}
			input.Posting.Quantity = *input.Quantity
		default:
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		var receipt inventory.Receipt
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			receipt, err = inventory.Post(tx, input.Posting)
			return err
		})
		if err != nil {
			inventoryError(c, err)
			return
		}
		c.JSON(201, receipt)
	}
}

func InventoryBalance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		b, err := inventory.Availability(db, c.Param("type"), id)
		if err != nil {
			inventoryError(c, err)
			return
		}
		c.JSON(200, b)
	}
}

func InventoryHistory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		after, err := strconv.ParseInt(c.DefaultQuery("after", "0"), 10, 64)
		if err != nil || after < 0 {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		rows, err := inventory.History(db, c.Param("type"), id, after)
		if err != nil {
			inventoryError(c, err)
			return
		}
		c.JSON(200, rows)
	}
}

// Keep item identities available for historical balances and backup restore.
func deleteInventoryItem(c *gin.Context, db *gorm.DB, typ string, item any) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid UUID"})
		return
	}
	history := db.Model(&inventory.Movement{}).Select("1").Where("item_type = ? AND item_id = ?", typ, id)
	result := db.Where("id = ? AND NOT EXISTS (?)", id, history).Delete(item)
	if result.Error != nil {
		inventoryError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		var used bool
		if err := db.Raw("SELECT EXISTS (?)", history).Scan(&used).Error; err != nil {
			inventoryError(c, err)
			return
		}
		if used {
			c.JSON(409, gin.H{"error": "Inventory history references this item"})
			return
		}
	}
	c.Status(204)
}

// InventoryMaterials returns ledger projections without purchase/usage counters.
func InventoryMaterials(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var items []models.Filament
		if err := db.Order("brand, type, color").Find(&items).Error; err != nil {
			inventoryError(c, err)
			return
		}
		rows := make([]gin.H, 0, len(items))
		for _, item := range items {
			balance, err := inventory.Availability(db, "filament", item.ID)
			if err != nil {
				inventoryError(c, err)
				return
			}
			rows = append(rows, gin.H{"id": item.ID, "brand": item.Brand, "type": item.Type, "color": item.Color, "threshold_mg": item.StockThresholdMG, "balance": balance})
		}
		c.JSON(200, rows)
	}
}

func UpdateInventoryThreshold(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		var input struct {
			Threshold *int64 `json:"threshold_mg"`
		}
		if err != nil || c.ShouldBindJSON(&input) != nil || input.Threshold == nil || *input.Threshold < 0 || *input.Threshold > inventory.MaxQuantity {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		result := db.Model(&models.Filament{}).Where("id = ?", id).Update("stock_threshold_mg", *input.Threshold)
		if result.Error != nil {
			inventoryError(c, result.Error)
			return
		}
		if result.RowsAffected == 0 {
			inventoryError(c, gorm.ErrRecordNotFound)
			return
		}
		c.Status(204)
	}
}

func InventoryFinishedGoods(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := inventory.FinishedProducts(db)
		if err != nil {
			inventoryError(c, err)
			return
		}
		c.JSON(200, rows)
	}
}
