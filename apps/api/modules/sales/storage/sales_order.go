package storage

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SalesOrderFulfilmentStatus string

const (
	SalesOrderFulfilmentDraft       SalesOrderFulfilmentStatus = "draft"
	SalesOrderFulfilmentConfirmed   SalesOrderFulfilmentStatus = "confirmed"
	SalesOrderFulfilmentPreparing   SalesOrderFulfilmentStatus = "preparing"
	SalesOrderFulfilmentReadyToShip SalesOrderFulfilmentStatus = "ready_to_ship"
	SalesOrderFulfilmentShipped     SalesOrderFulfilmentStatus = "shipped"
	SalesOrderFulfilmentCompleted   SalesOrderFulfilmentStatus = "completed"
	SalesOrderFulfilmentCancelled   SalesOrderFulfilmentStatus = "cancelled"
)

type SalesOrderPaymentStatus string

const (
	SalesOrderPaymentUnpaid   SalesOrderPaymentStatus = "unpaid"
	SalesOrderPaymentPaid     SalesOrderPaymentStatus = "paid"
	SalesOrderPaymentRefunded SalesOrderPaymentStatus = "refunded"
)

type SalesOrder struct {
	ID                  uuid.UUID                  `gorm:"type:uuid;primaryKey"`
	Reference           string                     `gorm:"not null"`
	OrderDate           time.Time                  `gorm:"not null"`
	SalesChannelID      *uuid.UUID                 `gorm:"type:uuid;index"`
	SalesChannel        *SalesChannel              `gorm:"foreignKey:SalesChannelID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	FulfilmentStatus    SalesOrderFulfilmentStatus `gorm:"type:text;not null;default:draft;check:chk_sales_orders_fulfilment_status,fulfilment_status IN ('draft','confirmed','preparing','ready_to_ship','shipped','completed','cancelled')"`
	PaymentStatus       SalesOrderPaymentStatus    `gorm:"type:text;not null;default:unpaid;check:chk_sales_orders_payment_status,payment_status IN ('unpaid','paid','refunded')"`
	PackagingCostCents  int64                      `gorm:"not null;default:0;check:chk_sales_orders_packaging_cost,packaging_cost_cents >= 0"`
	ShippingCostCents   int64                      `gorm:"not null;default:0;check:chk_sales_orders_shipping_cost,shipping_cost_cents >= 0"`
	PlatformFeeCents    int64                      `gorm:"not null;default:0;check:chk_sales_orders_platform_fee,platform_fee_cents >= 0"`
	OtherCostCents      int64                      `gorm:"not null;default:0;check:chk_sales_orders_other_cost,other_cost_cents >= 0"`
	Notes               string                     `gorm:"not null;default:''"`
	ProductionCostCents *int64                     `gorm:"check:chk_sales_orders_production_cost_snapshot,production_cost_cents IS NULL OR production_cost_cents >= 0"`
	TotalCostCents      *int64                     `gorm:"check:chk_sales_orders_total_cost_snapshot,total_cost_cents IS NULL OR total_cost_cents >= 0"`
	NetProfitCents      *int64
	MarginBasisPoints   *int64
	CreatedAt           time.Time
	UpdatedAt           time.Time

	Items []SalesOrderItem `gorm:"foreignKey:SalesOrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (SalesOrder) TableName() string { return "sales_orders" }

func (order *SalesOrder) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&order.ID)
	return nil
}

func (status SalesOrderFulfilmentStatus) Valid() bool {
	switch status {
	case SalesOrderFulfilmentDraft,
		SalesOrderFulfilmentConfirmed,
		SalesOrderFulfilmentPreparing,
		SalesOrderFulfilmentReadyToShip,
		SalesOrderFulfilmentShipped,
		SalesOrderFulfilmentCompleted,
		SalesOrderFulfilmentCancelled:
		return true
	default:
		return false
	}
}

func (status SalesOrderPaymentStatus) Valid() bool {
	switch status {
	case SalesOrderPaymentUnpaid, SalesOrderPaymentPaid, SalesOrderPaymentRefunded:
		return true
	default:
		return false
	}
}
