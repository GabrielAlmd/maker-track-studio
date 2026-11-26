package budget

import (
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FinanceInput struct {
	Kind             string `json:"kind"`
	Reference        string `json:"reference"`
	Date             string `json:"date"`
	DueDate          string `json:"due_date,omitempty"`
	AmountCents      int64  `json:"amount_cents"`
	OriginalRevision int64  `json:"original_revision,omitempty"`
}

type FinanceCommand struct {
	Command
	FinanceInput
}

// ID and Revision identify one immutable entry in an order's financial history.
type FinanceEntry struct {
	ID       uuid.UUID `json:"order_id" gorm:"type:uuid;primaryKey"`
	Revision int64     `json:"revision" gorm:"primaryKey;autoIncrement:false"`
	FinanceInput
	CreatedAt time.Time `json:"created_at"`
}

func (FinanceEntry) TableName() string { return "business_finance_entries" }

type FinanceSummary struct {
	Payable         int64          `json:"payable_cents"`
	Paid            int64          `json:"net_paid_cents"`
	Balance         int64          `json:"balance_cents"`
	Invoiced        int64          `json:"invoiced_cents"`
	EstimatedProfit *int64         `json:"estimated_profit_cents"`
	Overdue         bool           `json:"overdue"`
	PaymentStatus   string         `json:"payment_status"`
	InvoiceStatus   string         `json:"invoice_status"`
	Entries         []FinanceEntry `json:"entries"`
}

func validFinanceDate(date string) bool {
	_, err := time.Parse(time.DateOnly, date)
	return err == nil && len(date) == 10
}

func summarizeFinance(basis financeBasis, entries []FinanceEntry, today string) (FinanceSummary, error) {
	s := FinanceSummary{Payable: basis.payable, EstimatedProfit: basis.estimatedProfit, Entries: entries, InvoiceStatus: "none", PaymentStatus: "unpaid"}
	available := make(map[int64]int64)
	var due int64
	var refunded, billed bool
	for i, e := range entries {
		if e.Revision != int64(i)+1 || e.AmountCents <= 0 || e.AmountCents > maxSafeInteger || !validFinanceDate(e.Date) ||
			!validText(e.Reference, 1, 160) || strings.TrimSpace(e.Reference) == "" || (e.DueDate != "" && (e.Kind != "invoice" || !validFinanceDate(e.DueDate) || e.DueDate < e.Date)) {
			return s, invalid("entry", "Provide a positive amount, date and reference and a valid invoice due date")
		}
		switch e.Kind {
		case "invoice", "payment":
			if e.OriginalRevision != 0 {
				return s, invalid("original_revision", "Only reversals refer to a prior entry")
			}
			available[e.Revision] = e.AmountCents
			if e.Kind == "invoice" {
				if e.AmountCents > s.Payable-s.Invoiced {
					return s, conflict("invoice_limit", "Invoice exceeds the uninvoiced amount")
				}
				billed = true
				s.Invoiced += e.AmountCents
				if e.DueDate != "" && e.DueDate < today {
					due += e.AmountCents
				}
			} else {
				if e.AmountCents > s.Payable-s.Paid {
					return s, conflict("payment_limit", "Payment exceeds the outstanding balance")
				}
				s.Paid += e.AmountCents
			}
		case "refund", "void":
			if e.OriginalRevision < 1 || e.OriginalRevision >= e.Revision {
				return s, invalid("original_revision", "Choose an earlier entry on this order")
			}
			original := entries[e.OriginalRevision-1]
			if e.Date < original.Date || e.AmountCents > available[e.OriginalRevision] {
				return s, conflict("reversal_limit", "Check the reversal date and remaining amount")
			}
			if e.Kind == "void" {
				if original.Kind != "invoice" || e.AmountCents != original.AmountCents {
					return s, invalid("void", "Void a complete recorded invoice")
				}
				s.Invoiced -= e.AmountCents
				if original.DueDate != "" && original.DueDate < today {
					due -= e.AmountCents
				}
			} else {
				if original.Kind != "payment" {
					return s, invalid("refund", "Select the original receipt")
				}
				refunded = true
				s.Paid -= e.AmountCents
			}
			available[e.OriginalRevision] -= e.AmountCents
		default:
			return s, invalid("kind", "Unknown financial entry type")
		}
	}
	s.Balance = s.Payable - s.Paid
	s.Overdue = due > s.Paid
	switch {
	case s.Balance == 0:
		s.PaymentStatus = "paid"
	case refunded && s.Paid == 0:
		s.PaymentStatus = "refunded"
	case refunded:
		s.PaymentStatus = "partially_refunded"
	case s.Paid > 0:
		s.PaymentStatus = "partially_paid"
	}
	switch {
	case billed && s.Invoiced == 0:
		s.InvoiceStatus = "voided"
	case billed && s.Invoiced == s.Payable:
		s.InvoiceStatus = "recorded"
	case billed:
		s.InvoiceStatus = "partial"
	}
	return s, nil
}

type financeBasis struct {
	payable         int64
	estimatedProfit *int64
}

func proposalFinance(p Proposal) financeBasis {
	profit := proposalView(p).ProfitCents
	return financeBasis{payable: p.NetCents, estimatedProfit: &profit}
}

func quickSaleFinance(payable int64, pricing *QuickSalePricing) financeBasis {
	basis := financeBasis{payable: payable}
	if pricing != nil {
		profit := payable - pricing.TotalCostCents
		basis.estimatedProfit = &profit
	}
	return basis
}

func financeInputs(tx *gorm.DB, id uuid.UUID) (financeBasis, []FinanceEntry, error) {
	order, err := readRevision[OrderRevision](tx, id, 1)
	if err != nil {
		return financeBasis{}, nil, err
	}
	var basis financeBasis
	if order.ProposalID == uuid.Nil {
		var sale storage.SalesOrder
		if err := tx.Select("quick_sale_cents").Take(&sale, "id = ?", id).Error; err != nil {
			return basis, nil, err
		}
		if sale.QuickSaleCents == nil {
			return basis, nil, gorm.ErrRecordNotFound
		}
		basis = quickSaleFinance(*sale.QuickSaleCents, order.QuickSalePricing)
	} else {
		p, err := readRevision[Proposal](tx, order.ProposalID, order.ProposalRevision)
		if err != nil {
			return basis, nil, err
		}
		basis = proposalFinance(p)
	}
	entries := []FinanceEntry{}
	err = tx.Where("id = ?", id).Order("revision").Find(&entries).Error
	return basis, entries, err
}

func orderFinance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		var result FinanceSummary
		if err == nil {
			err = db.Transaction(func(tx *gorm.DB) error {
				basis, entries, err := financeInputs(tx, id)
				if err != nil {
					return err
				}
				result, err = summarizeFinance(basis, entries, time.Now().UTC().Format(time.DateOnly))
				return err
			})
		}
		reply(c, 200, result, err)
	}
}

