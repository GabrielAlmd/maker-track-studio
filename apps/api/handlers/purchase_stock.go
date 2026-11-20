package handlers

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func validPurchaseAmount(amount float64) bool {
	q, err := inventory.Milligrams(amount, 1000000)
	return err == nil && q > 0
}

func syncPurchaseStock(tx *gorm.DB, purchase models.FilamentPurchase, creating bool) error {
	key := "purchase:" + purchase.ID.String()
	if !creating {
		var count int64
		if err := tx.Model(&inventory.Movement{}).Where("source_key = ?", key).Count(&count).Error; err != nil {
			return err
		}
		// Historical purchases are reconciled separately, never replayed as stock.
		if count == 0 {
			return nil
		}
	}
	q, err := inventory.Milligrams(purchase.Amount, 1000000)
	if err != nil {
		return err
	}
	return inventory.SetSource(tx, key, map[uuid.UUID]int64{purchase.FilamentID: q}, "purchase")
}
