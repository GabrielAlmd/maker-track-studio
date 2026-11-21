package handlers_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const businessProposals = "/v2/business/proposals"

func seedProposal(t *testing.T, router *gin.Engine) (budget.ProposalCommand, budget.Record) {
	t.Helper()
	customer := businessRequest[budget.Customer](t, router, "POST", businessCustomers, budget.CustomerCommand{
		Command: budget.Command{OperationID: uuid.New()},
		CustomerInput: budget.CustomerInput{
			Name: "Sample customer", Address: "Sample address", Email: "internal@example.invalid",
		},
	}, 201)
	estimate := budgetCommand()
	estimate.Title = "Private project name"
	estimate.CustomerID = &customer.ID
	estimate.Items = []budget.ItemInput{{
		Name: "Private print process", Quantity: 2, Assumptions: budget.Assumptions{OtherCents: 800},
	}, {
		Name: "Private finishing process", Quantity: 1, Assumptions: budget.Assumptions{OtherCents: 300},
	}}
	built := businessRequest[budget.Record](t, router, "POST", businessBudgets, estimate, 201)
	return budget.ProposalCommand{
		Command:  budget.Command{OperationID: uuid.New()},
		BudgetID: built.ID, BudgetRevision: built.Revision, CustomerID: customer.ID,
		Terms: budget.ProposalTerms{
			Title: "Prototype quote", ValidUntil: time.Now().UTC().AddDate(0, 0, 10).Format(time.DateOnly),
			LeadTimeDays: 7, DeliveryTerms: "Collection", PaymentTerms: "Due on delivery",
		},
		Lines: []budget.ProposalLineInput{
			{BudgetLineID: built.Items[0].ID, Description: "Prototype housing",
				Price:    budget.PriceSelection{Mode: "margin", Value: new(int64(2000))},
				Discount: budget.Discount{Mode: "percent", Value: 500}},
			{BudgetLineID: built.Items[1].ID, Description: "Surface finish",
				Price: budget.PriceSelection{Mode: "manual", Value: new(int64(600))}},
		},
		OrderDiscount: budget.Discount{Mode: "fixed", Value: 100},
	}, built
}

