package saleshandlers

import (
	"errors"
	"fmt"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"math"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	errSalesAllocationItemConflict       = errors.New("Sales Order Item does not belong to this Sales Order")
	errSalesAllocationPrintJobNotFound   = errors.New("Print Job not found")
	errSalesAllocationModelConflict      = errors.New("Sales Order Item and Print Job Models do not match")
	errSalesAllocationItemCapacity       = errors.New("Sales Order Item allocation exceeds ordered quantity")
	errSalesAllocationPrintJobCapacity   = errors.New("Print Job allocation exceeds units printed")
	errSalesAllocationsNotEditable       = errors.New("Sales Order allocations are not editable")
	errSalesAllocationConcurrentConflict = errors.New("Sales allocation changed concurrently")
)

type allocationTotalRow struct {
	PrintJobID     uuid.UUID
	UnitsAllocated int64
}

func activeAllocationTotals(db *gorm.DB, printJobIDs []uuid.UUID, excludeOrderID *uuid.UUID) (map[uuid.UUID]int64, error) {
	totals := make(map[uuid.UUID]int64, len(printJobIDs))
	if len(printJobIDs) == 0 {
		return totals, nil
	}

	query := db.Table("sales_order_allocations AS allocations").
		Select("allocations.print_job_id AS print_job_id, SUM(allocations.units_allocated) AS units_allocated").
		Joins("JOIN sales_order_items AS items ON items.id = allocations.sales_order_item_id").
		Joins("JOIN sales_orders AS orders ON orders.id = items.sales_order_id").
		Where("allocations.print_job_id IN ?", printJobIDs).
		Where("orders.fulfilment_status <> ?", salesmodels.SalesOrderFulfilmentCancelled)
	if excludeOrderID != nil {
		query = query.Where("orders.id <> ?", *excludeOrderID)
	}

	var rows []allocationTotalRow
	if err := query.Group("allocations.print_job_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		totals[row.PrintJobID] = row.UnitsAllocated
	}
	return totals, nil
}

func allocationCapacityDTO(job models.PrintJob, allocated int64) salesdto.PrintJobAllocationCapacityDTO {
	return salesdto.PrintJobAllocationCapacityDTO{
		ID:             job.ID,
		ModelID:        job.ModelID,
		UnitsPrinted:   job.UnitsPrinted,
		UnitsAllocated: allocated,
		UnitsAvailable: int64(job.UnitsPrinted) - allocated,
	}
}

func loadSalesOrderAllocationsDTO(db *gorm.DB, orderID uuid.UUID) (salesdto.SalesOrderAllocationsDTO, error) {
	var order salesmodels.SalesOrder
	if err := db.Preload("Items", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("created_at ASC, id ASC")
	}).Preload("Items.Allocations", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("created_at ASC, id ASC")
	}).First(&order, "id = ?", orderID).Error; err != nil {
		return salesdto.SalesOrderAllocationsDTO{}, err
	}

	jobIDs := make([]uuid.UUID, 0)
	seenJobs := make(map[uuid.UUID]struct{})
	for _, item := range order.Items {
		for _, allocation := range item.Allocations {
			if allocation.PrintJobID == nil {
				continue
			}
			if _, exists := seenJobs[*allocation.PrintJobID]; !exists {
				seenJobs[*allocation.PrintJobID] = struct{}{}
				jobIDs = append(jobIDs, *allocation.PrintJobID)
			}
		}
	}

	jobs := make(map[uuid.UUID]models.PrintJob, len(jobIDs))
	if len(jobIDs) > 0 {
		var records []models.PrintJob
		if err := db.Where("id IN ?", jobIDs).Find(&records).Error; err != nil {
			return salesdto.SalesOrderAllocationsDTO{}, err
		}
		for _, job := range records {
			jobs[job.ID] = job
		}
	}
	totals, err := activeAllocationTotals(db, jobIDs, nil)
	if err != nil {
		return salesdto.SalesOrderAllocationsDTO{}, err
	}

	result := salesdto.SalesOrderAllocationsDTO{
		SalesOrderID: order.ID,
		Items:        make([]salesdto.SalesOrderItemAllocationsDTO, 0, len(order.Items)),
	}
	for _, item := range order.Items {
		itemDTO := salesdto.SalesOrderItemAllocationsDTO{
			SalesOrderItemID: item.ID,
			Quantity:         item.Quantity,
			Allocations:      make([]salesdto.SalesOrderAllocationDTO, 0, len(item.Allocations)),
		}
		for _, allocation := range item.Allocations {
			allocationDTO := salesdto.SalesOrderAllocationDTO{
				ID:                  allocation.ID,
				SalesOrderItemID:    allocation.SalesOrderItemID,
				PrintJobID:          allocation.PrintJobID,
				UnitsAllocated:      allocation.UnitsAllocated,
				ProductionCostCents: allocation.ProductionCostCents,
				CreatedAt:           allocation.CreatedAt,
				UpdatedAt:           allocation.UpdatedAt,
			}
			itemDTO.UnitsAllocated += int64(allocation.UnitsAllocated)
			if allocation.PrintJobID != nil {
				if job, exists := jobs[*allocation.PrintJobID]; exists {
					capacity := allocationCapacityDTO(job, totals[job.ID])
					allocationDTO.PrintJob = &capacity
				}
			}
			itemDTO.Allocations = append(itemDTO.Allocations, allocationDTO)
		}
		itemDTO.UnitsAvailable = int64(item.Quantity) - itemDTO.UnitsAllocated
		result.Items = append(result.Items, itemDTO)
	}
	return result, nil
}

