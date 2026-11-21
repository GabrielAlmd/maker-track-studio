package saleshandlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fulfilmentEvent struct {
	inventory.Movement
	Remaining int64 `json:"remaining"`
}
type fulfilmentLine struct {
	Allocation uuid.UUID         `json:"allocation_id"`
	Product    uuid.UUID         `json:"product_id"`
	RunItem    uuid.UUID         `json:"run_item_id"`
	Name       string            `json:"name"`
	Revision   int64             `json:"revision"`
	Reservable int64             `json:"reservable"`
	Reserved   int64             `json:"reserved"`
	Shipped    int64             `json:"shipped"`
	Returned   int64             `json:"returned"`
	Events     []fulfilmentEvent `json:"events"`
}
type fulfilment struct {
	CanCancel bool             `json:"can_cancel"`
	Status    string           `json:"status"`
	Cancelled bool             `json:"cancelled"`
	Lines     []fulfilmentLine `json:"lines"`
}

func projectAllocation(a storage.SalesOrderAllocation, name string, lot inventory.OutputLot) fulfilmentLine {
	line := fulfilmentLine{Allocation: a.ID, RunItem: *a.PrintRunItemID, Name: name, Events: []fulfilmentEvent{}}
	indices := map[string]int{}
	line.Revision = lot.Revision
	for _, row := range lot.Movements {
		line.Product = row.ItemID
		if row.Actor != storage.FulfilmentActor {
			continue
		}
		event := fulfilmentEvent{Movement: row}
		switch row.Kind {
		case "reserve":
			if row.Reference != a.ID.String() {
				continue
			}
			line.Reserved += row.Units
			event.Remaining = row.Units
		case "release", "dispatch", "restock", "reject_return":
			parent, ok := indices[row.Reference]
			if !ok {
				continue
			}
			line.Events[parent].Remaining -= row.Units
			switch row.Kind {
			case "dispatch":
				line.Shipped += row.Units
				event.Remaining = row.Units
				line.Reserved -= row.Units
			case "release":
				line.Reserved -= row.Units
			default:
				line.Returned += row.Units
			}
		default:
			continue
		}
		indices[row.OperationID.String()] = len(line.Events)
		line.Events = append(line.Events, event)
	}
	line.Reservable = max(0, min(lot.Available, int64(a.UnitsAllocated)-line.Reserved-line.Shipped))
	return line
}

func orderFulfilment(tx *gorm.DB, id uuid.UUID) (fulfilment, error) {
	var order storage.SalesOrder
	doc := fulfilment{Lines: []fulfilmentLine{}, Status: "unfulfilled"}
	err := tx.Preload("Items", func(q *gorm.DB) *gorm.DB { return q.Order("id") }).
		Preload("Items.Allocations", func(q *gorm.DB) *gorm.DB { return q.Where("print_run_item_id IS NOT NULL").Order("id") }).
		Take(&order, "id = ?", id).Error
	if err != nil {
		return doc, err
	}
	if order.AcceptedProposalID == nil {
		return doc, errProductionOrder
	}
	if !productionEditable(order) && order.FulfilmentStatus != storage.SalesOrderFulfilmentCancelled {
		return doc, productionConflict("This order is protected from fulfilment changes")
	}
	doc.Cancelled = order.FulfilmentStatus == storage.SalesOrderFulfilmentCancelled
	var ids []uuid.UUID
	for _, item := range order.Items {
		for _, a := range item.Allocations {
			ids = append(ids, *a.PrintRunItemID)
		}
	}
	history, err := inventory.OutputLots(tx, ids)
	if err != nil {
		return doc, err
	}
	var shipped, demand, returned int64
	for _, item := range order.Items {
		demand += int64(item.Quantity)
		for _, a := range item.Allocations {
			line := projectAllocation(a, item.ModelNameSnapshot, history[*a.PrintRunItemID])
			shipped += line.Shipped
			returned += line.Returned
			doc.Lines = append(doc.Lines, line)
		}
	}
	doc.CanCancel = !doc.Cancelled && shipped < demand
	switch {
	case returned > 0 && returned == shipped:
		doc.Status = "returned"
	case returned > 0:
		doc.Status = "partially_returned"
	case demand > 0 && shipped == demand:
		doc.Status = "shipped"
	case shipped > 0:
		doc.Status = "partially_shipped"
	}
	return doc, nil
}

type fulfilmentInput struct {
	Allocation uuid.UUID `json:"allocation_id"`
	Operation  uuid.UUID `json:"operation_id"`
	Source     uuid.UUID `json:"source_id"`
	Reference  uuid.UUID `json:"reference"`
	Kind       string    `json:"kind"`
	Reason     string    `json:"reason"`
	Units      int64     `json:"units"`
	Revision   int64     `json:"expected_revision"`
}

