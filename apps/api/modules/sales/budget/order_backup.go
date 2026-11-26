package budget

import (
	"reflect"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
)

const QuickSalePricingBackupSchema = 22

func (data Backup) validateOrderHistory(refs map[revisionKey]bool, schemaVersion int) error {
	proposals := map[revisionKey]Proposal{}
	for _, p := range data.proposals {
		proposals[revisionKey{"proposal", p.ID, p.Revision}] = p
	}
	type identity struct {
		id   uuid.UUID
		name string
	}
	customers := make(map[identity]bool)
	for _, c := range data.customers {
		if !c.IsArchived {
			customers[identity{c.ID, c.Name}] = true
		}
	}

	var previous OrderRevision
	for _, r := range data.orders {
		direct := r.ProposalID == uuid.Nil
		p := proposals[revisionKey{"proposal", r.ProposalID, r.ProposalRevision}]
		if r.ID == uuid.Nil || r.ConfirmedAt.IsZero() || !validText(r.PurchaseOrder, 0, 160) || !validText(r.DeliveryTerms, 0, 2000) {
			return errInvalidBackup
		}
		if r.QuickSalePricing != nil && (!direct || schemaVersion < QuickSalePricingBackupSchema) {
			return errInvalidBackup
		}
		if direct {
			if schemaVersion < 18 || r.ProposalRevision != 0 ||
				(r.CustomerID == uuid.Nil && r.CustomerName != "") ||
				(r.CustomerID != uuid.Nil && !customers[identity{r.CustomerID, r.CustomerName}]) {
				return errInvalidBackup
			}
		} else if r.CustomerName != "" || p.IssuedAt == nil || r.ConfirmedAt.Before(*p.IssuedAt) ||
			r.ConfirmedAt.UTC().Format(time.DateOnly) > p.Terms.ValidUntil || p.CustomerID != r.CustomerID {
			return errInvalidBackup
		}

		if r.ID != previous.ID {
			if r.Revision != 1 || r.Reason != "" || !r.CreatedAt.Equal(r.ConfirmedAt) ||
				r.DeliveryTerms != p.Terms.DeliveryTerms || (direct && r.PurchaseOrder != "") {
				return errInvalidBackup
			}
		} else if r.Revision != previous.Revision+1 || r.ProposalID != previous.ProposalID || r.ProposalRevision != previous.ProposalRevision ||
			r.CustomerID != previous.CustomerID || r.CustomerName != previous.CustomerName || !r.ConfirmedAt.Equal(previous.ConfirmedAt) ||
			!validText(r.Reason, 1, 500) || r.CreatedAt.Before(previous.CreatedAt) ||
			!reflect.DeepEqual(r.QuickSalePricing, previous.QuickSalePricing) {
			return errInvalidBackup
		}
		refs[revisionKey{"order", r.ID, r.Revision}] = true
		previous = r
	}
	return nil
}

