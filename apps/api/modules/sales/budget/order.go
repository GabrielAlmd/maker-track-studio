package budget

import (
	"net/http"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The ID is the SalesOrder ID. Revision 1 is the original acceptance.
type OrderRevision struct {
	ID               uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Revision         int64     `json:"revision" gorm:"primaryKey;autoIncrement:false"`
	ProposalID       uuid.UUID `json:"proposal_id" gorm:"type:uuid;not null"`
	ProposalRevision int64     `json:"proposal_revision"`
	CustomerName     string    `json:"customer_name,omitempty"`
	CustomerID       uuid.UUID `json:"customer_id"`
	ConfirmedAt      time.Time `json:"confirmed_at"`
	PurchaseOrder    string    `json:"purchase_order"`
	DeliveryTerms    string    `json:"delivery_terms"`
	Reason           string    `json:"reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (OrderRevision) TableName() string { return "business_order_revisions" }

type ConfirmationCommand struct {
	Command
	ProposalID       uuid.UUID `json:"proposal_id"`
	ProposalRevision int64     `json:"proposal_revision"`
	PurchaseOrder    string    `json:"purchase_order"`
}
type AmendmentCommand struct {
	Command
	PurchaseOrder *string `json:"purchase_order"`
	DeliveryTerms *string `json:"delivery_terms"`
	Reason        string  `json:"reason"`
}

func conflict(code, message string) error {
	return &problem{status: http.StatusConflict, Code: code, Message: message}
}

func confirmOrder(db *gorm.DB, id uuid.UUID, input ConfirmationCommand) (OrderRevision, error) {
	return execute(db, "order", id, input.Command, input, func(tx *gorm.DB) (OrderRevision, uuid.UUID, int64, error) {
		var revision OrderRevision
		if input.ProposalID == uuid.Nil || input.ProposalRevision < 1 || input.ProposalRevision > maxSafeInteger || !validText(input.PurchaseOrder, 0, 160) {
			return revision, id, 0, invalid("confirmation", "Provide the issued proposal ID and revision and a purchase order of at most 160 characters")
		}
		p, err := readRevision[Proposal](tx, input.ProposalID, input.ProposalRevision)
		if err != nil {
			return revision, id, 0, err
		}
		now := time.Now().UTC()
		if p.IssuedAt == nil || p.Terms.ValidUntil < now.Format(time.DateOnly) {
			return revision, id, 0, conflict("ineligible_revision", "Issue a current, unexpired proposal before accepting it")
		}
		var newer int64
		if err := tx.Model(&Proposal{}).Where("id = ? AND revision > ? AND issued_at IS NOT NULL", p.ID, p.Revision).Count(&newer).Error; err != nil {
			return revision, id, 0, err
		}
		if newer > 0 {
			return revision, id, 0, conflict("superseded", "Accept the latest issued revision")
		}
		var accepted int64
		if err := tx.Model(&storage.SalesOrder{}).Where("accepted_proposal_id = ?", p.ID).Count(&accepted).Error; err != nil {
			return revision, id, 0, err
		}
		if accepted > 0 {
			return revision, id, 0, conflict("already_confirmed", "This proposal already has a confirmed order")
		}
		estimate, err := readRevision[Record](tx, p.BudgetID, p.BudgetRevision)
		if err != nil {
			return revision, id, 0, err
		}
		order := storage.SalesOrder{Reference: p.Terms.Title, OrderDate: now, AcceptedProposalID: &p.ID}
		sources := make(map[uuid.UUID]Item, len(estimate.Items))
		for _, item := range estimate.Items {
			sources[item.ID] = item
		}
		items := make([]storage.SalesOrderItem, 0, len(p.Lines))
		for _, line := range p.Lines {
			source, exists := sources[line.BudgetLineID]
			if !exists {
				return revision, id, 0, conflict("missing_line", "The issued proposal refers to a missing budget line")
			}
			modelID := source.ModelID
			if modelID != nil {
				var exists int64
				if err := tx.Model(&models.Model{}).Where("id = ?", *modelID).Count(&exists).Error; err != nil {
					return revision, id, 0, err
				}
				if exists == 0 {
					modelID = nil
				}
			}
			item := storage.SalesOrderItem{
				BudgetLineID: &line.BudgetLineID, ModelID: modelID,
				ModelNameSnapshot: line.Description, Quantity: int(line.Quantity),
				UnitPriceCents: line.UnitPriceCents, DiscountCents: line.LineDiscountCents + line.OrderDiscountCents,
			}
			items = append(items, item)
		}
		revision = OrderRevision{
			CustomerID: p.CustomerID, ProposalID: p.ID, ProposalRevision: p.Revision,
			ConfirmedAt: now, CreatedAt: now, PurchaseOrder: strings.TrimSpace(input.PurchaseOrder), DeliveryTerms: p.Terms.DeliveryTerms,
		}
		err = saveConfirmedOrder(tx, &order, items, &revision)
		return revision, revision.ID, 1, err
	})
}

func saveConfirmedOrder(tx *gorm.DB, order *storage.SalesOrder, items []storage.SalesOrderItem, terms *OrderRevision) error {
	order.FulfilmentStatus = storage.SalesOrderFulfilmentConfirmed
	if err := tx.Create(order).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].SalesOrderID = order.ID
		if err := tx.Create(&items[i]).Error; err != nil {
			return err
		}
	}
	terms.ID, terms.Revision = order.ID, 1
	return tx.Create(terms).Error
}

func amendOrder(db *gorm.DB, id uuid.UUID, input AmendmentCommand) (OrderRevision, error) {
	return execute(db, "order", id, input.Command, input, func(tx *gorm.DB) (OrderRevision, uuid.UUID, int64, error) {
		row, err := readRevision[OrderRevision](tx, id, 0)
		if err != nil {
			return row, id, 0, err
		}
		if row.Revision != input.ExpectedRevision {
			return row, id, 0, stale(row.Revision)
		}
		var order storage.SalesOrder
		if err := tx.Take(&order, "id = ?", id).Error; err != nil {
			return row, id, 0, err
		}
		if order.FulfilmentStatus != storage.SalesOrderFulfilmentConfirmed && order.FulfilmentStatus != storage.SalesOrderFulfilmentPreparing {
			return row, id, 0, conflict("protected_order", "Only confirmed or preparing orders can be amended")
		}
		reason := strings.TrimSpace(input.Reason)
		if !validText(reason, 1, 500) {
			return row, id, 0, invalid("reason", "Explain the correction in 1 to 500 characters")
		}
		prior := row
		if input.PurchaseOrder != nil {
			row.PurchaseOrder = strings.TrimSpace(*input.PurchaseOrder)
		}
		if input.DeliveryTerms != nil {
			row.DeliveryTerms = strings.TrimSpace(*input.DeliveryTerms)
		}
		if !validText(row.PurchaseOrder, 0, 160) || !validText(row.DeliveryTerms, 0, 2000) {
			return row, id, 0, invalid("terms", "Purchase order and delivery terms exceed their supported lengths")
		}
		if row.PurchaseOrder == prior.PurchaseOrder && row.DeliveryTerms == prior.DeliveryTerms {
			return row, id, 0, invalid("amendment", "Change the purchase order or delivery terms")
		}
		row.Revision++
		row.CreatedAt = time.Now().UTC()
		row.Reason = reason
		err = tx.Create(&row).Error
		return row, id, row.Revision, err
	})
}