func TestBusinessProposalPricingHistoryAndCustomerDocument(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacy := createSalesBackupFixture(t, router, db)
	legacyPath := "/sales-orders/" + legacy.order.ID.String()
	before := performRequest(t, router, "GET", legacyPath, nil)
	request, estimate := seedProposal(t, router)
	first := businessRequest[budget.ProposalView](t, router, "POST", businessProposals, request, 201)
	if first.ID == uuid.Nil || first.Revision != 1 || first.EstimatedCostCents != 1900 ||
		first.GrossCents != 2600 || first.LineDiscountCents != 100 || first.OrderDiscountCents != 100 ||
		first.NetCents != 2400 || first.ProfitCents != 500 || first.MarginPercent == nil || first.MarkupPercent == nil ||
		first.Lines[0].OrderDiscountCents != 76 || first.Lines[0].NetCents != 1824 || first.Lines[1].NetCents != 576 {
		t.Fatalf("unexpected proposal: %+v", first)
	}

	path := businessProposals + "/" + first.ID.String()
	businessRequest[apiResponse](t, router, "GET", path+"/document", nil, 400)
	businessRequest[apiResponse](t, router, "GET", path+"/document?revision=1", nil, 404)
	issue := budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}
	issued := businessRequest[budget.ProposalView](t, router, "POST", path+"/issue", issue, 200)
	if issued.Revision != 2 || issued.IssuedAt == nil || issued.NetCents != first.NetCents {
		t.Fatalf("issuing did not freeze preview: %+v", issued)
	}
	documentPath := path + "/document?revision=2"
	document := businessRequest[map[string]any](t, router, "GET", documentPath, nil, 200)
	assertKeys(t, document, "id", "revision", "currency", "customer_name", "customer_address",
		"terms", "lines", "line_discount_cents", "order_discount_cents", "net_cents", "issued_at")
	assertKeys(t, document["terms"].(map[string]any), "title", "valid_until", "due_date",
		"lead_time_days", "delivery_terms", "payment_terms")
	line := document["lines"].([]any)[0].(map[string]any)
	assertKeys(t, line, "description", "quantity", "line_discount_cents", "order_discount_cents", "net_cents")
	if document["currency"] != "EUR" || document["customer_name"] != "Sample customer" ||
		document["net_cents"] != float64(2400) || line["description"] != "Prototype housing" {
		t.Fatalf("unexpected customer document: %#v", document)
	}

	businessRequest[budget.Customer](t, router, "PUT", businessCustomers+"/"+request.CustomerID.String(), budget.CustomerCommand{
		Command:       budget.Command{OperationID: uuid.New(), ExpectedRevision: 1},
		CustomerInput: budget.CustomerInput{Name: "Updated customer"},
	}, 200)
	changed := budgetCommand()
	changed.Command.ExpectedRevision = 1
	changed.CustomerID = &request.CustomerID
	changed.Items = []budget.ItemInput{estimate.Items[0].ItemInput, estimate.Items[1].ItemInput}
	changed.Items[0].Quantity = 3
	businessRequest[budget.Record](t, router, "PUT", businessBudgets+"/"+estimate.ID.String(), changed, 200)
	request.OperationID, request.ExpectedRevision, request.BudgetRevision = uuid.New(), 2, 2
	request.Terms.PaymentTerms = "Due before collection"
	next := businessRequest[budget.ProposalView](t, router, "PUT", path, request, 200)
	if next.Revision != 3 || next.IssuedAt != nil || next.EstimatedCostCents != 2700 ||
		next.CustomerRevision != 2 || next.Lines[0].BudgetLineID != issued.Lines[0].BudgetLineID {
		t.Fatalf("unexpected replacement draft: %+v", next)
	}
	latest := businessRequest[struct{ Items []budget.Proposal }](t, router, "GET", businessProposals, nil, 200)
	if len(latest.Items) != 1 || latest.Items[0].Revision != 3 || latest.Items[0].Terms.Title != request.Terms.Title ||
		latest.Items[0].Lines != nil {
		t.Fatalf("unexpected proposal list: %+v", latest)
	}
	businessRequest[apiResponse](t, router, "PUT", path, budget.ProposalCommand{
		Command:  budget.Command{OperationID: uuid.New(), ExpectedRevision: 2},
		BudgetID: request.BudgetID, BudgetRevision: 2, CustomerID: request.CustomerID,
	}, 409)
	newIssue := budget.Command{OperationID: uuid.New(), ExpectedRevision: 3}
	replacement := businessRequest[budget.ProposalView](t, router, "POST", path+"/issue", newIssue, 200)
	retry := businessRequest[budget.ProposalView](t, router, "POST", path+"/issue", issue, 200)
	if replacement.Revision != 4 || replacement.SupersedesRevision != 2 || retry.Revision != 2 {
		t.Fatalf("incorrect supersession or retry: %+v, %+v", replacement, retry)
	}
	history := businessRequest[struct {
		Items   []budget.Proposal
		HasMore bool `json:"has_more"`
	}](t, router, "GET", path+"/revisions?page_size=2&page=2", nil, 200)
	if len(history.Items) != 2 || history.Items[0].Revision != 2 || history.Items[1].Revision != 1 || history.HasMore {
		t.Fatalf("incorrect revision history: %+v", history)
	}
	saved := businessRequest[map[string]any](t, router, "GET", documentPath, nil, 200)
	after := performRequest(t, router, "GET", legacyPath, nil)
	if !reflect.DeepEqual(saved, document) || before.Code != 200 || after.Code != 200 ||
		before.Body.String() != after.Body.String() {
		t.Fatal("proposal changes altered an issued document or legacy Sales history")
	}
}

func assertKeys(t *testing.T, data map[string]any, keys ...string) {
	t.Helper()
	if len(data) != len(keys) {
		t.Fatalf("unexpected customer document keys: %#v", data)
	}
	for _, key := range keys {
		if _, ok := data[key]; !ok {
			t.Fatalf("customer document is missing %q: %#v", key, data)
		}
	}
}

