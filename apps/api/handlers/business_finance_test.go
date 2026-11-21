package handlers_test

import (
	"reflect"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/google/uuid"
)

func TestOrderFinanceEntriesAndRestore(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	order := confirmedProductionOrder(t, router)
	root := businessOrders + "/" + order.ID.String()
	path := root + "/finance"
	read := func() budget.FinanceSummary {
		return businessRequest[budget.FinanceSummary](t, router, "GET", path, nil, 200)
	}
	initial := read()
	total := order.Profitability.RevenueCents
	if initial.Payable != total || initial.Balance != total || initial.PaymentStatus != "unpaid" || len(initial.Entries) != 0 {
		t.Fatal(initial)
	}
	var revision int64
	record := func(kind string, amount, original int64, status int) budget.FinanceCommand {
		req := budget.FinanceCommand{Command: budget.Command{OperationID: uuid.New(), ExpectedRevision: revision}, FinanceInput: budget.FinanceInput{
			Kind: kind, Reference: "Sample reference", Date: "2026-01-01", AmountCents: amount, OriginalRevision: original,
		}}
		if kind == "invoice" {
			req.DueDate = "2026-01-02"
		}
		if status == 200 {
			entry := businessRequest[budget.FinanceEntry](t, router, "POST", path, req, 200)
			replayed := businessRequest[budget.FinanceEntry](t, router, "POST", path, req, 200)
			revision++
			if !reflect.DeepEqual(entry, replayed) || entry.Revision != revision {
				t.Fatalf("invalid replay %v", replayed)
			}
		} else {
			businessRequest[apiResponse](t, router, "POST", path, req, status)
		}
		return req
	}
	record("invoice", total, 0, 200)
	record("payment", 101, 0, 200)
	if s := read(); s.Paid != 101 || s.Balance != total-101 || s.PaymentStatus != "partially_paid" || !s.Overdue || s.InvoiceStatus != "recorded" {
		t.Fatal(s)
	}
	record("payment", total, 0, 409)
	record("refund", 102, 2, 409)
	record("refund", 1, 1, 400)
	record("payment", -1, 0, 400)
	record("refund", 1, 2, 200)
	if s := read(); s.PaymentStatus != "partially_refunded" || s.Balance != total-100 {
		t.Fatal(s)
	}
	record("refund", 100, 2, 200)
	if s := read(); s.PaymentStatus != "refunded" || s.Balance != total {
		t.Fatal(s)
	}
	record("void", total, 1, 200)
	record("void", total, 1, 409)
	if s := read(); s.InvoiceStatus != "voided" || s.Overdue {
		t.Fatal(s)
	}
	if err := db.Exec("UPDATE sales_orders SET fulfilment_status='shipped' WHERE id=?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	record("invoice", total, 0, 200)
	if err := db.Exec("CREATE TRIGGER reject_projection BEFORE UPDATE OF payment_status ON sales_orders BEGIN SELECT RAISE(ABORT,'private'); END").Error; err != nil {
		t.Fatal(err)
	}
	req := record("payment", total, 0, 500)
	if s := read(); len(s.Entries) != int(revision) || s.Paid != 0 {
		t.Fatal("partial write", s)
	}
	if err := db.Exec("DROP TRIGGER reject_projection").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[budget.FinanceEntry](t, router, "POST", path, req, 200)
	final := read()
	detail := getSalesOrderDTO(t, router, order.ID)
	if final.Overdue || final.Balance != 0 || final.PaymentStatus != "paid" || detail.FulfilmentStatus != "shipped" || detail.PaymentStatus != "paid" || !reflect.DeepEqual(detail.Profitability, order.Profitability) {
		t.Fatal(final, detail)
	}
	target, _ := setupDesktopTestRouter(t)
	restore := func(status int) {
		archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes())
		if res.Code != status {
			t.Fatalf("restore %d: %s", res.Code, res.Body.String())
		}
	}
	restore(200)
	businessRequest[budget.FinanceEntry](t, target, "POST", path, req, 200)
	if err := db.Exec("UPDATE business_finance_entries SET amount_cents=-1 WHERE id=? AND revision=1", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	restore(400)
	if s := businessRequest[budget.FinanceSummary](t, target, "GET", path, nil, 200); !reflect.DeepEqual(s, final) {
		t.Fatal("restore lost history", s)
	}
}

func TestOrderFinanceConcurrentReceipts(t *testing.T) {
	router, _ := setupDesktopTestRouter(t)
	order := confirmedProductionOrder(t, router)
	path := businessOrders + "/" + order.ID.String() + "/finance"
	start := make(chan struct{})
	results := make(chan int, 2)
	for range 2 {
		go func() {
			<-start
			results <- performRequest(t, router, "POST", path, budget.FinanceCommand{Command: budget.Command{OperationID: uuid.New()}, FinanceInput: budget.FinanceInput{Kind: "payment", Date: "2026-01-02", Reference: "Deposit", AmountCents: order.Profitability.RevenueCents}}).Code
		}()
	}
	close(start)
	first, second := <-results, <-results
	if !(first == 200 && second == 409 || first == 409 && second == 200) {
		t.Fatalf("concurrent receipts returned %d, %d", first, second)
	}
	s := businessRequest[budget.FinanceSummary](t, router, "GET", path, nil, 200)
	if len(s.Entries) != 1 || s.Balance != 0 {
		t.Fatal(s)
	}
}
