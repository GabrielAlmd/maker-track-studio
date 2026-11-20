package models

import (
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PrintRun is one physical attempt. LegacyReportedUnits never represents
// verified accepted output and is retained separately during migration.
type PrintRun struct {
	ID               uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	PrintJobID       uuid.UUID          `gorm:"type:uuid;not null;index" json:"print_job_id"`
	Status           string             `gorm:"not null" json:"status"`
	Provenance       string             `gorm:"not null" json:"provenance"`
	Revision         int64              `gorm:"not null" json:"revision"`
	PrintTimeMinutes *int               `json:"print_time_minutes"`
	LaborHours       float32            `json:"labor_hours"`
	Materials        []PrintJobFilament `gorm:"foreignKey:PrintRunID" json:"-"`
	Items            []PrintRunItem     `gorm:"foreignKey:PrintRunID" json:"items"`
}

type ProductionOperation struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	PrintJobID  uuid.UUID `gorm:"type:uuid;not null;index"`
	PayloadHash string    `gorm:"not null"`
	Response    string    `gorm:"not null"`
}

type PrintRunItem struct {
	Model               *Model     `gorm:"foreignKey:ModelID;constraint:OnDelete:SET NULL" json:"-"`
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	PrintRunID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"print_run_id"`
	ModelID             *uuid.UUID `gorm:"type:uuid" json:"model_id"`
	ModelName           string     `json:"model_name"`
	Planned             int64      `json:"planned"`
	Attempted           int64      `json:"attempted"`
	Accepted            int64      `json:"accepted"`
	Rejected            int64      `json:"rejected"`
	LegacyReportedUnits int64      `json:"legacy_reported_units"`
}

func (r *PrintRun) BeforeCreate(_ *gorm.DB) error     { ensureUUID(&r.ID); return nil }
func (r *PrintRunItem) BeforeCreate(_ *gorm.DB) error { ensureUUID(&r.ID); return nil }

func ValidatePrintRun(r PrintRun) error {
	if r.Provenance == "legacy_unverified" {
		if r.Status != "completed" || len(r.Items) != 1 {
			return errors.New("legacy run must remain completed")
		}
		for _, i := range r.Items {
			if i.Accepted != 0 || i.Rejected != 0 || i.Attempted != 0 {
				return errors.New("legacy output cannot become verified implicitly")
			}
		}
		return nil
	}
	if r.Provenance != "verified" || len(r.Items) == 0 || len(r.Items) > 100 {
		return errors.New("invalid run provenance or items")
	}
	terminal := r.Status == "completed" || r.Status == "failed" || r.Status == "cancelled"
	if !terminal && r.Status != "planned" && r.Status != "in_progress" {
		return errors.New("invalid run status")
	}
	for _, i := range r.Items {
		if i.Planned <= 0 || i.Planned > 1000000000 || i.Attempted < 0 || i.Attempted > i.Planned || i.Accepted < 0 || i.Rejected < 0 || i.Accepted > i.Attempted || i.Rejected > i.Attempted-i.Accepted || i.LegacyReportedUnits != 0 {
			return errors.New("invalid run quantities")
		}
		if terminal && i.Accepted+i.Rejected != i.Attempted {
			return errors.New("terminal quantities must reconcile")
		}
		if (r.Status == "failed" || r.Status == "cancelled" || r.Status == "planned") && i.Accepted != 0 {
			return errors.New("only completed output can be accepted")
		}
		if r.Status == "planned" && i.Attempted != 0 {
			return errors.New("planned run cannot have attempts")
		}
	}
	return nil
}

func ValidPrintRunTransition(from, to string) bool {
	return from == to || from == "planned" && (to == "in_progress" || to == "cancelled") || from == "in_progress" && (to == "completed" || to == "failed" || to == "cancelled")
}
