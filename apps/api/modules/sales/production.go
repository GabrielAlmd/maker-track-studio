package sales

import (
	"reflect"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
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