func replaceSalesOrderAllocations(db *gorm.DB, orderID uuid.UUID, input salesdto.SalesOrderAllocationsRequest) (salesdto.SalesOrderAllocationsDTO, error) {
	err := db.Transaction(func(tx *gorm.DB) error {
		// SQLite permits one writer. Taking its writer lock before any capacity
		// read serializes allocation checks across different Sales Orders too.
		if err := tx.Exec(
			"UPDATE sales_orders SET updated_at = updated_at WHERE id = ?",
			orderID,
		).Error; err != nil {
			return err
		}

		var order salesmodels.SalesOrder
		if err := tx.First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}
		if order.FulfilmentStatus == salesmodels.SalesOrderFulfilmentCompleted ||
			order.FulfilmentStatus == salesmodels.SalesOrderFulfilmentCancelled {
			return errSalesAllocationsNotEditable
		}

		var items []salesmodels.SalesOrderItem
		if err := tx.Where("sales_order_id = ?", orderID).Find(&items).Error; err != nil {
			return err
		}
		itemsByID := make(map[uuid.UUID]salesmodels.SalesOrderItem, len(items))
		itemIDs := make([]uuid.UUID, 0, len(items))
		for _, item := range items {
			itemsByID[item.ID] = item
			itemIDs = append(itemIDs, item.ID)
		}

		jobIDs := make([]uuid.UUID, 0, len(input.Allocations))
		seenJobs := make(map[uuid.UUID]struct{}, len(input.Allocations))
		for _, requested := range input.Allocations {
			if _, exists := itemsByID[requested.SalesOrderItemID]; !exists {
				return errSalesAllocationItemConflict
			}
			if _, exists := seenJobs[requested.PrintJobID]; !exists {
				seenJobs[requested.PrintJobID] = struct{}{}
				jobIDs = append(jobIDs, requested.PrintJobID)
			}
		}

		jobsByID := make(map[uuid.UUID]models.PrintJob, len(jobIDs))
		if len(jobIDs) > 0 {
			var jobs []models.PrintJob
			if err := tx.Where("id IN ?", jobIDs).Find(&jobs).Error; err != nil {
				return err
			}
			for _, job := range jobs {
				jobsByID[job.ID] = job
			}
			if len(jobsByID) != len(jobIDs) {
				return errSalesAllocationPrintJobNotFound
			}
		}

		itemTotals := make(map[uuid.UUID]int64, len(items))
		jobTotals := make(map[uuid.UUID]int64, len(jobIDs))
		for _, requested := range input.Allocations {
			item := itemsByID[requested.SalesOrderItemID]
			job := jobsByID[requested.PrintJobID]
			if item.ModelID == nil || job.ModelID == nil || *item.ModelID != *job.ModelID {
				return errSalesAllocationModelConflict
			}
			units := int64(requested.UnitsAllocated)
			if units > math.MaxInt64-itemTotals[item.ID] || units > math.MaxInt64-jobTotals[job.ID] {
				return errSalesAllocationItemCapacity
			}
			itemTotals[item.ID] += units
			jobTotals[job.ID] += units
			if itemTotals[item.ID] > int64(item.Quantity) {
				return errSalesAllocationItemCapacity
			}
		}

		otherTotals, err := activeAllocationTotals(tx, jobIDs, &orderID)
		if err != nil {
			return err
		}
		for jobID, requestedUnits := range jobTotals {
			job := jobsByID[jobID]
			if requestedUnits > math.MaxInt64-otherTotals[jobID] ||
				otherTotals[jobID]+requestedUnits > int64(job.UnitsPrinted) {
				return errSalesAllocationPrintJobCapacity
			}
		}

		if len(itemIDs) > 0 {
			if err := tx.Where("sales_order_item_id IN ?", itemIDs).
				Delete(&salesmodels.SalesOrderAllocation{}).Error; err != nil {
				return err
			}
		}
		for _, requested := range input.Allocations {
			printJobID := requested.PrintJobID
			allocation := salesmodels.SalesOrderAllocation{
				SalesOrderItemID: requested.SalesOrderItemID,
				PrintJobID:       &printJobID,
				UnitsAllocated:   requested.UnitsAllocated,
			}
			if err := tx.Create(&allocation).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if isSQLiteAllocationLockError(err) {
			return salesdto.SalesOrderAllocationsDTO{}, errSalesAllocationConcurrentConflict
		}
		return salesdto.SalesOrderAllocationsDTO{}, err
	}
	return loadSalesOrderAllocationsDTO(db, orderID)
}

func isSQLiteAllocationLockError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked")
}

