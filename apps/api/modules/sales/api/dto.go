package api

import (
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"time"
)

// SalesChannelDTO is the stable API representation of an order source.
// Archived channels remain embedded in historical Sales Order responses.
type SalesChannelDTO struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Notes      string    `json:"notes"`
	IsArchived bool      `json:"is_archived"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SalesOrderItemDTO intentionally exposes the historical Model name snapshot
// separately from the optional live Model link.
type SalesOrderItemDTO struct {
	ID                uuid.UUID  `json:"id"`
	SalesOrderID      uuid.UUID  `json:"sales_order_id"`
	ModelID           *uuid.UUID `json:"model_id,omitempty"`
	ModelNameSnapshot string     `json:"model_name_snapshot"`
	Quantity          int        `json:"quantity"`
	UnitPriceCents    int64      `json:"unit_price_cents"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SalesOrderAllocationDTO is the API representation of production units
// assigned to an order item. PrintJob is nil only for legacy rows whose live
// Print Job reference is no longer available.
type SalesOrderAllocationDTO struct {
	ID                  uuid.UUID                      `json:"id"`
	SalesOrderItemID    uuid.UUID                      `json:"sales_order_item_id"`
	PrintJobID          *uuid.UUID                     `json:"print_job_id,omitempty"`
	UnitsAllocated      int                            `json:"units_allocated"`
	ProductionCostCents *int64                         `json:"production_cost_cents,omitempty"`
	PrintJob            *PrintJobAllocationCapacityDTO `json:"print_job,omitempty"`
	CreatedAt           time.Time                      `json:"created_at"`
	UpdatedAt           time.Time                      `json:"updated_at"`
}

// PrintJobAllocationCapacityDTO consistently reports capacity using active
// allocations. Allocations owned by cancelled orders are not active.
type PrintJobAllocationCapacityDTO struct {
	ID             uuid.UUID  `json:"id"`
	ModelID        *uuid.UUID `json:"model_id,omitempty"`
	UnitsPrinted   int        `json:"units_printed"`
	UnitsAllocated int64      `json:"units_allocated"`
	UnitsAvailable int64      `json:"units_available"`
}

type SalesOrderItemAllocationsDTO struct {
	SalesOrderItemID uuid.UUID                 `json:"sales_order_item_id"`
	Quantity         int                       `json:"quantity"`
	UnitsAllocated   int64                     `json:"units_allocated"`
	UnitsAvailable   int64                     `json:"units_available"`
	Allocations      []SalesOrderAllocationDTO `json:"allocations"`
}

type SalesOrderAllocationsDTO struct {
	SalesOrderID uuid.UUID                      `json:"sales_order_id"`
	Items        []SalesOrderItemAllocationsDTO `json:"items"`
}

type SalesOrderAllocationRequest struct {
	SalesOrderItemID uuid.UUID `json:"sales_order_item_id"`
	PrintJobID       uuid.UUID `json:"print_job_id"`
	UnitsAllocated   int       `json:"units_allocated"`
}

// SalesOrderAllocationsRequest is a complete replacement document. Treating
// the allocation set as one write keeps multi-item changes atomic.
type SalesOrderAllocationsRequest struct {
	Allocations []SalesOrderAllocationRequest `json:"allocations"`
}

type PrintJobAllocationUsageDTO struct {
	AllocationID     uuid.UUID `json:"allocation_id"`
	SalesOrderID     uuid.UUID `json:"sales_order_id"`
	SalesOrderItemID uuid.UUID `json:"sales_order_item_id"`
	OrderReference   string    `json:"order_reference"`
	UnitsAllocated   int       `json:"units_allocated"`
}

type PrintJobAllocationsDTO struct {
	PrintJob PrintJobAllocationCapacityDTO `json:"print_job"`
	UsedBy   []PrintJobAllocationUsageDTO  `json:"used_by"`
}

// SalesMarginDTO makes an undefined zero-revenue margin distinct from 0%.
type SalesMarginDTO struct {
	State   string   `json:"state"` // calculated, not_calculated
	Reason  string   `json:"reason,omitempty"`
	Percent *float64 `json:"percent,omitempty"`
}

