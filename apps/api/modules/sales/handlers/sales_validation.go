package saleshandlers

import (
	"fmt"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxSalesChannelNameRunes = 100
	maxSalesReferenceRunes   = 200
	maxSalesNotesRunes       = 2000
	maxSalesOrderItems       = 100
)

// validateSalesChannelRequest normalizes user-entered text before it reaches
// persistence while keeping archive state server-owned.
func validateSalesChannelRequest(input *salesdto.SalesChannelRequest) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxSalesChannelNameRunes {
		return fmt.Errorf("sales channel name must contain 1 to %d characters", maxSalesChannelNameRunes)
	}
	if utf8.RuneCountInString(input.Notes) > maxSalesNotesRunes {
		return fmt.Errorf("sales channel notes must not exceed %d characters", maxSalesNotesRunes)
	}
	return nil
}

// validateSalesOrderRequest handles request-local invariants. Referenced rows
// and their lifecycle state are checked inside the same transaction as writes.
func validateSalesOrderRequest(input *salesdto.SalesOrderRequest) error {
	input.Reference = strings.TrimSpace(input.Reference)
	input.Notes = strings.TrimSpace(input.Notes)
	if input.Reference == "" || utf8.RuneCountInString(input.Reference) > maxSalesReferenceRunes {
		return fmt.Errorf("order reference must contain 1 to %d characters", maxSalesReferenceRunes)
	}
	if input.OrderDate.IsZero() {
		return fmt.Errorf("order date is required")
	}
	if input.SalesChannelID != nil && *input.SalesChannelID == uuid.Nil {
		return fmt.Errorf("sales channel ID must be a valid UUID")
	}
	if !input.FulfilmentStatus.Valid() {
		return fmt.Errorf("unsupported fulfilment status")
	}
	if !input.PaymentStatus.Valid() {
		return fmt.Errorf("unsupported payment status")
	}
	if input.PackagingCostCents < 0 || input.ShippingCostCents < 0 ||
		input.PlatformFeeCents < 0 || input.OtherCostCents < 0 {
		return fmt.Errorf("order costs must be non-negative integer cents")
	}
	additionalCosts := int64(0)
	for _, value := range []int64{
		input.PackagingCostCents,
		input.ShippingCostCents,
		input.PlatformFeeCents,
		input.OtherCostCents,
	} {
		var ok bool
		additionalCosts, ok = checkedAddInt64(additionalCosts, value)
		if !ok {
			return fmt.Errorf("order costs exceed the supported integer-cent range")
		}
	}
	if utf8.RuneCountInString(input.Notes) > maxSalesNotesRunes {
		return fmt.Errorf("order notes must not exceed %d characters", maxSalesNotesRunes)
	}
	if len(input.Items) == 0 || len(input.Items) > maxSalesOrderItems {
		return fmt.Errorf("an order must contain between 1 and %d items", maxSalesOrderItems)
	}
	revenue := int64(0)
	for index, item := range input.Items {
		if item.ModelID == uuid.Nil {
			return fmt.Errorf("item %d requires a valid Model ID", index+1)
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("item %d quantity must be greater than zero", index+1)
		}
		if item.UnitPriceCents < 0 {
			return fmt.Errorf("item %d unit price must be non-negative integer cents", index+1)
		}
		line, ok := checkedMultiplyNonnegative(item.UnitPriceCents, int64(item.Quantity))
		if !ok {
			return fmt.Errorf("item %d revenue exceeds the supported integer-cent range", index+1)
		}
		revenue, ok = checkedAddInt64(revenue, line)
		if !ok {
			return fmt.Errorf("order revenue exceeds the supported integer-cent range")
		}
	}
	return nil
}

func validateSalesOrderStatusRequest(input salesdto.SalesOrderStatusRequest) error {
	if input.FulfilmentStatus == nil && input.PaymentStatus == nil {
		return fmt.Errorf("at least one status is required")
	}
	if input.FulfilmentStatus != nil && !input.FulfilmentStatus.Valid() {
		return fmt.Errorf("unsupported fulfilment status")
	}
	if input.PaymentStatus != nil && !input.PaymentStatus.Valid() {
		return fmt.Errorf("unsupported payment status")
	}
	return nil
}

// validFulfilmentTransition centralizes the lifecycle graph so later
// completion/allocation rules can be added without rewriting HTTP handlers.
func validFulfilmentTransition(from, to salesmodels.SalesOrderFulfilmentStatus) bool {
	if from == to {
		return true
	}
	if from == salesmodels.SalesOrderFulfilmentCancelled || from == salesmodels.SalesOrderFulfilmentCompleted {
		return false
	}
	if to == salesmodels.SalesOrderFulfilmentCancelled {
		return true
	}
	allowed := map[salesmodels.SalesOrderFulfilmentStatus]salesmodels.SalesOrderFulfilmentStatus{
		salesmodels.SalesOrderFulfilmentDraft:       salesmodels.SalesOrderFulfilmentConfirmed,
		salesmodels.SalesOrderFulfilmentConfirmed:   salesmodels.SalesOrderFulfilmentPreparing,
		salesmodels.SalesOrderFulfilmentPreparing:   salesmodels.SalesOrderFulfilmentReadyToShip,
		salesmodels.SalesOrderFulfilmentReadyToShip: salesmodels.SalesOrderFulfilmentShipped,
		salesmodels.SalesOrderFulfilmentShipped:     salesmodels.SalesOrderFulfilmentCompleted,
	}
	return allowed[from] == to
}
