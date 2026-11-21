package budget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerCommand struct {
	Command
	CustomerInput
}

type BudgetCommand struct {
	Command
	Header
	Items []ItemInput `json:"items"`
}

type problem struct {
	status  int
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Current int64  `json:"current_revision,omitempty"`
}

func (p *problem) Error() string { return p.Message }

func invalid(field, message string) error {
	return &problem{status: http.StatusBadRequest, Code: "invalid_field", Field: field, Message: message}
}

func missing(err error, field, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &problem{status: http.StatusNotFound, Code: "not_found", Field: field, Message: message}
	}
	return err
}

func stale(current int64) error {
	return &problem{status: http.StatusConflict, Code: "stale_revision", Current: current, Message: "The record changed since it was loaded"}
}

func readRevision[T any](tx *gorm.DB, id uuid.UUID, revision int64) (T, error) {
	var row T
	query := tx.Where("id = ?", id)
	if revision != 0 {
		query = query.Where("revision = ?", revision)
	}
	err := query.Order("revision DESC").Take(&row).Error
	return row, missing(err, "id", "Record or revision was not found")
}

// Inserting the operation first takes the SQLite writer lock before revisions
// or cost inputs are read. Failed mutations roll back the operation as well.
func execute[T any](db *gorm.DB, resource string, id uuid.UUID, command Command, input any,
	mutate func(*gorm.DB) (T, uuid.UUID, int64, error),
) (T, error) {
	var result T
	if command.OperationID == uuid.Nil {
		return result, invalid("operation_id", "Provide an operation UUID")
	}
	if (id == uuid.Nil && command.ExpectedRevision != 0) ||
		(id != uuid.Nil && (command.ExpectedRevision < 1 || command.ExpectedRevision >= maxSafeInteger)) {
		return result, invalid("expected_revision", "Provide the current positive revision for an edit or 0 for a create")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(payload)
	op := operation{
		ID: command.OperationID, Resource: resource,
		Hash: id.String() + ":" + hex.EncodeToString(digest[:]),
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&op)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var prior operation
			if err := tx.Take(&prior, "id = ?", op.ID).Error; err != nil {
				return err
			}
			if prior.Hash != op.Hash || prior.Resource != op.Resource {
				return &problem{status: http.StatusConflict, Code: "operation_conflict", Message: "This operation UUID was used for a different request"}
			}
			result, err = readRevision[T](tx, prior.ResourceID, prior.Revision)
			return err
		}
		result, op.ResourceID, op.Revision, err = mutate(tx)
		if err != nil {
			return err
		}
		return tx.Model(&operation{}).Where("id = ?", op.ID).
			Updates(map[string]any{"resource_id": op.ResourceID, "revision": op.Revision}).Error
	})
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked) {
		return result, conflict("concurrent_update", "Another write is in progress. Retry the command.")
	}
	return result, err
}

func validText(s string, min, max int) bool {
	return !strings.ContainsRune(s, 0) && utf8.ValidString(s) &&
		utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max
}

func validCustomer(input CustomerInput) bool {
	return validText(input.Name, 1, 160) && strings.TrimSpace(input.Name) != "" &&
		validText(input.Email, 0, 254) && validText(input.Phone, 0, 64) && validText(input.Address, 0, 500)
}

func writeCustomer(db *gorm.DB, id uuid.UUID, request CustomerCommand) (Customer, error) {
	return execute(db, "customer", id, request.Command, request, func(tx *gorm.DB) (Customer, uuid.UUID, int64, error) {
		record := Customer{ID: id, Revision: request.ExpectedRevision + 1, CustomerInput: request.CustomerInput}
		record.Name, record.Email = strings.TrimSpace(record.Name), strings.TrimSpace(record.Email)
		record.Phone, record.Address = strings.TrimSpace(record.Phone), strings.TrimSpace(record.Address)
		if !validCustomer(record.CustomerInput) || (id == uuid.Nil && record.IsArchived) {
			return record, id, 0, invalid("customer", "Provide a name and contact details within the supported lengths")
		}
		if id != uuid.Nil {
			previous, err := readRevision[Customer](tx, id, 0)
			if err != nil {
				return record, id, 0, err
			}
			if previous.Revision != request.ExpectedRevision {
				return record, id, 0, stale(previous.Revision)
			}
		}
		err := tx.Create(&record).Error
		return record, record.ID, record.Revision, err
	})
}

