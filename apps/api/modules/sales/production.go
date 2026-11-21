package sales

import (
	"reflect"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (Module) CheckRunItemPlanChange(tx *gorm.DB, before, after models.PrintRunItem) error {
	var references struct {
		Count   int64
		Planned int64
	}
	if err := tx.Table("sales_production_plans AS plan").
		Select("COUNT(*) AS count, COALESCE(SUM(CASE WHEN orders.fulfilment_status <> ? THEN plan.units_planned ELSE 0 END), 0) AS planned",
			salesmodels.SalesOrderFulfilmentCancelled).
		Joins("JOIN sales_order_items AS line ON line.id = plan.sales_order_item_id").
		Joins("JOIN sales_orders AS orders ON orders.id = line.sales_order_id").
		Where("plan.print_run_item_id = ?", before.ID).Scan(&references).Error; err != nil {
		return err
	}
	if references.Count > 0 && (after.Planned < references.Planned || !reflect.DeepEqual(before.ModelID, after.ModelID)) {
		return modules.ErrRunItemPlanConflict
	}
	return nil
}

// CheckPrintJobDeletion protects historical allocations while core holds the
// same SQLite writer lock used by allocation writes.
func (Module) CheckPrintJobDeletion(tx *gorm.DB, id string) error {
	var count int64
	if err := tx.Model(&salesmodels.SalesOrderAllocation{}).Where("print_job_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return modules.PrintJobDeleteConflict{Message: "Print Job is referenced by Sales allocations"}
	}
	return nil
}

// RecordRunTransition snapshots which active orders had a link at the time of
// transition, so releasing or replacing a plan cannot rewrite order history.
func (Module) RecordRunTransition(tx *gorm.DB, run models.PrintRun) error {
	kind := salesmodels.OrderRunEventKind(run.Status)
	if kind == "" {
		return nil
	}
	var orders []uuid.UUID
	if err := tx.Raw(`
		SELECT DISTINCT orders.id
		FROM sales_order_items AS line JOIN sales_orders AS orders ON orders.id = line.sales_order_id
		WHERE orders.fulfilment_status IN ('confirmed', 'preparing')
		AND line.id IN (
			SELECT sales_order_item_id FROM sales_production_plans
			WHERE print_run_item_id IN (SELECT id FROM print_run_items WHERE print_run_id = ?)
			UNION
			SELECT sales_order_item_id FROM sales_order_allocations
			WHERE print_run_item_id IN (SELECT id FROM print_run_items WHERE print_run_id = ?)
		)`, run.ID, run.ID).Scan(&orders).Error; err != nil {
		return err
	}
	if len(orders) == 0 {
		return nil
	}
	events := make([]salesmodels.OrderEvent, 0, len(orders))
	for _, id := range orders {
		events = append(events, salesmodels.OrderEvent{SalesOrderID: id, Kind: kind, Revision: run.Revision, RunID: &run.ID})
	}
	return tx.CreateInBatches(&events, 100).Error
}