func TestBusinessProposalValidationAndRollback(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	request, _ := seedProposal(t, router)
	for _, tc := range []struct {
		name   string
		change func(*budget.ProposalCommand)
		status int
	}{
		{"wrong budget revision", func(p *budget.ProposalCommand) { p.BudgetRevision++ }, 404},
		{"duplicate line", func(p *budget.ProposalCommand) { p.Lines[1] = p.Lines[0] }, 400},
		{"missing line price", func(p *budget.ProposalCommand) { p.Lines[0].Price.Value = nil }, 400},
		{"line discount exceeds subtotal", func(p *budget.ProposalCommand) {
			p.Lines[0].Discount = budget.Discount{Mode: "fixed", Value: 2001}
		}, 400},
		{"order discount exceeds remaining subtotal", func(p *budget.ProposalCommand) { p.OrderDiscount.Value = 2501 }, 400},
		{"unsafe subtotal", func(p *budget.ProposalCommand) {
			p.Lines[0].Price = budget.PriceSelection{Mode: "manual", Value: new(int64(1<<53 - 1))}
		}, 400},
		{"invalid date", func(p *budget.ProposalCommand) { p.Terms.ValidUntil = "2026-02-30" }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := request
			payload.Lines = append([]budget.ProposalLineInput(nil), request.Lines...)
			tc.change(&payload)
			businessRequest[apiResponse](t, router, "POST", businessProposals, payload, tc.status)
		})
	}
	created := businessRequest[budget.ProposalView](t, router, "POST", businessProposals, request, 201)
	path := businessProposals + "/" + created.ID.String()
	issue := budget.Command{OperationID: request.OperationID, ExpectedRevision: 1}
	businessRequest[apiResponse](t, router, "POST", path+"/issue", issue, 409)
	issue.OperationID = uuid.New()

	if err := db.Exec(`CREATE TRIGGER fail_proposal_operation BEFORE UPDATE ON business_budget_operations
		BEGIN SELECT RAISE(ABORT, 'private failure detail'); END`).Error; err != nil {
		t.Fatal(err)
	}
	failure := performRequest(t, router, "POST", path+"/issue", issue)
	if failure.Code != 500 || strings.Contains(failure.Body.String(), "private failure detail") {
		t.Fatalf("unexpected failure response: %d %s", failure.Code, failure.Body.String())
	}
	reloaded := businessRequest[budget.ProposalView](t, router, "GET", path, nil, 200)
	if reloaded.Revision != 1 || reloaded.IssuedAt != nil {
		t.Fatal("failed issue left an issued revision behind")
	}
	if err := db.Exec("DROP TRIGGER fail_proposal_operation").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[budget.ProposalView](t, router, "POST", path+"/issue", issue, 200)
	issue.OperationID, issue.ExpectedRevision = uuid.New(), 2
	businessRequest[apiResponse](t, router, "POST", path+"/issue", issue, 409)

	request.OperationID, request.ExpectedRevision = uuid.New(), 2
	request.Terms.ValidUntil = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	businessRequest[budget.ProposalView](t, router, "PUT", path, request, 200)
	issue.OperationID, issue.ExpectedRevision = uuid.New(), 3
	businessRequest[apiResponse](t, router, "POST", path+"/issue", issue, 400)
}

func TestBusinessProposalBackupRoundTrip(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	target, _ := setupDesktopTestRouter(t)
	request, estimate := seedProposal(t, source)
	export := func() []byte {
		t.Helper()
		archive := performRequest(t, source, "GET", "/desktop-data/export", nil)
		if archive.Code != 200 {
			t.Fatal(archive.Body.String())
		}
		return archive.Body.Bytes()
	}
	restore := func(archive []byte, status int) {
		t.Helper()
		response := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "business.zip", archive)
		if response.Code != status {
			t.Fatalf("restore: got %d, want %d: %s", response.Code, status, response.Body.String())
		}
	}
	// Build a schema 12 archive with customer and budget data and no proposal table.
	if err := db.Migrator().DropTable(&budget.Proposal{}); err != nil {
		t.Fatal(err)
	}
	legacy := rewriteBackupSchemaVersion(t, export(), 12)
	if err := db.AutoMigrate(&budget.Proposal{}); err != nil {
		t.Fatal(err)
	}

	created := businessRequest[budget.ProposalView](t, source, "POST", businessProposals, request, 201)
	path := businessProposals + "/" + created.ID.String()
	issue := budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}
	businessRequest[budget.ProposalView](t, source, "POST", path+"/issue", issue, 200)
	archive := export()
	for range 2 {
		restore(archive, 200)
		replayed := businessRequest[budget.ProposalView](t, target, "POST", path+"/issue", issue, 200)
		if replayed.ID != created.ID || replayed.Revision != 2 || replayed.IssuedAt == nil {
			t.Fatal("restore did not preserve issued revision or retry record")
		}
	}
	restore(rewriteBackupSchemaVersion(t, archive, 12), 400)
	if err := db.Model(&budget.Proposal{}).Where("id = ? AND revision = ?", created.ID, 2).Update("net_cents", 1).Error; err != nil {
		t.Fatal(err)
	}
	restore(export(), 400)
	doc := businessRequest[budget.ProposalDocument](t, target, "GET", path+"/document?revision=2", nil, 200)
	if doc.NetCents != 2400 {
		t.Fatal("invalid archive replaced the existing proposal")
	}
	restore(legacy, 200)
	businessRequest[apiResponse](t, target, "GET", path, nil, 404)
	prior := businessRequest[budget.Record](t, target, "GET", businessBudgets+"/"+estimate.ID.String(), nil, 200)
	if prior.ID != estimate.ID || prior.EstimatedCostCents != estimate.EstimatedCostCents {
		t.Fatal("schema 12 restore lost the original budget")
	}
}
