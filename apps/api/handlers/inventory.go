package handlers

import (
	"errors"
	"strconv"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
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
			Quantity *int64 `json:"quantity"`
		}
		if c.ShouldBindJSON(&input) != nil || input.Quantity == nil {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		input.Posting.Quantity = *input.Quantity
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
