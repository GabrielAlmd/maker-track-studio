package saleshandlers

import (
	"errors"
	"fmt"
	corehandlers "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	errFormalOrder              = errors.New("Use the Business Operations amendment endpoint for a Business order")
	errProtectedLine            = errors.New("Use a distinct line ID belonging to this order and release affected allocations before replacing a model, removing its line, or reducing allocated demand")
	errSalesChannelNotFound     = errors.New("Sales Channel not found")
	errSalesChannelArchived     = errors.New("Sales Channel is archived")
	errSalesModelNotFound       = errors.New("Model not found")
	errSalesOrderNotEditable    = errors.New("Sales Order is not editable")
	errSalesStatusRequiresRoute = errors.New("Sales Order status requires the status endpoint")
	errSalesStatusConflict      = errors.New("Sales Order status transition is not allowed")
	errSalesDeleteConflict      = errors.New("Sales Order cannot be permanently deleted")
)

func loadSalesOrderDTO(db *gorm.DB, id uuid.UUID) (salesdto.SalesOrderDTO, error) {
	var order salesmodels.SalesOrder
	err := db.
		Preload("SalesChannel").
		Preload("Items", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("created_at ASC, id ASC")
		}).
		Preload("Items.Allocations", func(tx *gorm.DB) *gorm.DB { return tx.Order("created_at ASC, id ASC") }).
		Preload("Items.Allocations.PrintJob.Model").
		Preload("Items.Allocations.PrintJob.Filaments.Filament").
		First(&order, "id = ?", id).Error
	if err != nil {
		return salesdto.SalesOrderDTO{}, err
	}
	var cfg map[string]float64
	if order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCompleted {
		cfg, err = corehandlers.LoadProductionCostConfig(db)
		if err != nil {
			return salesdto.SalesOrderDTO{}, err
		}
	}
	profitability, err := calculateSalesOrderProfitability(order, cfg)
	if err != nil {
		return salesdto.SalesOrderDTO{}, err
	}
	return salesOrderDTO(order, profitability), nil
}

