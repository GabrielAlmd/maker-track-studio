package sales

import (
	"errors"
	"reflect"

	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errInvalidProductionBackup = errors.New("Backup contains inconsistent Sales Order production links")

func (data *backupData) validateProduction(db *gorm.DB, schemaVersion int) error {
	hasPlans := db.Migrator().HasTable(&storage.SalesProductionPlan{})
	if schemaVersion >= 15 && (!hasPlans ||
		!db.Migrator().HasColumn(&storage.SalesOrder{}, "production_revision") ||
		!db.Migrator().HasColumn(&storage.SalesOrderAllocation{}, "print_run_item_id")) {
		return errors.New("Backup is missing Sales Order production data")
	}
	if hasPlans {
		if err := db.Find(&data.plans).Error; err != nil {
			return errors.New("Backup Sales Order plans could not be read")
		}
	}
	orders := make(map[uuid.UUID]storage.SalesOrder, len(data.orders))
	for _, order := range data.orders {
		if order.ProductionRevision < 0 || order.ProductionRevision > 0 && (schemaVersion < 15 || order.AcceptedProposalID == nil) {
			return errInvalidProductionBackup
		}
		orders[order.ID] = order
	}
	hasAllocations := false
	for _, allocation := range data.allocations {
		hasAllocations = hasAllocations || allocation.PrintRunItemID != nil
	}
	if len(data.plans) == 0 && !hasAllocations {
		return nil
	}
	if schemaVersion < 15 {
		return errInvalidProductionBackup
	}
	items := make(map[uuid.UUID]storage.SalesOrderItem, len(data.items))
	for _, item := range data.items {
		items[item.ID] = item
	}

	var rows []salesdto.ProductionSource
	if err := storage.ProductionSources(db).Where(`item.id IN (
		SELECT print_run_item_id FROM sales_production_plans
		UNION SELECT print_run_item_id FROM sales_order_allocations WHERE print_run_item_id IS NOT NULL
	)`).Scan(&rows).Error; err != nil {
		return errInvalidProductionBackup
	}
	sources := make(map[uuid.UUID]salesdto.ProductionSource, len(rows))
	for _, source := range rows {
		sources[source.PrintRunItemID] = source
	}
	type linkKey struct{ line, source uuid.UUID }
	validLink := func(key linkKey) bool {
		line, lineExists := items[key.line]
		source, sourceExists := sources[key.source]
		order := orders[line.SalesOrderID]
		return lineExists && sourceExists && order.AcceptedProposalID != nil &&
			order.ProductionRevision > 0 && reflect.DeepEqual(line.ModelID, source.ModelID)
	}

	seenPlans := make(map[linkKey]bool, len(data.plans))
	planned := make(map[uuid.UUID]int64)
	for _, plan := range data.plans {
		key := linkKey{plan.SalesOrderItemID, plan.PrintRunItemID}
		if !validLink(key) || seenPlans[key] || plan.UnitsPlanned <= 0 || plan.UnitsPlanned > 1_000_000_000 {
			return errInvalidProductionBackup
		}
		seenPlans[key] = true
		if orders[items[key.line].SalesOrderID].FulfilmentStatus != storage.SalesOrderFulfilmentCancelled {
			if plan.UnitsPlanned > sources[key.source].UnitsPlanned-planned[key.source] {
				return errInvalidProductionBackup
			}
			planned[key.source] += plan.UnitsPlanned
		}
	}

	// Include the dispatched stock retained by cancelled orders when
	// checking the shared accepted capacity across all owners.
	var overCapacity int64
	if err := db.Table("sales_order_allocations AS link").
		Joins("JOIN sales_order_items AS line ON line.id = link.sales_order_item_id").
		Joins("JOIN sales_orders AS orders ON orders.id = line.sales_order_id").
		Joins("JOIN print_run_items AS item ON item.id = link.print_run_item_id").
		Group("item.id").Having("SUM(" + storage.AllocationCapacitySQL + ") > item.accepted").
		Count(&overCapacity).Error; err != nil {
		return err
	}
	if overCapacity > 0 {
		return errInvalidProductionBackup
	}
	seenAllocations := make(map[linkKey]bool)
	lineTotals := make(map[uuid.UUID]int64)
	for _, allocation := range data.allocations {
		if allocation.PrintRunItemID == nil {
			continue
		}
		key := linkKey{allocation.SalesOrderItemID, *allocation.PrintRunItemID}
		source := sources[key.source]
		units := int64(allocation.UnitsAllocated)
		if !validLink(key) || seenAllocations[key] || allocation.PrintJobID == nil ||
			*allocation.PrintJobID != source.PrintJobID || units > source.UnitsAccepted ||
			units > int64(items[key.line].Quantity)-lineTotals[key.line] {
			return errInvalidProductionBackup
		}
		seenAllocations[key] = true
		lineTotals[key.line] += units
	}
	return nil
}
