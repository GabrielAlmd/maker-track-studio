package saleshandlers

import (
	"fmt"
	"maps"
	"math"
	"slices"

	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type productionConflict string

func (err productionConflict) Error() string { return string(err) }

const (
	errProductionOrder    = productionConflict("Run-item production requires a proposal-backed order")
	errProductionEdit     = productionConflict("Production links can only be changed on confirmed or preparing orders")
	errProductionRevision = productionConflict("The production plan has changed. Reload it before saving")
)

const (
	productionNotStarted = "not_started"
	productionPlanned    = "planned"
	productionInProgress = "in_progress"
	productionComplete   = "complete"
	productionCancelled  = "cancelled"
)

type productionLinkKey struct {
	line, source uuid.UUID
}

type orderProductionState struct {
	order       storage.SalesOrder
	plans       map[productionLinkKey]storage.SalesProductionPlan
	allocations map[productionLinkKey]storage.SalesOrderAllocation
	sources     map[uuid.UUID]salesdto.ProductionSource
	links       map[productionLinkKey]salesdto.OrderProductionLinkInput
}

func productionEditable(order storage.SalesOrder) bool {
	return order.FulfilmentStatus == storage.SalesOrderFulfilmentConfirmed ||
		order.FulfilmentStatus == storage.SalesOrderFulfilmentPreparing
}

func loadOrderProduction(tx *gorm.DB, id uuid.UUID) (orderProductionState, error) {
	state := orderProductionState{
		plans:       make(map[productionLinkKey]storage.SalesProductionPlan),
		allocations: make(map[productionLinkKey]storage.SalesOrderAllocation),
		sources:     make(map[uuid.UUID]salesdto.ProductionSource),
		links:       make(map[productionLinkKey]salesdto.OrderProductionLinkInput),
	}
	if err := tx.Preload("Items", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at, id")
	}).Preload("Items.Allocations", "print_run_item_id IS NOT NULL").
		Take(&state.order, "id = ?", id).Error; err != nil {
		return state, err
	}
	if state.order.AcceptedProposalID == nil {
		return state, errProductionOrder
	}
	var plans []storage.SalesProductionPlan
	lines := tx.Model(&storage.SalesOrderItem{}).Select("id").Where("sales_order_id = ?", id)
	if err := tx.Where("sales_order_item_id IN (?)", lines).Find(&plans).Error; err != nil {
		return state, err
	}
	for _, plan := range plans {
		key := productionLinkKey{plan.SalesOrderItemID, plan.PrintRunItemID}
		state.plans[key] = plan
		state.links[key] = salesdto.OrderProductionLinkInput{
			SalesOrderItemID: key.line, PrintRunItemID: key.source, UnitsPlanned: plan.UnitsPlanned,
		}
	}
	for _, line := range state.order.Items {
		for _, allocation := range line.Allocations {
			key := productionLinkKey{line.ID, *allocation.PrintRunItemID}
			link := state.links[key]
			link.SalesOrderItemID, link.PrintRunItemID = key.line, key.source
			link.UnitsAllocated = int64(allocation.UnitsAllocated)
			state.links[key] = link
			state.allocations[key] = allocation
		}
	}
	var err error
	state.sources, err = findProductionSources(tx, state.links)
	return state, err
}

func findProductionSources(tx *gorm.DB, links map[productionLinkKey]salesdto.OrderProductionLinkInput) (map[uuid.UUID]salesdto.ProductionSource, error) {
	sources := make(map[uuid.UUID]salesdto.ProductionSource)
	if len(links) == 0 {
		return sources, nil
	}
	ids := make(map[uuid.UUID]struct{})
	for key := range links {
		ids[key.source] = struct{}{}
	}
	var rows []salesdto.ProductionSource
	if err := storage.ProductionSources(tx).Where("item.id IN ?", slices.Collect(maps.Keys(ids))).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, source := range rows {
		sources[source.PrintRunItemID] = source
	}
	if len(sources) != len(ids) {
		return nil, productionConflict("A linked run item is missing or its output is unverified")
	}
	return sources, nil
}