func createSalesOrder(db *gorm.DB, input salesdto.SalesOrderRequest) (salesdto.SalesOrderDTO, error) {
	var order salesmodels.SalesOrder
	err := db.Transaction(func(tx *gorm.DB) error {
		var channelID *uuid.UUID
		if input.SalesChannelID != nil {
			// Acquire SQLite's writer lock before checking channel state so an
			// archive cannot race with assignment to this new order.
			if err := tx.Exec(
				"UPDATE sales_channels SET is_archived = is_archived WHERE id = ?",
				*input.SalesChannelID,
			).Error; err != nil {
				return err
			}
			var channel salesmodels.SalesChannel
			if err := tx.First(&channel, "id = ?", *input.SalesChannelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errSalesChannelNotFound
				}
				return err
			}
			if channel.IsArchived {
				return errSalesChannelArchived
			}
			value := channel.ID
			channelID = &value
		}
		order = salesmodels.SalesOrder{
			Reference:          input.Reference,
			OrderDate:          input.OrderDate,
			SalesChannelID:     channelID,
			FulfilmentStatus:   input.FulfilmentStatus,
			PaymentStatus:      input.PaymentStatus,
			PackagingCostCents: input.PackagingCostCents,
			ShippingCostCents:  input.ShippingCostCents,
			PlatformFeeCents:   input.PlatformFeeCents,
			OtherCostCents:     input.OtherCostCents,
			Notes:              input.Notes,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		for index, requestedItem := range input.Items {
			var product models.Model
			if err := tx.First(&product, "id = ?", requestedItem.ModelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w at item %d", errSalesModelNotFound, index+1)
				}
				return err
			}
			modelID := product.ID
			item := salesmodels.SalesOrderItem{
				SalesOrderID:      order.ID,
				ModelID:           &modelID,
				ModelNameSnapshot: product.Name,
				Quantity:          requestedItem.Quantity,
				UnitPriceCents:    requestedItem.UnitPriceCents,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return salesdto.SalesOrderDTO{}, err
	}
	return loadSalesOrderDTO(db, order.ID)
}

func updateSalesOrder(db *gorm.DB, id uuid.UUID, input salesdto.SalesOrderRequest) (salesdto.SalesOrderDTO, error) {
	var saved salesdto.SalesOrderDTO
	err := db.Transaction(func(tx *gorm.DB) error {
		// Take SQLite's writer lock before reading lifecycle and reference state.
		if err := tx.Exec(
			"UPDATE sales_orders SET updated_at = updated_at WHERE id = ?",
			id,
		).Error; err != nil {
			return err
		}

		var order salesmodels.SalesOrder
		if err := tx.First(&order, "id = ?", id).Error; err != nil {
			return err
		}
		if order.IsV2() {
			return errFormalOrder
		}
		if order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentDraft {
			return errSalesOrderNotEditable
		}
		if input.FulfilmentStatus != order.FulfilmentStatus || input.PaymentStatus != order.PaymentStatus {
			return errSalesStatusRequiresRoute
		}

		var channelID *uuid.UUID
		if input.SalesChannelID != nil {
			if err := tx.Exec(
				"UPDATE sales_channels SET is_archived = is_archived WHERE id = ?",
				*input.SalesChannelID,
			).Error; err != nil {
				return err
			}
			var channel salesmodels.SalesChannel
			if err := tx.First(&channel, "id = ?", *input.SalesChannelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errSalesChannelNotFound
				}
				return err
			}
			// An archived historical Channel may remain on the same draft order,
			// but it cannot be newly assigned to another order.
			if channel.IsArchived && (order.SalesChannelID == nil || *order.SalesChannelID != channel.ID) {
				return errSalesChannelArchived
			}
			value := channel.ID
			channelID = &value
		}
		order.Reference = input.Reference
		order.OrderDate = input.OrderDate
		order.SalesChannelID = channelID
		order.PackagingCostCents = input.PackagingCostCents
		order.ShippingCostCents = input.ShippingCostCents
		order.PlatformFeeCents = input.PlatformFeeCents
		order.OtherCostCents = input.OtherCostCents
		order.Notes = input.Notes
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		submittedIDs, err := reconcileDraftItems(tx, order.ID, input.Items)
		if err != nil {
			return err
		}
		saved, err = loadSalesOrderDTO(tx, id)
		if err != nil {
			return err
		}
		// Legacy clients associate allocations with positions in the PUT response.
		byID := make(map[uuid.UUID]salesdto.SalesOrderItemDTO, len(saved.Items))
		for _, item := range saved.Items {
			byID[item.ID] = item
		}
		saved.Items = make([]salesdto.SalesOrderItemDTO, 0, len(submittedIDs))
		for _, id := range submittedIDs {
			item, ok := byID[id]
			if !ok {
				return errSalesAllocationConcurrentConflict
			}
			saved.Items = append(saved.Items, item)
		}
		return nil
	})
	return saved, err
}

func updateSalesOrderStatus(db *gorm.DB, id uuid.UUID, input salesdto.SalesOrderStatusRequest) (salesdto.SalesOrderDTO, error) {
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"UPDATE sales_orders SET updated_at = updated_at WHERE id = ?",
			id,
		).Error; err != nil {
			return err
		}
		var order salesmodels.SalesOrder
		if err := tx.First(&order, "id = ?", id).Error; err != nil {
			return err
		}
		if order.IsV2() {
			return errFormalOrder
		}
		targetFulfilment := order.FulfilmentStatus
		if input.FulfilmentStatus != nil {
			if !validFulfilmentTransition(order.FulfilmentStatus, *input.FulfilmentStatus) {
				return errSalesStatusConflict
			}
			targetFulfilment = *input.FulfilmentStatus
		}
		if input.PaymentStatus != nil {
			order.PaymentStatus = *input.PaymentStatus
		}
		if targetFulfilment == salesmodels.SalesOrderFulfilmentCompleted &&
			order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCompleted {
			return completeSalesOrder(tx, &order)
		}
		order.FulfilmentStatus = targetFulfilment
		return tx.Save(&order).Error
	})
	if err != nil {
		return salesdto.SalesOrderDTO{}, err
	}
	return loadSalesOrderDTO(db, id)
}

