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
	err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		history := tx.Model(&inventory.Movement{}).Select("1").Where("item_type = ? AND item_id = ?", typ, id)
		result := tx.Where("id = ? AND NOT EXISTS (?)", id, history).Delete(item)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var used bool
			if err := tx.Raw("SELECT EXISTS (?)", history).Scan(&used).Error; err != nil {
				return err
			}
			if used {
				return inventory.ErrConflict
			}
			return nil
		}
		// Keep these references consistent even on legacy connections with foreign keys disabled.
		if typ == "model" {
			return tx.Where("model_id = ?", id).Delete(&models.ModelMaterialRequirement{}).Error
		}
		return tx.Model(&models.ModelMaterialRequirement{}).Where("preferred_filament_id = ?", id).
			Update("preferred_filament_id", nil).Error
	})
	if errors.Is(err, inventory.ErrConflict) {
		c.JSON(409, gin.H{"error": "Inventory history references this item"})
		return
	}
	if err != nil {
		inventoryError(c, err)
		return
	}
	c.Status(204)
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

// UpdateInventoryLifecycle applies independent controls under SQLite's writer lock.
func UpdateInventoryLifecycle(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		var input struct {
			ExpectedRevision *int64 `json:"expected_revision"`
			Tracking         *bool  `json:"tracking_enabled"`
			Warnings         *bool  `json:"warnings_enabled"`
			Archived         *bool  `json:"archived"`
		}
		if err != nil || c.ShouldBindJSON(&input) != nil || (input.Tracking == nil && input.Warnings == nil && input.Archived == nil) {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		if input.Tracking != nil && (input.ExpectedRevision == nil || *input.ExpectedRevision < 0 || *input.ExpectedRevision > inventory.MaxQuantity) {
			inventoryError(c, inventory.ErrInvalid)
			return
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("UPDATE inventory_movements SET sequence = sequence WHERE 0").Error; err != nil {
				return err
			}
			b, err := inventory.Availability(tx, "filament", id)
			if err != nil {
				return err
			}
			if input.Tracking != nil && *input.Tracking == b.TrackingPaused {
				if *input.ExpectedRevision != b.Revision {
					return inventory.ErrConflict
				}
				kind := "tracking_paused"
				if *input.Tracking {
					kind = "tracking_resumed"
				}
				_, err := inventory.Post(tx, inventory.Posting{OperationID: uuid.New(), SourceID: uuid.New(), ItemType: "filament", ItemID: id, Kind: kind, Reason: kind, Actor: "local operator"})
				if err != nil {
					return err
				}
			}
			updates := map[string]any{}
			if input.Warnings != nil {
				updates["warnings_disabled"] = !*input.Warnings
			}
			if input.Archived != nil {
				updates["archived"] = *input.Archived
			}
			if len(updates) == 0 {
				return nil
			}
			return tx.Model(&models.Filament{}).Where("id = ?", id).Updates(updates).Error
		})
		if err != nil {
			inventoryError(c, err)
			return
		}
		c.Status(204)
	}
}