func (state orderProductionState) document() salesdto.OrderProduction {
	result := salesdto.OrderProduction{
		Status: productionNotStarted, Revision: state.order.ProductionRevision, Editable: productionEditable(state.order),
		Items: make([]salesdto.OrderProductionLine, 0, len(state.order.Items)),
	}
	cancelled := state.order.FulfilmentStatus == storage.SalesOrderFulfilmentCancelled
	allComplete := len(state.order.Items) > 0
	byLine := make(map[uuid.UUID][]salesdto.OrderProductionLink)
	for key, link := range state.links {
		source := state.sources[key.source]
		row := salesdto.OrderProductionLink{OrderProductionLinkInput: link, Source: source}
		if allocation, exists := state.allocations[key]; exists {
			row.AllocationID = &allocation.ID
		}
		byLine[key.line] = append(byLine[key.line], row)
	}
	for _, item := range state.order.Items {
		line := salesdto.OrderProductionLine{
			Status: productionNotStarted, SalesOrderItemID: item.ID, ModelName: item.ModelNameSnapshot,
			CanLink: item.ModelID != nil, UnitsRequired: int64(item.Quantity), Links: byLine[item.ID],
		}
		if line.Links == nil {
			line.Links = []salesdto.OrderProductionLink{}
		}
		slices.SortFunc(line.Links, func(a, b salesdto.OrderProductionLink) int {
			return slices.Compare(a.PrintRunItemID[:], b.PrintRunItemID[:])
		})
		for _, link := range line.Links {
			if cancelled {
				continue
			}
			switch link.Source.Status {
			case "planned":
				line.UnitsPlanned += link.UnitsPlanned
				if line.Status == productionNotStarted {
					line.Status = productionPlanned
				}
			case "in_progress", "completed":
				line.UnitsPlanned += link.UnitsPlanned
				line.Status = productionInProgress
			case "failed":
				line.Status = productionInProgress
			}
			if link.Source.Status == "completed" && link.UnitsAllocated <= link.Source.UnitsAccepted {
				line.UnitsAccepted += link.UnitsAllocated
			}
			line.UnitsAllocated += link.UnitsAllocated
		}
		line.UnitsOutstanding = max(0, line.UnitsRequired-line.UnitsAccepted)
		switch {
		case cancelled:
			line.Status = productionCancelled
		case line.UnitsRequired > 0 && line.UnitsOutstanding == 0:
			line.Status = productionComplete
		}
		allComplete = allComplete && line.Status == productionComplete
		switch line.Status {
		case productionComplete, productionInProgress:
			result.Status = productionInProgress
		case productionPlanned:
			if result.Status == productionNotStarted {
				result.Status = productionPlanned
			}
		}
		result.Items = append(result.Items, line)
	}
	switch {
	case cancelled:
		result.Status = productionCancelled
	case allComplete:
		result.Status = productionComplete
	}
	return result
}

func listOrderProductionSources(db *gorm.DB, orderID, lineID, cursor uuid.UUID) (salesdto.ProductionSources, error) {
	var line storage.SalesOrderItem
	if err := db.Joins("JOIN sales_orders AS orders ON orders.id = sales_order_items.sales_order_id").
		Where("orders.id = ? AND orders.accepted_proposal_id IS NOT NULL", orderID).
		Take(&line, "sales_order_items.id = ?", lineID).Error; err != nil {
		return salesdto.ProductionSources{}, err
	}
	result := salesdto.ProductionSources{Items: []salesdto.ProductionSource{}}
	if line.ModelID == nil {
		return result, nil
	}
	const pageSize = 50
	query := storage.ProductionSources(db).Where("item.model_id = ?", *line.ModelID).
		Where("(run.status IN ('planned', 'in_progress') OR (run.status = 'completed' AND item.accepted > 0))")
	if cursor != uuid.Nil {
		query = query.Where("item.id > ?", cursor)
	}
	if err := query.Order("item.id").Limit(pageSize + 1).Scan(&result.Items).Error; err != nil {
		return result, err
	}
	if len(result.Items) > pageSize {
		result.Items = result.Items[:pageSize]
		result.NextCursor = &result.Items[pageSize-1].PrintRunItemID
	}
	return result, nil
}

func replaceOrderProduction(db *gorm.DB, id uuid.UUID, expected int64, links map[productionLinkKey]salesdto.OrderProductionLinkInput) (salesdto.OrderProduction, error) {
	var result salesdto.OrderProduction
	err := db.Transaction(func(tx *gorm.DB) error {
		// Serialize against other orders and core production lifecycle writes.
		if err := tx.Exec("UPDATE sales_orders SET updated_at = updated_at WHERE id = ?", id).Error; err != nil {
			return err
		}
		state, err := loadOrderProduction(tx, id)
		if err != nil {
			return err
		}
		if !productionEditable(state.order) {
			return errProductionEdit
		}
		if maps.Equal(state.links, links) {
			result = state.document()
			return nil
		}
		if expected != state.order.ProductionRevision || expected == math.MaxInt64 {
			return errProductionRevision
		}
		sources, err := findProductionSources(tx, links)
		if err != nil {
			return err
		}
		if err := validateProductionLinks(tx, state, links, sources); err != nil {
			return err
		}
		if err := saveProductionLinks(tx, state, links, sources); err != nil {
			return err
		}
		// The preloaded items still contain the allocations that were just released.
		if err := tx.Model(&state.order).Omit(clause.Associations).Update("production_revision", expected+1).Error; err != nil {
			return err
		}
		state, err = loadOrderProduction(tx, id)
		if err == nil {
			result = state.document()
		}
		return err
	})
	return result, err
}