func deleteSalesOrder(db *gorm.DB, id uuid.UUID) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"UPDATE sales_orders SET updated_at = updated_at WHERE id = ?",
			id,
		).Error; err != nil {
			return err
		}
		var order salesmodels.SalesOrder
		if err := tx.First(&order, "id = ?", id).Error; err != nil {
			return err
		}
		if order.IsV2() {
			return errFormalOrder
		}
		if order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentDraft {
			return errSalesDeleteConflict
		}

		var itemIDs []uuid.UUID
		if err := tx.Model(&salesmodels.SalesOrderItem{}).
			Where("sales_order_id = ?", id).
			Pluck("id", &itemIDs).Error; err != nil {
			return err
		}
		if len(itemIDs) > 0 {
			if err := tx.Where("sales_order_item_id IN ?", itemIDs).
				Delete(&salesmodels.SalesOrderAllocation{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("sales_order_id = ?", id).Delete(&salesmodels.SalesOrderItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("sales_order_id = ?", id).Delete(&salesmodels.OrderAttachment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&order).Error
	})
}

// Older clients omit IDs. Reuse their matching model lines in historical order.
// Reserve explicit IDs first so a new row cannot claim another request row.
func reconcileDraftItems(tx *gorm.DB, orderID uuid.UUID, requests []salesdto.SalesOrderItemRequest) ([]uuid.UUID, error) {
	var rows []salesmodels.SalesOrderItem
	if err := tx.Preload("Allocations").Where("sales_order_id = ?", orderID).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]salesmodels.SalesOrderItem{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	reserved := map[uuid.UUID]bool{}
	for _, request := range requests {
		if request.ID != nil {
			id := *request.ID
			if _, ok := byID[id]; !ok || reserved[id] {
				return nil, errProtectedLine
			}
			reserved[id] = true
		}
	}
	ids := make([]uuid.UUID, 0, len(requests))
	for _, request := range requests {
		var row salesmodels.SalesOrderItem
		if request.ID != nil {
			row = byID[*request.ID]
		} else if len(reserved) == 0 {
			for _, candidate := range rows {
				if _, available := byID[candidate.ID]; available && candidate.ModelID != nil && *candidate.ModelID == request.ModelID {
					row = candidate
					break
				}
			}
		}
		var allocated int
		for _, a := range row.Allocations {
			allocated += a.UnitsAllocated
		}
		if allocated > request.Quantity {
			return nil, errProtectedLine
		}
		if row.ModelID == nil || *row.ModelID != request.ModelID {
			if allocated > 0 {
				return nil, errProtectedLine
			}
			var model models.Model
			if err := tx.First(&model, "id = ?", request.ModelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, errSalesModelNotFound
				}
				return nil, err
			}
			row.ModelID = &model.ID
			row.ModelNameSnapshot = model.Name
		}
		delete(byID, row.ID)
		row.SalesOrderID = orderID
		row.Quantity = request.Quantity
		row.UnitPriceCents = request.UnitPriceCents
		if err := tx.Omit("Allocations").Save(&row).Error; err != nil {
			return nil, err
		}
		ids = append(ids, row.ID)
	}
	for _, row := range byID {
		if len(row.Allocations) > 0 {
			return nil, errProtectedLine
		}
		if err := tx.Delete(&row).Error; err != nil {
			return nil, err
		}
	}
	return ids, nil
}
