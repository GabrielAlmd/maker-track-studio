package budget

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Backup struct {
	customers  []Customer
	budgets    []Record
	proposals  []Proposal
	orders     []OrderRevision
	finance    []FinanceEntry
	operations []operation
}

var errInvalidBackup = errors.New("Backup contains invalid Business customer, budget, proposal or operation data")

func ReadBackup(source *gorm.DB, schemaVersion int) (Backup, error) {
	var data Backup
	if schemaVersion < BackupSchema {
		for _, table := range Tables() {
			if requireEmptyTable(source, table) != nil {
				return data, errInvalidBackup
			}
		}
		return data, nil
	}
	if schemaVersion < 17 {
		if requireEmptyTable(source, &FinanceEntry{}) != nil {
			return data, errInvalidBackup
		}
	} else if source.Order("id, revision").Find(&data.finance).Error != nil {
		return data, errInvalidBackup
	}
	for _, table := range budgetTables() {
		if !source.Migrator().HasTable(table) {
			return data, errInvalidBackup
		}
	}
	if schemaVersion >= 18 && !source.Migrator().HasColumn(&OrderRevision{}, "customer_name") {
		return data, errInvalidBackup
	}
	if schemaVersion < 14 {
		if requireEmptyTable(source, &OrderRevision{}) != nil {
			return data, errInvalidBackup
		}
	} else if source.Order("id, revision").Find(&data.orders).Error != nil {
		return data, errInvalidBackup
	}
	if schemaVersion < ProposalBackupSchema {
		if requireEmptyTable(source, &Proposal{}) != nil {
			return data, errInvalidBackup
		}
	} else if !source.Migrator().HasTable(&Proposal{}) ||
		source.Order("id, revision").Find(&data.proposals).Error != nil {
		return data, errInvalidBackup
	}
	if source.Order("id, revision").Find(&data.customers).Error != nil ||
		source.Order("id, revision").Find(&data.budgets).Error != nil ||
		source.Find(&data.operations).Error != nil {
		return data, errInvalidBackup
	}
	return data, data.validate(schemaVersion)
}

func requireEmptyTable(source *gorm.DB, table any) error {
	if source.Migrator().HasTable(table) {
		var count int64
		if source.Model(table).Count(&count).Error != nil || count != 0 {
			return errInvalidBackup
		}
	}
	return nil
}

type revisionKey struct {
	resource string
	id       uuid.UUID
	revision int64
}

func (data Backup) validate(schemaVersion int) error {
	customers := make(map[uuid.UUID]int64)
	budgets := make(map[uuid.UUID]int64)
	refs := make(map[revisionKey]bool)
	for _, customer := range data.customers {
		if customer.ID == uuid.Nil || customer.CreatedAt.IsZero() || !validCustomer(customer.CustomerInput) ||
			customer.Revision != customers[customer.ID]+1 || (customer.Revision == 1 && customer.IsArchived) {
			return errInvalidBackup
		}
		customers[customer.ID] = customer.Revision
		refs[revisionKey{"customer", customer.ID, customer.Revision}] = true
	}
	var lastBudget Record
	lineOwners := make(map[uuid.UUID]uuid.UUID)
	for _, budget := range data.budgets {
		if budget.ID == uuid.Nil || budget.CreatedAt.IsZero() || !validText(budget.Title, 1, 160) ||
			budget.Revision != budgets[budget.ID]+1 || len(budget.Items) < 1 || len(budget.Items) > maxItems ||
			(budget.CustomerID != nil && customers[*budget.CustomerID] == 0) ||
			(budget.IsArchived && (budget.Revision == 1 || lastBudget.IsArchived)) {
			return errInvalidBackup
		}
		var total int64
		priorLines := make(map[uuid.UUID]bool)
		if lastBudget.ID == budget.ID {
			for _, line := range lastBudget.Items {
				priorLines[line.ID] = true
			}
		}
		seen := make(map[uuid.UUID]bool)
		for _, item := range budget.Items {
			owner, previouslySeen := lineOwners[item.ID]
			if item.ID == uuid.Nil || seen[item.ID] || (previouslySeen && (owner != budget.ID || !priorLines[item.ID])) ||
				!validText(item.Name, 1, 200) || !validItem(item.ItemInput) {
				return errInvalidBackup
			}
			estimate, err := calculate(item.ItemInput, item.Sources)
			if err != nil || estimate != item.Estimate || estimate.TotalCents > maxSafeInteger-total {
				return errInvalidBackup
			}
			seen[item.ID], lineOwners[item.ID] = true, budget.ID
			total += estimate.TotalCents
		}
		if total != budget.EstimatedCostCents {
			return errInvalidBackup
		}
		budgets[budget.ID] = budget.Revision
		refs[revisionKey{"budget", budget.ID, budget.Revision}] = true
		lastBudget = budget
	}
	if err := data.validateProposals(refs); err != nil {
		return err
	}
	if err := data.validateOrderHistory(refs, schemaVersion); err != nil {
		return err
	}
	for _, entry := range data.finance {
		refs[revisionKey{"finance", entry.ID, entry.Revision}] = true
	}
	usedOperations := make(map[uuid.UUID]bool)
	for _, op := range data.operations {
		target, digest, ok := strings.Cut(op.Hash, ":")
		decoded, err := hex.DecodeString(digest)
		key := revisionKey{op.Resource, op.ResourceID, op.Revision}
		wantTarget := op.ResourceID
		if op.Revision == 1 && op.Resource != "finance" {
			wantTarget = uuid.Nil
		}
		if op.ID == uuid.Nil || usedOperations[op.ID] || !ok || err != nil || len(decoded) != 32 ||
			target != wantTarget.String() || !refs[key] {
			return errInvalidBackup
		}
		usedOperations[op.ID] = true
		delete(refs, key)
	}
	if len(refs) != 0 {
		return errInvalidBackup
	}
	return nil
}

func DeleteBackup(tx *gorm.DB) error {
	tables := Tables()
	deleteAll := tx.Session(&gorm.Session{AllowGlobalUpdate: true})
	for i := len(tables) - 1; i >= 0; i-- {
		if err := deleteAll.Delete(tables[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

func RestoreBackup(tx *gorm.DB, data Backup) error {
	if len(data.customers) > 0 {
		if err := tx.CreateInBatches(&data.customers, 100).Error; err != nil {
			return err
		}
	}
	if len(data.budgets) > 0 {
		if err := tx.CreateInBatches(&data.budgets, 100).Error; err != nil {
			return err
		}
	}
	if len(data.proposals) > 0 {
		if err := tx.CreateInBatches(&data.proposals, 100).Error; err != nil {
			return err
		}
	}
	if len(data.orders) > 0 {
		if err := tx.CreateInBatches(&data.orders, 100).Error; err != nil {
			return err
		}
	}
	if len(data.finance) > 0 {
		if err := tx.CreateInBatches(&data.finance, 100).Error; err != nil {
			return err
		}
	}
	if len(data.operations) > 0 {
		return tx.CreateInBatches(&data.operations, 100).Error
	}
	return nil
}
