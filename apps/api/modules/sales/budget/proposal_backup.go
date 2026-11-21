package budget

import (
	"reflect"

	"github.com/google/uuid"
)

func (data Backup) validateProposals(refs map[revisionKey]bool) error {
	if len(data.proposals) == 0 {
		return nil
	}
	type key struct {
		id       uuid.UUID
		revision int64
	}
	budgets := make(map[key]Record, len(data.budgets))
	for _, record := range data.budgets {
		budgets[key{record.ID, record.Revision}] = record
	}
	customers := make(map[key]Customer, len(data.customers))
	for _, record := range data.customers {
		customers[key{record.ID, record.Revision}] = record
	}
	var prior Proposal
	var latestIssued int64
	for _, proposal := range data.proposals {
		if prior.ID != proposal.ID {
			prior, latestIssued = Proposal{}, 0
		}
		if proposal.ID == uuid.Nil || proposal.CreatedAt.IsZero() || proposal.Revision != prior.Revision+1 ||
			(prior.Revision != 0 && (proposal.BudgetID != prior.BudgetID || proposal.CustomerID != prior.CustomerID)) {
			return errInvalidBackup
		}
		estimate := budgets[key{proposal.BudgetID, proposal.BudgetRevision}]
		customer := customers[key{proposal.CustomerID, proposal.CustomerRevision}]
		if estimate.ID == uuid.Nil || estimate.IsArchived || customer.ID == uuid.Nil || customer.IsArchived ||
			(estimate.CustomerID != nil && *estimate.CustomerID != customer.ID) {
			return errInvalidBackup
		}
		if proposal.IssuedAt != nil {
			if prior.Revision == 0 || prior.IssuedAt != nil || proposal.SupersedesRevision != latestIssued ||
				!proposal.CreatedAt.Equal(*proposal.IssuedAt) || !validAtIssue(proposal.Terms, *proposal.IssuedAt) ||
				!reflect.DeepEqual(commercialSnapshot(proposal), commercialSnapshot(prior)) {
				return errInvalidBackup
			}
			latestIssued = proposal.Revision
		} else {
			inputs := make([]ProposalLineInput, 0, len(proposal.Lines))
			for _, line := range proposal.Lines {
				inputs = append(inputs, line.ProposalLineInput)
			}
			rebuilt, err := buildProposal(ProposalCommand{
				Terms: proposal.Terms, Lines: inputs, OrderDiscount: proposal.OrderDiscount,
			}, estimate, customer)
			if err != nil || proposal.SupersedesRevision != 0 || !reflect.DeepEqual(commercialSnapshot(proposal), rebuilt) {
				return errInvalidBackup
			}
		}
		refs[revisionKey{"proposal", proposal.ID, proposal.Revision}] = true
		prior = proposal
	}
	return nil
}

// Remove revision metadata before comparing the frozen commercial content.
func commercialSnapshot(p Proposal) Proposal {
	p.ID, p.Revision = uuid.Nil, 0
	p.CreatedAt, p.IssuedAt, p.SupersedesRevision = Proposal{}.CreatedAt, nil, 0
	return p
}