func (s FinanceSummary) legacyPayment() storage.SalesOrderPaymentStatus {
	switch s.PaymentStatus {
	case "paid":
		return storage.SalesOrderPaymentPaid
	case "refunded":
		return storage.SalesOrderPaymentRefunded
	default:
		return storage.SalesOrderPaymentUnpaid
	}
}

func recordFinance(db *gorm.DB, id uuid.UUID, input FinanceCommand) (FinanceEntry, error) {
	return execute(db, "finance", id, input.Command, input, func(tx *gorm.DB) (FinanceEntry, uuid.UUID, int64, error) {
		basis, entries, err := financeInputs(tx, id)
		current := int64(len(entries))
		entry := FinanceEntry{ID: id, Revision: current + 1, FinanceInput: input.FinanceInput}
		if err != nil {
			return entry, id, 0, err
		}
		if current != input.ExpectedRevision {
			return entry, id, 0, stale(current)
		}
		s, err := summarizeFinance(basis, append(entries, entry), time.Now().UTC().Format(time.DateOnly))
		if err != nil {
			return entry, id, 0, err
		}
		if err := tx.Create(&entry).Error; err != nil {
			return entry, id, 0, err
		}
		err = tx.Model(&storage.SalesOrder{}).Where("id = ?", id).Update("payment_status", s.legacyPayment()).Error
		return entry, id, entry.Revision, err
	})
}
