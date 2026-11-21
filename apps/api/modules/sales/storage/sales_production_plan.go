package storage

import "github.com/google/uuid"

type SalesProductionPlan struct {
	SalesOrderItemID uuid.UUID      `gorm:"type:uuid;primaryKey"`
	SalesOrderItem   SalesOrderItem `gorm:"foreignKey:SalesOrderItemID;constraint:OnDelete:RESTRICT"`
	PrintRunItemID   uuid.UUID      `gorm:"type:uuid;primaryKey;index"`
	UnitsPlanned     int64          `gorm:"not null;check:chk_sales_production_plan_units,units_planned > 0 AND units_planned <= 1000000000"`
}

func (SalesProductionPlan) TableName() string { return "sales_production_plans" }
