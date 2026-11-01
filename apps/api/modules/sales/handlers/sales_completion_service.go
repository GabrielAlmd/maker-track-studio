package saleshandlers

import (
	"errors"
	"fmt"
	corehandlers "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"math"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	errSalesCompletionAllocationConflict = errors.New("Sales Order allocations are invalid for completion")
	errSalesCompletionCostUnavailable    = errors.New("Sales Order production cost is unavailable")
)

func completeSalesOrder(tx *gorm.DB, order *salesmodels.SalesOrder) error {
	var loaded salesmodels.SalesOrder
	if err := tx.
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC, id ASC")
		}).
		Preload("Items.Allocations", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC, id ASC")
		}).
		Preload("Items.Allocations.PrintJob.Model").
		Preload("Items.Allocations.PrintJob.Filaments.Filament").
		First(&loaded, "id = ?", order.ID).Error; err != nil {
		return err
	}
	loaded.PaymentStatus = order.PaymentStatus

	if err := validateSalesOrderAllocationsForCompletion(tx, loaded); err != nil {
		return err
	}
	cfg, err := corehandlers.LoadProductionCostConfig(tx)
	if err != nil {
		return err
	}
	profitability, allocationCosts, err := resolveLiveSalesOrderProfitability(loaded, cfg)
	if err != nil {
		return err
	}
	if profitability.State != "calculated" ||
		profitability.ProductionCostCents == nil ||
		profitability.TotalCostCents == nil ||
		profitability.NetProfitCents == nil {
		return fmt.Errorf("%w: %s", errSalesCompletionCostUnavailable, profitability.Reason)
	}

	var marginBasisPoints *int64
	if profitability.RevenueCents > 0 {
		value, ok := salesMarginBasisPoints(*profitability.NetProfitCents, profitability.RevenueCents)
		if !ok {
			return errSalesMoneyOverflow
		}
		marginBasisPoints = int64Pointer(value)
	}

	for allocationID, costCents := range allocationCosts {
		result := tx.Model(&salesmodels.SalesOrderAllocation{}).
			Where("id = ?", allocationID).
			Update("production_cost_cents", costCents)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errSalesCompletionAllocationConflict
		}
	}

	loaded.ProductionCostCents = int64Pointer(*profitability.ProductionCostCents)
	loaded.TotalCostCents = int64Pointer(*profitability.TotalCostCents)
	loaded.NetProfitCents = int64Pointer(*profitability.NetProfitCents)
	loaded.MarginBasisPoints = marginBasisPoints
	loaded.FulfilmentStatus = salesmodels.SalesOrderFulfilmentCompleted
	result := tx.Model(&salesmodels.SalesOrder{}).
		Where("id = ?", loaded.ID).
		Updates(map[string]any{
			"fulfilment_status":     loaded.FulfilmentStatus,
			"payment_status":        loaded.PaymentStatus,
			"production_cost_cents": loaded.ProductionCostCents,
			"total_cost_cents":      loaded.TotalCostCents,
			"net_profit_cents":      loaded.NetProfitCents,
			"margin_basis_points":   loaded.MarginBasisPoints,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	*order = loaded
	return nil
}

func validateSalesOrderAllocationsForCompletion(tx *gorm.DB, order salesmodels.SalesOrder) error {
	itemTotals := make(map[uuid.UUID]int64, len(order.Items))
	jobs := make(map[uuid.UUID]models.PrintJob)
	jobIDs := make([]uuid.UUID, 0)

	for _, item := range order.Items {
		for _, allocation := range item.Allocations {
			if allocation.ID == uuid.Nil || allocation.UnitsAllocated <= 0 ||
				allocation.PrintJobID == nil || allocation.PrintJob == nil {
				return errSalesCompletionAllocationConflict
			}
			job := *allocation.PrintJob
			if item.ModelID == nil || job.ModelID == nil || *item.ModelID != *job.ModelID {
				return errSalesAllocationModelConflict
			}
			units := int64(allocation.UnitsAllocated)
			if units > math.MaxInt64-itemTotals[item.ID] {
				return errSalesAllocationItemCapacity
			}
			itemTotals[item.ID] += units
			if itemTotals[item.ID] > int64(item.Quantity) {
				return errSalesAllocationItemCapacity
			}
			if _, exists := jobs[job.ID]; !exists {
				jobs[job.ID] = job
				jobIDs = append(jobIDs, job.ID)
			}
		}
	}

	activeTotals, err := activeAllocationTotals(tx, jobIDs, nil)
	if err != nil {
		return err
	}
	for jobID, job := range jobs {
		if activeTotals[jobID] > int64(job.UnitsPrinted) {
			return errSalesAllocationPrintJobCapacity
		}
	}
	return nil
}
