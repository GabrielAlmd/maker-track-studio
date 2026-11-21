package budget

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const ProposalBackupSchema = 13

type ProposalTerms struct {
	Title         string `json:"title"`
	ValidUntil    string `json:"valid_until"`
	DueDate       string `json:"due_date"`
	LeadTimeDays  int64  `json:"lead_time_days"`
	DeliveryTerms string `json:"delivery_terms"`
	PaymentTerms  string `json:"payment_terms"`
}

type ProposalLineInput struct {
	BudgetLineID uuid.UUID      `json:"budget_line_id"`
	Description  string         `json:"description"`
	Price        PriceSelection `json:"price"`
	Discount     Discount       `json:"discount"`
}

type ProposalCommand struct {
	Command
	BudgetID       uuid.UUID           `json:"budget_id"`
	BudgetRevision int64               `json:"budget_revision"`
	CustomerID     uuid.UUID           `json:"customer_id"`
	Terms          ProposalTerms       `json:"terms"`
	Lines          []ProposalLineInput `json:"lines"`
	OrderDiscount  Discount            `json:"order_discount"`
}

type ProposalLine struct {
	ProposalLineInput
	Quantity           int64 `json:"quantity"`
	EstimatedCostCents int64 `json:"estimated_cost_cents"`
	UnitPriceCents     int64 `json:"unit_price_cents"`
	GrossCents         int64 `json:"gross_cents"`
	LineDiscountCents  int64 `json:"line_discount_cents"`
	OrderDiscountCents int64 `json:"order_discount_cents"`
	NetCents           int64 `json:"net_cents"`
}

type Proposal struct {
	ID                 uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	Revision           int64          `json:"revision" gorm:"primaryKey;autoIncrement:false"`
	BudgetID           uuid.UUID      `json:"budget_id" gorm:"type:uuid;not null"`
	BudgetRevision     int64          `json:"budget_revision" gorm:"not null"`
	CustomerID         uuid.UUID      `json:"customer_id" gorm:"type:uuid;not null"`
	CustomerRevision   int64          `json:"customer_revision" gorm:"not null"`
	CustomerName       string         `json:"customer_name"`
	CustomerAddress    string         `json:"customer_address"`
	Terms              ProposalTerms  `json:"terms" gorm:"serializer:json;not null"`
	Lines              []ProposalLine `json:"lines" gorm:"serializer:json;not null"`
	OrderDiscount      Discount       `json:"order_discount" gorm:"serializer:json;not null"`
	EstimatedCostCents int64          `json:"estimated_cost_cents"`
	GrossCents         int64          `json:"gross_cents"`
	LineDiscountCents  int64          `json:"line_discount_cents"`
	OrderDiscountCents int64          `json:"order_discount_cents"`
	NetCents           int64          `json:"net_cents"`
	CreatedAt          time.Time      `json:"created_at"`
	IssuedAt           *time.Time     `json:"issued_at"`
	SupersedesRevision int64          `json:"supersedes_revision,omitempty"`
}

func (Proposal) TableName() string { return "business_proposals" }

func (p *Proposal) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func validTerms(terms ProposalTerms) bool {
	return strings.TrimSpace(terms.Title) != "" && validText(terms.Title, 1, 160) &&
		validText(terms.DeliveryTerms, 0, 2000) && validText(terms.PaymentTerms, 0, 2000) &&
		terms.LeadTimeDays >= 0 && terms.LeadTimeDays <= 3650 &&
		(terms.ValidUntil == "" || validProposalDate(terms.ValidUntil)) &&
		(terms.DueDate == "" || validProposalDate(terms.DueDate))
}

func validProposalDate(date string) bool {
	parsed, err := time.Parse(time.DateOnly, date)
	return err == nil && parsed.Year() > 0 && parsed.Format(time.DateOnly) == date
}

func validAtIssue(terms ProposalTerms, now time.Time) bool {
	today := now.UTC().Format(time.DateOnly)
	return terms.ValidUntil >= today && (terms.DueDate == "" || terms.DueDate >= today)
}

func buildProposal(request ProposalCommand, estimate Record, customer Customer) (Proposal, error) {
	if !validTerms(request.Terms) {
		return Proposal{}, invalid("terms", "Provide a title and supported dates, lead time and commercial terms")
	}
	if len(request.Lines) != len(estimate.Items) || len(request.Lines) == 0 {
		return Proposal{}, invalid("lines", "Specify one price and description per budget line")
	}

	result := Proposal{
		BudgetID: estimate.ID, BudgetRevision: estimate.Revision, CustomerID: customer.ID,
		CustomerRevision: customer.Revision, CustomerName: customer.Name, CustomerAddress: customer.Address,
		Terms: request.Terms, OrderDiscount: request.OrderDiscount, EstimatedCostCents: estimate.EstimatedCostCents,
		Lines: make([]ProposalLine, 0, len(request.Lines)),
	}
	inputs := make(map[uuid.UUID]ProposalLineInput, len(request.Lines))
	for _, input := range request.Lines {
		inputs[input.BudgetLineID] = input
	}
	for _, item := range estimate.Items {
		input, ok := inputs[item.ID]
		if !ok || strings.TrimSpace(input.Description) == "" || !validText(input.Description, 1, 200) {
			return Proposal{}, invalid("lines", "Provide a customer-facing description for each distinct budget line")
		}
		price, ok := unitPrice(item.Estimate.UnitCents, input.Price)
		if !ok || price > (maxSafeInteger-result.GrossCents)/item.Quantity {
			return Proposal{}, invalid("price", "Choose a valid unit price, markup or margin within the supported total")
		}
		gross := price * item.Quantity
		discount, ok := appliedDiscount(gross, input.Discount)
		if !ok {
			return Proposal{}, invalid("line_discount", "A line discount cannot exceed its line subtotal")
		}
		result.Lines = append(result.Lines, ProposalLine{
			ProposalLineInput: input, Quantity: item.Quantity, EstimatedCostCents: item.Estimate.TotalCents,
			UnitPriceCents: price, GrossCents: gross, LineDiscountCents: discount, NetCents: gross - discount,
		})
		result.GrossCents += gross
		result.LineDiscountCents += discount
	}
	subtotal := result.GrossCents - result.LineDiscountCents
	discount, ok := appliedDiscount(subtotal, request.OrderDiscount)
	if !ok {
		return Proposal{}, invalid("order_discount", "The order discount cannot exceed the subtotal after line discounts")
	}
	allocateOrderDiscount(result.Lines, subtotal, discount)
	result.OrderDiscountCents = discount
	result.NetCents = subtotal - discount
	return result, nil
}

