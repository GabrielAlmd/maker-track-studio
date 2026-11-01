package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PrintJob struct {
	PrinterID        *uuid.UUID `gorm:"type:uuid;index"`
	PrinterCost      *PrinterCostSnapshot `gorm:"serializer:json"`
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ModelID          *uuid.UUID `gorm:"type:uuid"`
	Model            *Model     `gorm:"foreignKey:ModelID"`
	PrintDate        time.Time  `gorm:"not null"`
	UnitsPrinted     int        `gorm:"not null"`
	LaborHours       float32    `gorm:"not null"`
	PrintTimeMinutes *int       `gorm:"default:null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Filaments []PrintJobFilament `gorm:"foreignKey:PrintJobID"`
}

func (PrintJob) TableName() string {
	return "print_jobs"
}

func (pj *PrintJob) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&pj.ID)
	return nil
}
