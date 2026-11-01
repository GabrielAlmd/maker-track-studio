package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FilamentPurchase struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	FilamentID   uuid.UUID `gorm:"type:uuid;not null"`
	Filament     Filament  `gorm:"foreignKey:FilamentID"`
	Price        float64   `gorm:"not null"`
	Amount       float64   `gorm:"not null"` // in kg
	PurchaseDate time.Time `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (FilamentPurchase) TableName() string {
	return "filament_purchases"
}

func (fp *FilamentPurchase) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&fp.ID)
	return nil
}