func checkProposalSources(tx *gorm.DB, budgetID, customerID uuid.UUID) (Customer, error) {
	estimate, err := readRevision[Record](tx, budgetID, 0)
	if err != nil {
		return Customer{}, err
	}
	customer, err := readRevision[Customer](tx, customerID, 0)
	if err != nil {
		return Customer{}, err
	}
	if estimate.IsArchived || customer.IsArchived {
		return Customer{}, &problem{status: http.StatusConflict, Code: "archived_source", Message: "The budget and customer must be active"}
	}
	return customer, nil
}

func writeProposal(db *gorm.DB, id uuid.UUID, request ProposalCommand) (Proposal, error) {
	return execute(db, "proposal", id, request.Command, request, func(tx *gorm.DB) (Proposal, uuid.UUID, int64, error) {
		if request.BudgetID == uuid.Nil || request.BudgetRevision < 1 || request.BudgetRevision > maxSafeInteger ||
			request.CustomerID == uuid.Nil {
			return Proposal{}, id, 0, invalid("source", "Select an exact budget revision and a customer")
		}
		if id != uuid.Nil {
			previous, err := readProposalForWrite(tx, id, request.ExpectedRevision)
			if err != nil {
				return Proposal{}, id, 0, err
			}
			if previous.BudgetID != request.BudgetID || previous.CustomerID != request.CustomerID {
				return Proposal{}, id, 0, invalid("source", "A proposal keeps the same budget and customer")
			}
		}
		estimate, err := readRevision[Record](tx, request.BudgetID, request.BudgetRevision)
		if err != nil {
			return Proposal{}, id, 0, err
		}
		if estimate.IsArchived || (estimate.CustomerID != nil && *estimate.CustomerID != request.CustomerID) {
			return Proposal{}, id, 0, invalid("source", "Select a draft budget revision with a compatible customer")
		}
		customer, err := checkProposalSources(tx, estimate.ID, request.CustomerID)
		if err != nil {
			return Proposal{}, id, 0, err
		}
		record, err := buildProposal(request, estimate, customer)
		if err != nil {
			return Proposal{}, id, 0, err
		}
		record.ID, record.Revision = id, request.ExpectedRevision+1
		err = tx.Create(&record).Error
		return record, record.ID, record.Revision, err
	})
}

func readProposalForWrite(tx *gorm.DB, id uuid.UUID, expectedRevision int64) (Proposal, error) {
	var accepted int64
	if err := tx.Model(&OrderRevision{}).Where("proposal_id = ?", id).Count(&accepted).Error; err != nil {
		return Proposal{}, err
	}
	if accepted > 0 {
		return Proposal{}, conflict("already_confirmed", "The accepted proposal is frozen")
	}
	record, err := readRevision[Proposal](tx, id, 0)
	if err == nil && record.Revision != expectedRevision {
		err = stale(record.Revision)
	}
	return record, err
}

func issueProposal(db *gorm.DB, id uuid.UUID, request Command) (Proposal, error) {
	// The action is part of the digest so an issue command cannot replay a draft write.
	action := struct {
		Command
		Action string `json:"action"`
	}{request, "issue"}
	return execute(db, "proposal", id, request, action, func(tx *gorm.DB) (Proposal, uuid.UUID, int64, error) {
		record, err := readProposalForWrite(tx, id, request.ExpectedRevision)
		if err != nil {
			return Proposal{}, id, 0, err
		}
		if record.IssuedAt != nil {
			return Proposal{}, id, 0, &problem{status: http.StatusConflict, Code: "not_draft", Message: "Only a draft proposal can be issued"}
		}
		if _, err := checkProposalSources(tx, record.BudgetID, record.CustomerID); err != nil {
			return Proposal{}, id, 0, err
		}
		now := time.Now().UTC()
		if !validAtIssue(record.Terms, now) {
			return Proposal{}, id, 0, invalid("terms", "Provide a validity date and any due date on or after today's UTC date")
		}
		var previousIssued int64
		if err := tx.Model(&Proposal{}).Where("id = ? AND issued_at IS NOT NULL", id).
			Select("COALESCE(MAX(revision), 0)").Scan(&previousIssued).Error; err != nil {
			return Proposal{}, id, 0, err
		}
		record.Revision++
		record.CreatedAt, record.IssuedAt = now, &now
		record.SupersedesRevision = previousIssued
		err = tx.Create(&record).Error
		return record, record.ID, record.Revision, err
	})
}
