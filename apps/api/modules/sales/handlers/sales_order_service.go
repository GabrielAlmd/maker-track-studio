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
		var existingItemIDs []uuid.UUID
		if err := tx.Model(&salesmodels.SalesOrderItem{}).
			Where("sales_order_id = ?", order.ID).
			Pluck("id", &existingItemIDs).Error; err != nil {
			return err
		}
		if len(existingItemIDs) > 0 {
			if err := tx.Where("sales_order_item_id IN ?", existingItemIDs).
				Delete(&salesmodels.SalesOrderAllocation{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("sales_order_id = ?", order.ID).Delete(&salesmodels.SalesOrderItem{}).Error; err != nil {
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
	return loadSalesOrderDTO(db, id)
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
		return tx.Delete(&order).Error
	})
}
