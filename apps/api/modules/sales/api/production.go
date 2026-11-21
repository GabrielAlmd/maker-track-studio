package api

import (
	"time"

	"github.com/google/uuid"
)

type OrderProductionLinkInput struct {
	SalesOrderItemID uuid.UUID `json:"sales_order_item_id"`
	PrintRunItemID   uuid.UUID `json:"print_run_item_id"`
	UnitsPlanned     int64     `json:"units_planned"`
	UnitsAllocated   int64     `json:"units_allocated"`
}

type OrderProductionRequest struct {
	ExpectedRevision *int64                     `json:"expected_revision"`
	Links            []OrderProductionLinkInput `json:"links"`
}

type ProductionSource struct {
	PrintRunItemID uuid.UUID  `json:"print_run_item_id"`
	PrintRunID     uuid.UUID  `json:"print_run_id"`
	PrintJobID     uuid.UUID  `json:"print_job_id"`
	ModelID        *uuid.UUID `json:"model_id"`
	PrintDate      time.Time  `json:"print_date"`
	Status         string     `json:"status"`
	UnitsPlanned   int64      `json:"units_planned"`
	UnitsAccepted  int64      `json:"units_accepted"`
	UnitsAvailable int64      `json:"units_available"`
}

type OrderProductionLink struct {
	OrderProductionLinkInput
	AllocationID *uuid.UUID       `json:"allocation_id,omitempty"`
	Source       ProductionSource `json:"source"`
}

type OrderProductionLine struct {
	SalesOrderItemID uuid.UUID             `json:"sales_order_item_id"`
	ModelName        string                `json:"model_name"`
	CanLink          bool                  `json:"can_link"`
	UnitsRequired    int64                 `json:"units_required"`
	UnitsPlanned     int64                 `json:"units_planned"`
	UnitsAccepted    int64                 `json:"units_accepted"`
	UnitsAllocated   int64                 `json:"units_allocated"`
	UnitsOutstanding int64                 `json:"units_outstanding"`
	Links            []OrderProductionLink `json:"links"`
}

type OrderProduction struct {
	Revision int64                 `json:"revision"`
	Editable bool                  `json:"editable"`
	Items    []OrderProductionLine `json:"items"`
}

type ProductionSources struct {
	Items      []ProductionSource `json:"items"`
	NextCursor *uuid.UUID         `json:"next_cursor,omitempty"`
}
