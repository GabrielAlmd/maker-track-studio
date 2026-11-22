package budget

import (
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
)

func (data Backup) validateOrderHistory(refs map[revisionKey]bool) error {
	proposals := map[revisionKey]Proposal{}
	for _, p := range data.proposals {
		proposals[revisionKey{"proposal", p.ID, p.Revision}] = p
	}
	var previous OrderRevision
	for _, r := range data.orders {
		p := proposals[revisionKey{"proposal", r.ProposalID, r.ProposalRevision}]
		if r.ID == uuid.Nil || p.IssuedAt == nil || r.ConfirmedAt.Before(*p.IssuedAt) || r.ConfirmedAt.UTC().Format(time.DateOnly) > p.Terms.ValidUntil || p.CustomerID != r.CustomerID ||
			!validText(r.PurchaseOrder, 0, 160) || !validText(r.DeliveryTerms, 0, 2000) {
			return errInvalidBackup
		}
		if r.ID != previous.ID {
			if r.Revision != 1 || r.Reason != "" || !r.CreatedAt.Equal(r.ConfirmedAt) || r.DeliveryTerms != p.Terms.DeliveryTerms {
				return errInvalidBackup
			}
		} else if r.Revision != previous.Revision+1 || r.ProposalID != previous.ProposalID || r.ProposalRevision != previous.ProposalRevision || !r.ConfirmedAt.Equal(previous.ConfirmedAt) || !validText(r.Reason, 1, 500) || r.CreatedAt.Before(previous.CreatedAt) {
			return errInvalidBackup
		}
		refs[revisionKey{"order", r.ID, r.Revision}] = true
		previous = r
	}
	return nil
}

// Cross-check the legacy projection against the immutable proposal before replacement.
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
	for _, order := range orders {
		r, ok := revisions[order.ID]
		if !ok && order.AcceptedProposalID == nil {
			continue
		}
		if !ok || order.AcceptedProposalID == nil || *order.AcceptedProposalID != r.ProposalID || order.FulfilmentStatus == storage.SalesOrderFulfilmentDraft {
			return errInvalidBackup
		}
		delete(revisions, order.ID)
		lines[order.ID] = map[uuid.UUID]ProposalLine{}
		for _, line := range proposals[revisionKey{"proposal", r.ProposalID, r.ProposalRevision}].Lines {
			lines[order.ID][line.BudgetLineID] = line
		}
	}
	if len(revisions) > 0 {
		return errInvalidBackup
	}
	for _, item := range items {
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
	return data.validateFinance(orders, proposals)
}

func (data Backup) validateFinance(salesOrders []storage.SalesOrder, proposals map[revisionKey]Proposal) error {
	statuses := make(map[uuid.UUID]storage.SalesOrderPaymentStatus, len(salesOrders))
	for _, order := range salesOrders {
		statuses[order.ID] = order.PaymentStatus
	}
	orders := map[uuid.UUID]OrderRevision{}
	for _, order := range data.orders {
		orders[order.ID] = order
	}
	for start := 0; start < len(data.finance); {
		id := data.finance[start].ID
		order, ok := orders[id]
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
		summary, err := summarizeFinance(proposals[revisionKey{"proposal", order.ProposalID, order.ProposalRevision}], data.finance[start:end], time.Now().UTC().Format(time.DateOnly))
		if err != nil || summary.legacyPayment() != statuses[id] {
			return errInvalidBackup
		}
		start = end
	}
	return nil
}
