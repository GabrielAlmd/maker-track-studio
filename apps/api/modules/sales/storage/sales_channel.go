package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SalesChannel identifies where an order was received. Archived channels stay
// available to historical orders and are not automatically removed.
type SalesChannel struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name       string    `gorm:"not null"`
	Notes      string    `gorm:"not null;default:''"`
	IsArchived bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time

	Orders []SalesOrder `gorm:"foreignKey:SalesChannelID"`
}

func (SalesChannel) TableName() string { return "sales_channels" }

func (channel *SalesChannel) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&channel.ID)
	return nil
}