func validateProductionLinks(tx *gorm.DB, state orderProductionState, links map[productionLinkKey]salesdto.OrderProductionLinkInput, sources map[uuid.UUID]salesdto.ProductionSource) error {
	lines := make(map[uuid.UUID]storage.SalesOrderItem, len(state.order.Items))
	for _, item := range state.order.Items {
		lines[item.ID] = item
	}
	lineTotals := make(map[uuid.UUID]int64, len(lines))
	planned := make(map[uuid.UUID]int64, len(sources))
	allocated := make(map[uuid.UUID]int64, len(sources))
	for key, link := range links {
		item, exists := lines[key.line]
		source := sources[key.source]
		if !exists {
			return productionConflict("Production line does not belong to this order")
		}
		previous := state.links[key]
		if (item.ModelID == nil || source.ModelID == nil || *item.ModelID != *source.ModelID) &&
			(link.UnitsAllocated > previous.UnitsAllocated || link.UnitsPlanned > previous.UnitsPlanned) {
			return productionConflict("The order line and run item must reference the same model")
		}
		if source.Status != "planned" && source.Status != "in_progress" && link.UnitsPlanned > previous.UnitsPlanned {
			return productionConflict("Only planned or in-progress runs can receive new planning links")
		}
		lineTotals[key.line] += link.UnitsAllocated
		planned[key.source] += link.UnitsPlanned
		allocated[key.source] += link.UnitsAllocated
		if lineTotals[key.line] > int64(item.Quantity) {
			return productionConflict("Allocation exceeds the order line quantity")
		}
	}
	if len(sources) == 0 {
		return nil
	}
	otherPlans, err := productionSourceTotals(tx, "sales_production_plans", "units_planned", state.order.ID, sources)
	if err != nil {
		return err
	}
	otherAllocations, err := productionSourceTotals(tx, "sales_order_allocations", "units_allocated", state.order.ID, sources)
	if err != nil {
		return err
	}
	for id, source := range sources {
		if planned[id] > source.UnitsPlanned-otherPlans[id] {
			return productionConflict("Planning exceeds the run item's remaining planned quantity")
		}
		if allocated[id] > source.UnitsAccepted-otherAllocations[id] {
			return productionConflict("Allocation exceeds the run item's remaining verified accepted units")
		}
	}
	return nil
}

func productionSourceTotals(tx *gorm.DB, table, column string, orderID uuid.UUID, sources map[uuid.UUID]salesdto.ProductionSource) (map[uuid.UUID]int64, error) {
	var rows []struct {
		PrintRunItemID uuid.UUID
		Units          int64
	}
	// Table and column names come from the two fixed call sites.
	err := tx.Table(table+" AS link").Select(fmt.Sprintf("link.print_run_item_id, SUM(link.%s) AS units", column)).
		Joins("JOIN sales_order_items AS line ON line.id = link.sales_order_item_id").
		Joins("JOIN sales_orders AS orders ON orders.id = line.sales_order_id").
		Where("link.print_run_item_id IN ? AND orders.id <> ? AND orders.fulfilment_status <> ?",
			slices.Collect(maps.Keys(sources)), orderID, storage.SalesOrderFulfilmentCancelled).
		Group("link.print_run_item_id").Scan(&rows).Error
	totals := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		totals[row.PrintRunItemID] = row.Units
	}
	return totals, err
}

func saveProductionLinks(tx *gorm.DB, state orderProductionState, links map[productionLinkKey]salesdto.OrderProductionLinkInput, sources map[uuid.UUID]salesdto.ProductionSource) error {
	for key, plan := range state.plans {
		if links[key].UnitsPlanned == 0 {
			if err := tx.Delete(&plan).Error; err != nil {
				return err
			}
		}
	}
	for key, allocation := range state.allocations {
		if links[key].UnitsAllocated == 0 {
			if err := tx.Delete(&allocation).Error; err != nil {
				return err
			}
		}
	}
	for key, link := range links {
		prior := state.links[key]
		if link.UnitsPlanned > 0 && link.UnitsPlanned != prior.UnitsPlanned {
			plan := storage.SalesProductionPlan{SalesOrderItemID: key.line, PrintRunItemID: key.source, UnitsPlanned: link.UnitsPlanned}
			if err := tx.Save(&plan).Error; err != nil {
				return err
			}
		}
		if link.UnitsAllocated > 0 && link.UnitsAllocated != prior.UnitsAllocated {
			if allocation, exists := state.allocations[key]; exists {
				if err := tx.Model(&allocation).Update("units_allocated", link.UnitsAllocated).Error; err != nil {
					return err
				}
			} else {
				jobID := sources[key.source].PrintJobID
				allocation := storage.SalesOrderAllocation{
					SalesOrderItemID: key.line, PrintRunItemID: &key.source,
					PrintJobID: &jobID, UnitsAllocated: int(link.UnitsAllocated),
				}
				if err := tx.Create(&allocation).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}
