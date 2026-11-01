package storage

import (
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SalesOrderAllocation struct {
	ID                  uuid.UUID        `gorm:"type:uuid;primaryKey"`
	SalesOrderItemID    uuid.UUID        `gorm:"type:uuid;not null;index"`
	SalesOrderItem      SalesOrderItem   `gorm:"foreignKey:SalesOrderItemID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	PrintJobID          *uuid.UUID       `gorm:"type:uuid;index"`
	PrintJob            *models.PrintJob `gorm:"foreignKey:PrintJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	UnitsAllocated      int              `gorm:"not null;check:chk_sales_order_allocations_units,units_allocated > 0"`
	ProductionCostCents *int64           `gorm:"check:chk_sales_order_allocations_production_cost_snapshot,production_cost_cents IS NULL OR production_cost_cents >= 0"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (SalesOrderAllocation) TableName() string { return "sales_order_allocations" }

func (allocation *SalesOrderAllocation) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&allocation.ID)
	return nil
}