// SalesOrderProfitabilityDTO keeps Sales money in integer cents. Values that
// depend on unresolved production cost remain nil rather than appearing as 0.
type SalesOrderProfitabilityDTO struct {
	State                string         `json:"state"` // calculated, not_calculated
	Reason               string         `json:"reason,omitempty"`
	RevenueCents         int64          `json:"revenue_cents"`
	AdditionalCostsCents int64          `json:"additional_costs_cents"`
	ProductionCostCents  *int64         `json:"production_cost_cents,omitempty"`
	TotalCostCents       *int64         `json:"total_cost_cents,omitempty"`
	NetProfitCents       *int64         `json:"net_profit_cents,omitempty"`
	Margin               SalesMarginDTO `json:"margin"`
}
type SalesOrderDTO struct {
	ID                 uuid.UUID                              `json:"id"`
	Reference          string                                 `json:"reference"`
	OrderDate          time.Time                              `json:"order_date"`
	SalesChannelID     *uuid.UUID                             `json:"sales_channel_id,omitempty"`
	SalesChannel       *SalesChannelDTO                       `json:"sales_channel,omitempty"`
	FulfilmentStatus   salesmodels.SalesOrderFulfilmentStatus `json:"fulfilment_status"`
	PaymentStatus      salesmodels.SalesOrderPaymentStatus    `json:"payment_status"`
	PackagingCostCents int64                                  `json:"packaging_cost_cents"`
	ShippingCostCents  int64                                  `json:"shipping_cost_cents"`
	PlatformFeeCents   int64                                  `json:"platform_fee_cents"`
	OtherCostCents     int64                                  `json:"other_cost_cents"`
	Notes              string                                 `json:"notes"`
	Items              []SalesOrderItemDTO                    `json:"items"`
	Profitability      SalesOrderProfitabilityDTO             `json:"profitability"`
	CreatedAt          time.Time                              `json:"created_at"`
	UpdatedAt          time.Time                              `json:"updated_at"`
}

// SalesSummaryRecentOrderDTO keeps the Dashboard payload compact while using
// the same Sales status and channel representations as the Orders workflow.
type SalesSummaryRecentOrderDTO struct {
	ID               uuid.UUID                              `json:"id"`
	Reference        string                                 `json:"reference"`
	OrderDate        time.Time                              `json:"order_date"`
	SalesChannel     *SalesChannelDTO                       `json:"sales_channel,omitempty"`
	FulfilmentStatus salesmodels.SalesOrderFulfilmentStatus `json:"fulfilment_status"`
	PaymentStatus    salesmodels.SalesOrderPaymentStatus    `json:"payment_status"`
	RevenueCents     int64                                  `json:"revenue_cents"`
}

// SalesSummaryDTO separates operational counters from completed-order
// financial history. All monetary values remain integer cents.
type SalesSummaryDTO struct {
	ActiveOrderCount    int64                        `json:"active_order_count"`
	CompletedOrderCount int64                        `json:"completed_order_count"`
	UnpaidOrderCount    int64                        `json:"unpaid_order_count"`
	RevenueCents        int64                        `json:"revenue_cents"`
	ProductionCostCents int64                        `json:"production_cost_cents"`
	PackagingCostCents  int64                        `json:"packaging_cost_cents"`
	ShippingCostCents   int64                        `json:"shipping_cost_cents"`
	PlatformFeeCents    int64                        `json:"platform_fee_cents"`
	OtherCostCents      int64                        `json:"other_cost_cents"`
	TotalCostCents      int64                        `json:"total_cost_cents"`
	NetProfitCents      int64                        `json:"net_profit_cents"`
	Margin              SalesMarginDTO               `json:"margin"`
	RecentOrders        []SalesSummaryRecentOrderDTO `json:"recent_orders"`
}

type SalesChannelRequest struct {
	Name  string `json:"name"`
	Notes string `json:"notes"`
}

// SalesOrderItemRequest excludes ModelNameSnapshot by design. The API always
// reads the current Model name and owns creation of the historical snapshot.
type SalesOrderItemRequest struct {
	ModelID        uuid.UUID `json:"model_id"`
	Quantity       int       `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
}

type SalesOrderRequest struct {
	Reference          string                                 `json:"reference"`
	OrderDate          time.Time                              `json:"order_date"`
	SalesChannelID     *uuid.UUID                             `json:"sales_channel_id"`
	FulfilmentStatus   salesmodels.SalesOrderFulfilmentStatus `json:"fulfilment_status"`
	PaymentStatus      salesmodels.SalesOrderPaymentStatus    `json:"payment_status"`
	PackagingCostCents int64                                  `json:"packaging_cost_cents"`
	ShippingCostCents  int64                                  `json:"shipping_cost_cents"`
	PlatformFeeCents   int64                                  `json:"platform_fee_cents"`
	OtherCostCents     int64                                  `json:"other_cost_cents"`
	Notes              string                                 `json:"notes"`
	Items              []SalesOrderItemRequest                `json:"items"`
}

// SalesOrderStatusRequest uses pointers so payment and fulfilment can be
// changed independently. At least one field is required by domain validation.
type SalesOrderStatusRequest struct {
	FulfilmentStatus *salesmodels.SalesOrderFulfilmentStatus `json:"fulfilment_status"`
	PaymentStatus    *salesmodels.SalesOrderPaymentStatus    `json:"payment_status"`
}
