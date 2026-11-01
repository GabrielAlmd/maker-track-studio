package models

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Printer identifies one physical machine, not a manufacturer/model catalogue entry.
// No hard-delete API is exposed: archived machines must retain their stable IDs.
type Printer struct {
	CostConfig   map[string]float64 `gorm:"serializer:json" json:"cost_config"`
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name         string    `gorm:"not null" json:"name"`
	Manufacturer string    `gorm:"not null" json:"manufacturer"`
	Model        string    `gorm:"not null" json:"model"`
	Notes        string    `gorm:"not null" json:"notes"`
	IsDefault    bool      `gorm:"not null;default:false;uniqueIndex:idx_printers_default,where:is_default = 1" json:"is_default"`
	IsArchived   bool      `gorm:"not null;default:false;check:chk_printers_active_default,NOT (is_default = 1 AND is_archived = 1)" json:"is_archived"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Printer) TableName() string { return "printers" }

func (p *Printer) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&p.ID)
	return nil
}

// ValidatePrinter also applies to restored backups, which bypass request binding.
func ValidatePrinter(p Printer) error {
	if p.ID == uuid.Nil {
		return fmt.Errorf("printer ID is required")
	}
	for _, field := range []struct {
		value string
		max   int
	}{
		{p.Name, 100}, {p.Manufacturer, 100}, {p.Model, 100},
	} {
		if strings.TrimSpace(field.value) == "" || utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("printer name, manufacturer and model must contain 1 to 100 characters")
		}
	}
	if utf8.RuneCountInString(p.Notes) > 2000 {
		return fmt.Errorf("printer notes must not exceed 2000 characters")
	}
	if p.IsDefault && p.IsArchived {
		return fmt.Errorf("an archived printer cannot be the default")
	}
	if p.CostConfig != nil {
		return ValidatePrinterCostConfig(p.CostConfig)
	}
	return nil
}
