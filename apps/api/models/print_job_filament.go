package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PrintJobFilament struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	PrintJobID uuid.UUID `gorm:"type:uuid;not null"`
	PrintJob   PrintJob  `gorm:"foreignKey:PrintJobID"`
	FilamentID uuid.UUID `gorm:"type:uuid;not null"`
	Filament   Filament  `gorm:"foreignKey:FilamentID"`
	WeightUsed float64   `gorm:"not null"` // in grams
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (PrintJobFilament) TableName() string {
	return "print_job_filaments"
}

func (pjf *PrintJobFilament) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&pjf.ID)
	return nil
}