func loadPrintJobAllocationsDTO(db *gorm.DB, printJobID uuid.UUID) (salesdto.PrintJobAllocationsDTO, error) {
	var job models.PrintJob
	if err := db.First(&job, "id = ?", printJobID).Error; err != nil {
		return salesdto.PrintJobAllocationsDTO{}, err
	}

	totals, err := activeAllocationTotals(db, []uuid.UUID{printJobID}, nil)
	if err != nil {
		return salesdto.PrintJobAllocationsDTO{}, err
	}

	type usageRow struct {
		AllocationID     uuid.UUID
		SalesOrderID     uuid.UUID
		SalesOrderItemID uuid.UUID
		OrderReference   string
		UnitsAllocated   int
	}
	var rows []usageRow
	if err := db.Table("sales_order_allocations AS allocations").
		Select(`allocations.id AS allocation_id,
			orders.id AS sales_order_id,
			items.id AS sales_order_item_id,
			orders.reference AS order_reference,
			allocations.units_allocated AS units_allocated`).
		Joins("JOIN sales_order_items AS items ON items.id = allocations.sales_order_item_id").
		Joins("JOIN sales_orders AS orders ON orders.id = items.sales_order_id").
		Where("allocations.print_job_id = ?", printJobID).
		Where("orders.fulfilment_status <> ?", salesmodels.SalesOrderFulfilmentCancelled).
		Order("orders.order_date ASC, orders.id ASC, allocations.created_at ASC, allocations.id ASC").
		Scan(&rows).Error; err != nil {
		return salesdto.PrintJobAllocationsDTO{}, err
	}

	result := salesdto.PrintJobAllocationsDTO{
		PrintJob: allocationCapacityDTO(job, totals[printJobID]),
		UsedBy:   make([]salesdto.PrintJobAllocationUsageDTO, 0, len(rows)),
	}
	for _, row := range rows {
		result.UsedBy = append(result.UsedBy, salesdto.PrintJobAllocationUsageDTO{
			AllocationID:     row.AllocationID,
			SalesOrderID:     row.SalesOrderID,
			SalesOrderItemID: row.SalesOrderItemID,
			OrderReference:   row.OrderReference,
			UnitsAllocated:   row.UnitsAllocated,
		})
	}
	return result, nil
}

func allocationPairKey(itemID, printJobID uuid.UUID) string {
	return fmt.Sprintf("%s:%s", itemID, printJobID)
}