func writeBudget(db *gorm.DB, id uuid.UUID, request BudgetCommand) (Record, error) {
	return execute(db, "budget", id, request.Command, request, func(tx *gorm.DB) (Record, uuid.UUID, int64, error) {
		var previous Record
		if id != uuid.Nil {
			var err error
			previous, err = readRevision[Record](tx, id, 0)
			if err != nil {
				return Record{}, id, 0, err
			}
			if previous.Revision != request.ExpectedRevision {
				return Record{}, id, 0, stale(previous.Revision)
			}
		}
		record, err := buildBudget(tx, previous, request)
		if err != nil {
			return Record{}, id, 0, err
		}
		err = tx.Create(&record).Error
		return record, record.ID, record.Revision, err
	})
}

func buildBudget(tx *gorm.DB, previous Record, request BudgetCommand) (Record, error) {
	result := Record{ID: previous.ID, Revision: previous.Revision + 1, Header: request.Header}
	result.Title = strings.TrimSpace(result.Title)
	if !validText(result.Title, 1, 160) || !validID(result.CustomerID) {
		return result, invalid("budget", "Provide a title and a valid customer reference")
	}
	if len(request.Items) == 0 || len(request.Items) > maxItems {
		return result, invalid("items", "Provide between 1 and 100 budget lines")
	}
	if previous.ID == uuid.Nil && result.IsArchived {
		return result, invalid("is_archived", "A new budget must start as a draft")
	}
	if previous.IsArchived && result.IsArchived {
		return result, &problem{status: http.StatusConflict, Code: "archived_budget", Message: "Restore the budget before editing it"}
	}
	if result.CustomerID != nil && !reflect.DeepEqual(result.CustomerID, previous.CustomerID) {
		customer, err := readRevision[Customer](tx, *result.CustomerID, 0)
		if err != nil {
			return result, err
		}
		if customer.IsArchived {
			return result, invalid("customer_id", "Select an active customer")
		}
	}

	priorItems := make(map[uuid.UUID]Item, len(previous.Items))
	for _, item := range previous.Items {
		priorItems[item.ID] = item
	}
	for _, input := range request.Items {
		prior, exists := priorItems[input.ID]
		if input.ID != uuid.Nil && !exists {
			return result, invalid("items", "Line IDs must belong to the current budget revision and appear only once")
		}
		delete(priorItems, input.ID)
		item, err := buildItem(tx, input, prior)
		if err != nil {
			return result, err
		}
		if item.Estimate.TotalCents > maxSafeInteger-result.EstimatedCostCents {
			return result, invalid("items", "Budget cost exceeds the supported range")
		}
		result.EstimatedCostCents += item.Estimate.TotalCents
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func buildItem(tx *gorm.DB, input ItemInput, prior Item) (Item, error) {
	if !validItem(input) {
		return Item{}, invalid("items", "Invalid quantity, production assumptions or cost")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Assumptions.Materials == nil {
		input.Assumptions.Materials = []MaterialInput{}
	}
	modelUnchanged := prior.ID != uuid.Nil && reflect.DeepEqual(input.ModelID, prior.ModelID)
	if input.ModelID != nil {
		if !modelUnchanged {
			var model models.Model
			if err := tx.Take(&model, "id = ?", *input.ModelID).Error; err != nil {
				return Item{}, missing(err, "model_id", "Model was not found")
			}
			if input.Name == "" {
				input.Name = model.Name
			}
		} else if input.Name == "" {
			input.Name = prior.Name
		}
	}
	if !validText(input.Name, 1, 200) {
		return Item{}, invalid("name", "Provide a line name of up to 200 characters")
	}
	source := prior.Sources
	if prior.ID == uuid.Nil || !reflect.DeepEqual(input.Assumptions, prior.Assumptions) {
		var err error
		source, err = captureSources(tx, input.Assumptions)
		if err != nil {
			return Item{}, err
		}
	}
	estimate, err := calculate(input, source)
	return Item{ItemInput: input, Estimate: estimate, Sources: source}, err
}
