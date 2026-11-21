package sales

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Inventory replays the physical ledger independently. Validate the commercial
// side of each reference before allowing the archive to replace a live database.
func (data *backupData) validateFulfilment(db *gorm.DB) error {
	var rows []inventory.Movement
	if err := db.Where("actor = ? AND kind IN ?", storage.FulfilmentActor,
		[]string{"reserve", "release", "dispatch", "restock", "reject_return"}).Order("sequence").Find(&rows).Error; err != nil {
		return err
	}
	allocations := map[uuid.UUID]storage.SalesOrderAllocation{}
	for _, a := range data.allocations {
		allocations[a.ID] = a
	}
	type totals struct{ held, shipped int64 }
	byAllocation := map[uuid.UUID]totals{}
	owners := map[uuid.UUID]uuid.UUID{}
	for _, row := range rows {
		ref, err := uuid.Parse(row.Reference)
		if err != nil {
			return errInvalidProductionBackup
		}
		owner := owners[ref]
		if row.Kind == "reserve" {
			owner = ref
		}
		a := allocations[owner]
		if a.PrintRunItemID == nil || row.SourceKey != "output:"+a.PrintRunItemID.String() {
			return errInvalidProductionBackup
		}
		owners[row.OperationID] = owner
		t := byAllocation[owner]
		switch row.Kind {
		case "reserve":
			t.held += row.Units
		case "dispatch":
			t.shipped += row.Units
			t.held -= row.Units
		case "release":
			t.held -= row.Units
		}
		byAllocation[owner] = t
	}
	cancelled := map[uuid.UUID]bool{}
	for _, o := range data.orders {
		cancelled[o.ID] = o.FulfilmentStatus == storage.SalesOrderFulfilmentCancelled
	}
	itemOrder := map[uuid.UUID]uuid.UUID{}
	for _, i := range data.items {
		itemOrder[i.ID] = i.SalesOrderID
	}
	for owner, t := range byAllocation {
		a := allocations[owner]
		if t.held < 0 || t.held+t.shipped > int64(a.UnitsAllocated) || cancelled[itemOrder[a.SalesOrderItemID]] && t.held > 0 {
			return errInvalidProductionBackup
		}
	}
	return nil
}