func (input fulfilmentInput) apply(tx *gorm.DB, doc fulfilment) (inventory.Receipt, error) {
	for _, line := range doc.Lines {
		if line.Allocation != input.Allocation {
			continue
		}
		refOK := input.Kind == "reserve" && input.Reference == line.Allocation
		replay := false
		for _, e := range line.Events {
			if e.OperationID == input.Operation {
				replay = true
			}
			if e.OperationID != input.Reference {
				continue
			}
			switch input.Kind {
			case "dispatch", "release":
				refOK = e.Kind == "reserve"
			case "restock", "reject_return":
				refOK = e.Kind == "dispatch"
			}
		}
		returning := input.Kind == "restock" || input.Kind == "reject_return"
		if !refOK || !replay && (doc.Cancelled && !returning || input.Kind == "reserve" && input.Units > line.Reservable) {
			return inventory.Receipt{}, inventory.ErrConflict
		}
		return inventory.ApplyFulfilment(tx, inventory.FulfilmentCommand{OperationID: input.Operation, SourceID: input.Source, RunItemID: line.RunItem, ProductID: line.Product,
			Reference: input.Reference, Kind: input.Kind, Reason: input.Reason, Units: input.Units, ExpectedRevision: input.Revision, Actor: storage.FulfilmentActor})
	}
	return inventory.Receipt{}, gorm.ErrRecordNotFound
}

func cancelRemainder(tx *gorm.DB, id uuid.UUID, doc fulfilment) error {
	if doc.Cancelled {
		return nil
	}
	if !doc.CanCancel {
		return productionConflict("This order has no unshipped remainder")
	}
	revisions := map[uuid.UUID]int64{}
	for _, line := range doc.Lines {
		for _, e := range line.Events {
			if e.Kind != "reserve" || e.Remaining == 0 {
				continue
			}
			op := uuid.NewSHA1(e.OperationID, []byte("cancel remainder"))
			receipt, err := inventory.ApplyFulfilment(tx, inventory.FulfilmentCommand{OperationID: op, SourceID: op, ProductID: line.Product, RunItemID: line.RunItem,
				Reference: e.OperationID, Kind: "release", Reason: "Order remainder cancelled", Actor: storage.FulfilmentActor, Units: e.Remaining, ExpectedRevision: max(line.Revision, revisions[line.RunItem])})
			if err != nil {
				return err
			}
			revisions[line.RunItem] = receipt.Sequence
		}
	}
	return tx.Model(&storage.SalesOrder{}).Where("id = ?", id).Update("fulfilment_status", storage.SalesOrderFulfilmentCancelled).Error
}

func OrderFulfilment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		var input fulfilmentInput
		if c.Request.Method == http.MethodPost {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
			if err := c.ShouldBindJSON(&input); err != nil || input.Operation == uuid.Nil || input.Allocation == uuid.Nil || input.Reference == uuid.Nil ||
				input.Revision < 1 || input.Units < 1 || input.Units > maxProductionUnits || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 512 {
				c.JSON(400, gin.H{"error": "Provide valid event IDs, revision, units and reason"})
				return
			}
			if input.Source == uuid.Nil {
				input.Source = input.Operation
			}
			switch input.Kind {
			case "reserve", "dispatch", "release", "restock", "reject_return":
			default:
				c.JSON(400, gin.H{"error": "Unknown fulfilment action"})
				return
			}
		}
		var result any
		err := db.Transaction(func(tx *gorm.DB) error {
			if c.Request.Method != http.MethodGet {
				if err := tx.Exec("UPDATE sales_orders SET updated_at = updated_at WHERE id = ?", id).Error; err != nil {
					return err
				}
			}
			if c.Request.Method != http.MethodGet {
				features, err := models.LoadFeatureConfig(tx)
				if err != nil {
					return err
				}
				if !features["sales_orders"] {
					return productionConflict("Enable Business Operations before changing fulfilment")
				}
			}
			doc, err := orderFulfilment(tx, id)
			if err != nil {
				return err
			}
			switch c.Request.Method {
			case http.MethodPut:
				if err := cancelRemainder(tx, id, doc); err != nil {
					return err
				}
				result = gin.H{"cancelled": true}
			case http.MethodPost:
				result, err = input.apply(tx, doc)
			default:
				result = doc
			}
			return err
		})
		if errors.Is(err, inventory.ErrConflict) {
			err = productionConflict("Stock or order capacity changed. Refresh fulfilment before retrying")
		}
		if errors.Is(err, inventory.ErrInvalid) {
			c.JSON(400, gin.H{"error": "Invalid stock event"})
			return
		}
		if err != nil {
			orderProductionError(c, err)
			return
		}
		c.JSON(200, result)
	}
}
