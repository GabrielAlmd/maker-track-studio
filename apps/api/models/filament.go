package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Filament struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Brand          string    `gorm:"not null"`
	Type           string    `gorm:"not null"`
	Color          string    `gorm:"not null"`
	AvgPrice       float64   `gorm:"not null"`
	PersonalScore  int
	TimesUsed      int
	TimesPurchased int
	CreatedAt      time.Time
	UpdatedAt      time.Time

	Purchases []FilamentPurchase `gorm:"foreignKey:FilamentID"`
	PrintJobs []PrintJobFilament `gorm:"foreignKey:FilamentID"`
}

func (f *Filament) TableName() string {
	return "filaments"
}

func (f *Filament) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&f.ID)
	return nil
}
