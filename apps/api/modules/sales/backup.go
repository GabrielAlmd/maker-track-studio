package sales

import (
	"errors"
	"fmt"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type backupData struct {
	channels    []salesmodels.SalesChannel
	orders      []salesmodels.SalesOrder
	items       []salesmodels.SalesOrderItem
	allocations []salesmodels.SalesOrderAllocation
}

func (Module) ValidateBackup(sourceDB *gorm.DB, schemaVersion int) (any, error) {
	var data backupData
	if schemaVersion < 4 {
		return data, nil
	}
	for _, table := range []any{&salesmodels.SalesChannel{}, &salesmodels.SalesOrder{}, &salesmodels.SalesOrderItem{}, &salesmodels.SalesOrderAllocation{}} {
		if !sourceDB.Migrator().HasTable(table) {
			return data, errors.New("Backup is missing Sales data")
		}
	}

	if err := sourceDB.Order("created_at asc").Find(&data.channels).Error; err != nil {
		return data, errors.New("Backup Sales Channels could not be read")
	}
	if err := sourceDB.Order("created_at asc").Find(&data.orders).Error; err != nil {
		return data, errors.New("Backup Sales Orders could not be read")
	}
	if err := sourceDB.Order("created_at asc").Find(&data.items).Error; err != nil {
		return data, errors.New("Backup Sales Order Items could not be read")
	}
	if err := sourceDB.Order("created_at asc").Find(&data.allocations).Error; err != nil {
		return data, errors.New("Backup Sales Order Allocations could not be read")
	}

	var modelsList []models.Model
	if err := sourceDB.Find(&modelsList).Error; err != nil {
		return data, errors.New("Backup Models could not be read")
	}
	var printJobs []models.PrintJob
	if err := sourceDB.Find(&printJobs).Error; err != nil {
		return data, errors.New("Backup Print Jobs could not be read")
	}

	channelIDs := make(map[uuid.UUID]struct{}, len(data.channels))
	for _, channel := range data.channels {
		if channel.ID == uuid.Nil || strings.TrimSpace(channel.Name) == "" {
			return data, fmt.Errorf("Backup Sales Channel %s is invalid", channel.ID)
		}
		channelIDs[channel.ID] = struct{}{}
	}
	modelIDs := make(map[uuid.UUID]struct{}, len(modelsList))
	for _, model := range modelsList {
		modelIDs[model.ID] = struct{}{}
	}
	printJobIDs := make(map[uuid.UUID]struct{}, len(printJobs))
	for _, printJob := range printJobs {
		printJobIDs[printJob.ID] = struct{}{}
	}

	orderIDs := make(map[uuid.UUID]struct{}, len(data.orders))
	for _, order := range data.orders {
		if order.ID == uuid.Nil || strings.TrimSpace(order.Reference) == "" || order.OrderDate.IsZero() ||
			!order.FulfilmentStatus.Valid() || !order.PaymentStatus.Valid() ||
			order.PackagingCostCents < 0 || order.ShippingCostCents < 0 ||
			order.PlatformFeeCents < 0 || order.OtherCostCents < 0 ||
			(order.ProductionCostCents != nil && *order.ProductionCostCents < 0) ||
			(order.TotalCostCents != nil && *order.TotalCostCents < 0) {
			return data, fmt.Errorf("Backup Sales Order %s is invalid", order.ID)
		}
		if order.SalesChannelID != nil {
			if _, ok := channelIDs[*order.SalesChannelID]; !ok {
				return data, fmt.Errorf("Backup Sales Order %s references a missing Sales Channel", order.ID)
			}
		}
		orderIDs[order.ID] = struct{}{}
	}

	itemIDs := make(map[uuid.UUID]struct{}, len(data.items))
	for _, item := range data.items {
		if item.ID == uuid.Nil || strings.TrimSpace(item.ModelNameSnapshot) == "" ||
			item.Quantity <= 0 || item.UnitPriceCents < 0 {
			return data, fmt.Errorf("Backup Sales Order Item %s is invalid", item.ID)
		}
		if _, ok := orderIDs[item.SalesOrderID]; !ok {
			return data, fmt.Errorf("Backup Sales Order Item %s references a missing Sales Order", item.ID)
		}
		if item.ModelID != nil {
			if _, ok := modelIDs[*item.ModelID]; !ok {
				return data, fmt.Errorf("Backup Sales Order Item %s references a missing Model", item.ID)
			}
		}
		itemIDs[item.ID] = struct{}{}
	}

	for _, allocation := range data.allocations {
		if allocation.ID == uuid.Nil || allocation.UnitsAllocated <= 0 ||
			(allocation.ProductionCostCents != nil && *allocation.ProductionCostCents < 0) {
			return data, fmt.Errorf("Backup Sales Order Allocation %s is invalid", allocation.ID)
		}
		if _, ok := itemIDs[allocation.SalesOrderItemID]; !ok {
			return data, fmt.Errorf("Backup Sales Order Allocation %s references a missing Sales Order Item", allocation.ID)
		}
		if allocation.PrintJobID != nil {
			if _, ok := printJobIDs[*allocation.PrintJobID]; !ok {
				return data, fmt.Errorf("Backup Sales Order Allocation %s references a missing Print Job", allocation.ID)
			}
		}
	}

	return data, nil
}

func (Module) DeleteBackup(tx *gorm.DB) error {
	deleteSession := tx.Session(&gorm.Session{AllowGlobalUpdate: true})
	for _, table := range []any{&salesmodels.SalesOrderAllocation{}, &salesmodels.SalesOrderItem{}, &salesmodels.SalesOrder{}, &salesmodels.SalesChannel{}} {
		if err := deleteSession.Delete(table).Error; err != nil {
			return err
		}
	}
	return nil
}

func (Module) RestoreBackup(tx *gorm.DB, value any) error {
	data, ok := value.(backupData)
	if !ok {
		return errors.New("invalid Sales backup data")
	}
	if len(data.channels) > 0 {
		if err := tx.Create(&data.channels).Error; err != nil {
			return err
		}
	}
	if len(data.orders) > 0 {
		if err := tx.Create(&data.orders).Error; err != nil {
			return err
		}
	}
	if len(data.items) > 0 {
		if err := tx.Create(&data.items).Error; err != nil {
			return err
		}
	}
	if len(data.allocations) > 0 {
		if err := tx.Create(&data.allocations).Error; err != nil {
			return err
		}
	}
	return nil
}