// Cross-check the legacy projection against the immutable confirmed terms before replacement.
func (data Backup) ValidateOrderLinks(orders []storage.SalesOrder, items []storage.SalesOrderItem) error {
	revisions := map[uuid.UUID]OrderRevision{}
	for _, r := range data.orders {
		revisions[r.ID] = r
	}
	proposals := map[revisionKey]Proposal{}
	for _, p := range data.proposals {
		proposals[revisionKey{"proposal", p.ID, p.Revision}] = p
	}
	lines := map[uuid.UUID]map[uuid.UUID]ProposalLine{}
	type directTerms struct {
		payable  int64
		pricing  *QuickSalePricing
		lineSeen bool
	}
	direct := make(map[uuid.UUID]directTerms)
	bases := make(map[uuid.UUID]financeBasis)
	for _, order := range orders {
		r, ok := revisions[order.ID]
		if !ok && !order.IsV2() {
			continue
		}
		if !ok || order.FulfilmentStatus == storage.SalesOrderFulfilmentDraft {
			return errInvalidBackup
		}
		delete(revisions, order.ID)
		if r.ProposalID == uuid.Nil {
			if order.AcceptedProposalID != nil || order.QuickSaleCents == nil || *order.QuickSaleCents <= 0 || !validMoney(*order.QuickSaleCents) ||
				!order.OrderDate.Equal(r.ConfirmedAt) ||
				(order.FulfilmentStatus != storage.SalesOrderFulfilmentConfirmed && order.FulfilmentStatus != storage.SalesOrderFulfilmentPreparing && order.FulfilmentStatus != storage.SalesOrderFulfilmentCancelled) {
				return errInvalidBackup
			}
			direct[order.ID] = directTerms{payable: *order.QuickSaleCents, pricing: r.QuickSalePricing}
			bases[order.ID] = quickSaleFinance(*order.QuickSaleCents, r.QuickSalePricing)
			continue
		}
		if order.QuickSaleCents != nil || order.AcceptedProposalID == nil || *order.AcceptedProposalID != r.ProposalID {
			return errInvalidBackup
		}
		p := proposals[revisionKey{"proposal", r.ProposalID, r.ProposalRevision}]
		bases[order.ID] = proposalFinance(p)
		lines[order.ID] = map[uuid.UUID]ProposalLine{}
		for _, line := range p.Lines {
			lines[order.ID][line.BudgetLineID] = line
		}
	}
	if len(revisions) > 0 {
		return errInvalidBackup
	}
	for _, item := range items {
		if terms, ok := direct[item.SalesOrderID]; ok {
			quantity := int64(item.Quantity)
			if terms.lineSeen || item.BudgetLineID != nil || item.DiscountCents != 0 || quantity < 1 || quantity > quickSaleMaxQuantity ||
				item.UnitPriceCents < 1 || item.UnitPriceCents > maxSafeInteger/quantity || item.UnitPriceCents*quantity != terms.payable ||
				!validQuickSalePricing(terms.pricing, quantity) {
				return errInvalidBackup
			}
			terms.lineSeen = true
			direct[item.SalesOrderID] = terms
			continue
		}
		expected, formal := lines[item.SalesOrderID]
		if !formal {
			if item.BudgetLineID != nil || item.DiscountCents != 0 {
				return errInvalidBackup
			}
			continue
		}
		if item.BudgetLineID == nil {
			return errInvalidBackup
		}
		line, ok := expected[*item.BudgetLineID]
		if !ok || line.Description != item.ModelNameSnapshot || line.Quantity != int64(item.Quantity) || line.UnitPriceCents != item.UnitPriceCents || line.GrossCents-line.NetCents != item.DiscountCents {
			return errInvalidBackup
		}
		delete(expected, *item.BudgetLineID)
	}
	for _, remaining := range lines {
		if len(remaining) > 0 {
			return errInvalidBackup
		}
	}
	for _, terms := range direct {
		if !terms.lineSeen {
			return errInvalidBackup
		}
	}
	return data.validateFinance(orders, bases)
}

func (data Backup) validateFinance(salesOrders []storage.SalesOrder, bases map[uuid.UUID]financeBasis) error {
	statuses := make(map[uuid.UUID]storage.SalesOrderPaymentStatus, len(salesOrders))
	for _, order := range salesOrders {
		statuses[order.ID] = order.PaymentStatus
	}

	seen := make(map[uuid.UUID]bool)
	for start := 0; start < len(data.finance); {
		id := data.finance[start].ID
		basis, ok := bases[id]
		if !ok {
			return errInvalidBackup
		}
		end := start
		for end < len(data.finance) && data.finance[end].ID == id {
			entry := data.finance[end]
			if entry.CreatedAt.IsZero() {
				return errInvalidBackup
			}

			end++
		}
		summary, err := summarizeFinance(basis, data.finance[start:end], time.Now().UTC().Format(time.DateOnly))
		if err != nil || summary.legacyPayment() != statuses[id] {
			return errInvalidBackup
		}
		seen[id] = true
		start = end
	}
	for _, order := range salesOrders {
		if order.QuickSaleCents != nil && !seen[order.ID] && order.PaymentStatus != storage.SalesOrderPaymentUnpaid {
			return errInvalidBackup
		}
	}
	return nil
}
