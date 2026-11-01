package sales

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"gorm.io/gorm"
)

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
