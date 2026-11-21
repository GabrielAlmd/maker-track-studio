package storage

import "gorm.io/gorm"

// ProductionSources selects verified run items. Only completed output is
// accepted supply, even if an in-progress run carries tentative counts.
func ProductionSources(db *gorm.DB) *gorm.DB {
	return db.Table("print_run_items AS item").
		Select(`item.id AS print_run_item_id, run.id AS print_run_id, job.id AS print_job_id,
			item.model_id, job.print_date, run.status, item.planned AS units_planned,
			CASE WHEN run.status = 'completed' THEN item.accepted ELSE 0 END AS units_accepted,
			CASE WHEN run.status = 'completed' THEN MAX(0, item.accepted - COALESCE((
				SELECT SUM(allocation.units_allocated)
				FROM sales_order_allocations AS allocation
				JOIN sales_order_items AS line ON line.id = allocation.sales_order_item_id
				JOIN sales_orders AS orders ON orders.id = line.sales_order_id
				WHERE allocation.print_run_item_id = item.id AND orders.fulfilment_status <> 'cancelled'
			), 0)) ELSE 0 END AS units_available`).
		Joins("JOIN print_runs AS run ON run.id = item.print_run_id").
		Joins("JOIN print_jobs AS job ON job.id = run.print_job_id").
		Where("run.provenance = ? AND job.workflow_version = ?", "verified", 2)
}
