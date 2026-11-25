package handlers

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/gorm"
)

func validPurchaseAmount(amount float64) bool {
	q, err := inventory.Milligrams(amount, 1000000)
	return err == nil && q > 0
}

func syncPurchaseStock(tx *gorm.DB, purchase models.FilamentPurchase, previous *models.FilamentPurchase) error {
	key := "purchase:" + purchase.ID.String()
	var old *inventory.SourceAmount
	if previous != nil {
		// Unposted legacy and paused purchases are financial records only.
		var posted bool
		history := tx.Model(&inventory.Movement{}).Select("1").Where("source_key = ?", key)
		if err := tx.Raw("SELECT EXISTS (?)", history).Scan(&posted).Error; err != nil {
			return err
		}
		if !posted {
			return nil
		}
		priorMG, err := inventory.Milligrams(previous.Amount, 1000000)
		if err != nil {
			return err
		}
		old = &inventory.SourceAmount{ItemID: previous.FilamentID, Quantity: priorMG}
	}
	q, err := inventory.Milligrams(purchase.Amount, 1000000)
	if err != nil || (purchase.Amount != 0 && q == 0) {
		return inventory.ErrInvalid
	}
	return inventory.SetSource(tx, key, inventory.SourceAmount{ItemID: purchase.FilamentID, Quantity: q}, "purchase", old)
}
