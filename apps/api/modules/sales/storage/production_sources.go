package storage

import "gorm.io/gorm"

const FulfilmentActor = "Business Operations"

// AllocationCapacitySQL uses the link and orders aliases. Cancelled orders
// retain capacity only for dispatched units that have not returned to stock.
const AllocationCapacitySQL = `CASE WHEN orders.fulfilment_status <> 'cancelled' THEN link.units_allocated ELSE
	COALESCE((SELECT SUM(d.units - COALESCE((SELECT SUM(r.units) FROM inventory_movements r
		WHERE r.kind = 'restock' AND r.reference = d.operation_id), 0))
		FROM inventory_movements h JOIN inventory_movements d ON d.reference = h.operation_id AND d.kind = 'dispatch'
		WHERE h.kind = 'reserve' AND h.actor = 'Business Operations' AND h.reference = link.id), 0) END`

// ProductionSources selects verified run items. Only completed output is
// accepted supply, even if an in-progress run carries tentative counts.
func ProductionSources(db *gorm.DB) *gorm.DB {
	return db.Table("print_run_items AS item").
		Select(`item.id AS print_run_item_id, run.id AS print_run_id, job.id AS print_job_id,
			item.model_id, job.print_date, run.status, item.planned AS units_planned,
			CASE WHEN run.status = 'completed' THEN item.accepted ELSE 0 END AS units_accepted,
			CASE WHEN run.status = 'completed' THEN MAX(0, item.accepted - COALESCE((
				SELECT SUM(`+AllocationCapacitySQL+`)
				FROM sales_order_allocations AS link
				JOIN sales_order_items AS line ON line.id = link.sales_order_item_id
				JOIN sales_orders AS orders ON orders.id = line.sales_order_id
				WHERE link.print_run_item_id = item.id
			), 0)) ELSE 0 END AS units_available`).
		Joins("JOIN print_runs AS run ON run.id = item.print_run_id").
		Joins("JOIN print_jobs AS job ON job.id = run.print_job_id").
		Where("run.provenance = ? AND job.workflow_version = ?", "verified", 2)
}
