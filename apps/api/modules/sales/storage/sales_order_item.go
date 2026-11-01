package storage

import (
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SalesOrderItem struct {
	ID                uuid.UUID     `gorm:"type:uuid;primaryKey"`
	SalesOrderID      uuid.UUID     `gorm:"type:uuid;not null;index"`
	SalesOrder        SalesOrder    `gorm:"foreignKey:SalesOrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ModelID           *uuid.UUID    `gorm:"type:uuid;index"`
	Model             *models.Model `gorm:"foreignKey:ModelID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	ModelNameSnapshot string        `gorm:"not null"`
	Quantity          int           `gorm:"not null;check:chk_sales_order_items_quantity,quantity > 0"`
	UnitPriceCents    int64         `gorm:"not null;check:chk_sales_order_items_unit_price,unit_price_cents >= 0"`
	CreatedAt         time.Time
	UpdatedAt         time.Time

	Allocations []SalesOrderAllocation `gorm:"foreignKey:SalesOrderItemID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (SalesOrderItem) TableName() string { return "sales_order_items" }

func (item *SalesOrderItem) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&item.ID)
	return nil
}
