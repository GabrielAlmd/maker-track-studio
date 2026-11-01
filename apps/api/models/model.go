package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Model struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name           string    `gorm:"not null"`
	ImageURL       string
	MaterialWeight float64 `gorm:"not null"`
	PrintTime      int     `gorm:"not null"`
	WasteMaterial  float64 `gorm:"not null"`
	IsMulticolor   bool    `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time

	PrintJobs []PrintJob `gorm:"foreignKey:ModelID"`
}

func (Model) TableName() string {
	return "models"
}

func (m *Model) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&m.ID)
	return nil
}
