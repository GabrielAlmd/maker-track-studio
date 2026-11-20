package models

import (
	"gorm.io/gorm"
)

// NormalizeLegacyProduction is repeatable on startup and older backup import.
// Existing material UUIDs and financial inputs remain authoritative.
func NormalizeLegacyProduction(tx *gorm.DB) error {
	var jobs []PrintJob
	if err := tx.Where("workflow_version <= ?", 1).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if err := SyncLegacyProduction(tx, job); err != nil {
			return err
		}
	}
	return nil
}

func SyncLegacyProduction(tx *gorm.DB, job PrintJob) error {
	if job.WorkflowVersion > 1 {
		return nil
	}
	var run PrintRun
	if err := tx.Where("print_job_id = ? AND provenance = ?", job.ID, "legacy_unverified").Attrs(PrintRun{PrintJobID: job.ID, Status: "completed", Provenance: "legacy_unverified", Revision: 1}).FirstOrCreate(&run).Error; err != nil {
		return err
	}
	var item PrintRunItem
	if err := tx.Where("print_run_id = ?", run.ID).Attrs(PrintRunItem{PrintRunID: run.ID, ModelID: job.ModelID, LegacyReportedUnits: int64(job.UnitsPrinted)}).FirstOrCreate(&item).Error; err != nil {
		return err
	}
	if err := tx.Model(&item).Updates(map[string]any{"model_id": job.ModelID, "legacy_reported_units": job.UnitsPrinted}).Error; err != nil {
		return err
	}
	return tx.Model(&PrintJobFilament{}).Where("print_job_id = ? AND print_run_id IS NULL", job.ID).Update("print_run_id", run.ID).Error
}
