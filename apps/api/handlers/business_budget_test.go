package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const businessBudgets = "/v2/business/budgets"
const businessCustomers = "/v2/business/customers"

func businessRequest[T any](t *testing.T, router *gin.Engine, method, path string, body any, status int) T {
	t.Helper()
	response := performRequest(t, router, method, path, body)
	if response.Code != status || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("%s %s: got %d and cache header %q, want %d: %s", method, path, response.Code,
			response.Header().Get("Cache-Control"), status, response.Body.String())
	}
	var result T
	decodeBody(t, response, &result)
	return result
}

func budgetCommand() budget.BudgetCommand {
	return budget.BudgetCommand{
		Command: budget.Command{OperationID: uuid.New()},
		Header:  budget.Header{Title: "Sample production"},
		Items: []budget.ItemInput{{
			Name: "Prototype", Quantity: 2,
			Assumptions: budget.Assumptions{PostProcessCents: 125, OtherCents: 25},
		}},
	}
}

func TestBusinessBudgetCostAndRevisionHistory(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacy := createSalesBackupFixture(t, router, db)
	legacyPath := "/sales-orders/" + legacy.order.ID.String()
	before := performRequest(t, router, http.MethodGet, legacyPath, nil)

	fixture := createModernCostFixture(t, router)
	customerInput := budget.CustomerCommand{
		Command:       budget.Command{OperationID: uuid.New()},
		CustomerInput: budget.CustomerInput{Name: " Sample Customer ", Email: "orders@example.invalid"},
	}
	customer := businessRequest[budget.Customer](t, router, "POST", businessCustomers, customerInput, 201)
	if customer.Name != "Sample Customer" || customer.Revision != 1 || customer.ID == uuid.Nil {
		t.Fatalf("unexpected customer: %+v", customer)
	}

	payload := budgetCommand()
	payload.CustomerID = &customer.ID
	modelID := uuid.MustParse(fixture.ModelID)
	filamentID := uuid.MustParse(fixture.FilamentID)
	waste := 20.0
	payload.Items = append(payload.Items, budget.ItemInput{
		ModelID: &modelID, Quantity: 3,
		Assumptions: budget.Assumptions{
			PrinterID: &fixture.Printer.ID, PrintTimeMinutes: 60, LaborHours: 1,
			Materials: []budget.MaterialInput{{FilamentID: filamentID, WeightG: 100, WasteG: &waste}},
		},
	})
	record := businessRequest[budget.Record](t, router, "POST", businessBudgets, payload, 201)
	want := budget.Estimate{MaterialCents: 270, EnergyCents: 20, WearCents: 8, LaborCents: 1500, UnitCents: 1798, TotalCents: 5394}
	if record.ID == uuid.Nil || record.Revision != 1 || record.EstimatedCostCents != 5694 ||
		record.Items[0].ModelID != nil || record.Items[1].Name != "FlowModel" || record.Items[1].Estimate != want {
		t.Fatalf("unexpected estimate: %+v", record)
	}

	path := businessBudgets + "/" + record.ID.String()
	update := payload
	update.OperationID, update.ExpectedRevision = uuid.New(), 1
	update.Items = []budget.ItemInput{record.Items[0].ItemInput, record.Items[1].ItemInput}
	update.Items[1].Quantity = 4
	if err := db.Model(&models.Filament{}).Where("id = ?", filamentID).Update("avg_price", 100).Error; err != nil {
		t.Fatal(err)
	}
	edited := businessRequest[budget.Record](t, router, "PUT", path, update, 200)
	if edited.Revision != 2 || edited.Items[1].ID != record.Items[1].ID || edited.EstimatedCostCents != 7492 ||
		edited.Items[1].Sources.Materials[0].PricePerKg != 25 {
		t.Fatalf("edit lost line identity or cost snapshot: %+v", edited)
	}
	prior := businessRequest[budget.Record](t, router, "GET", path+"?revision=1", nil, 200)
	replayed := businessRequest[budget.Record](t, router, "POST", businessBudgets, payload, 201)
	if prior.EstimatedCostCents != 5694 || replayed.ID != record.ID || replayed.Revision != 1 || replayed.Items[1].ID != record.Items[1].ID {
		t.Fatal("create retry or historical read returned the wrong revision")
	}
	list := businessRequest[struct{ Items []budget.Record }](t, router, "GET", businessBudgets, nil, 200)
	if len(list.Items) != 1 || list.Items[0].Revision != 2 || list.Items[0].Items != nil {
		t.Fatalf("unexpected budget list: %+v", list)
	}

	customerInput.OperationID, customerInput.ExpectedRevision = uuid.New(), 1
	customerInput.Name, customerInput.IsArchived = "Retired customer", true
	customerPath := businessCustomers + "/" + customer.ID.String()
	changedCustomer := businessRequest[budget.Customer](t, router, "PUT", customerPath, customerInput, 200)
	if changedCustomer.ID != customer.ID || changedCustomer.Revision != 2 || !changedCustomer.IsArchived {
		t.Fatalf("unexpected customer edit: %+v", changedCustomer)
	}
	customers := businessRequest[struct{ Items []budget.Customer }](t, router, "GET", businessCustomers, nil, 200)
	if len(customers.Items) != 1 || customers.Items[0].ID != customer.ID || customers.Items[0].Revision != 2 {
		t.Fatalf("unexpected customer list: %+v", customers)
	}
	update.OperationID, update.ExpectedRevision, update.IsArchived = uuid.New(), 2, true
	businessRequest[budget.Record](t, router, "PUT", path, update, 200)
	update.OperationID, update.ExpectedRevision = uuid.New(), 3
	businessRequest[apiResponse](t, router, "PUT", path, update, 409)
	update.IsArchived = false
	businessRequest[budget.Record](t, router, "PUT", path, update, 200)
	after := performRequest(t, router, http.MethodGet, legacyPath, nil)
	if before.Code != 200 || after.Code != 200 || before.Body.String() != after.Body.String() {
		t.Fatal("Business changes altered the legacy Sales order response")
	}
}

func TestBusinessBudgetValidationAndAtomicRetry(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	for _, sample := range []struct {
		body string
		code int
	}{
		{`{} {}`, 400},
		{`{}` + strings.Repeat(" ", 512<<10), 413},
	} {
		request := httptest.NewRequest("POST", businessBudgets, strings.NewReader(sample.body))
		request.ContentLength = -1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != sample.code {
			t.Fatalf("invalid body: got %d, want %d", response.Code, sample.code)
		}
	}

	payload := budgetCommand()
	payload.Items[0].Assumptions.OtherCents = 1 << 53
	businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 400)
	payload.Items[0].Assumptions.OtherCents = 1<<53 - 1
	businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 400)
	payload.Items[0].Assumptions.OtherCents = 25
	payload.Items[0].ModelID = new(uuid.New())
	businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 404)
	payload.Items[0].ModelID = nil
	payload.Items[0].ID = uuid.New()
	businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 400)
	payload.Items[0].ID = uuid.Nil

	if err := db.Exec(`CREATE TRIGGER fail_business_operation BEFORE UPDATE ON business_budget_operations
		BEGIN SELECT RAISE(ABORT, 'private failure detail'); END`).Error; err != nil {
		t.Fatal(err)
	}
	failure := businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 500)
	encoded, err := json.Marshal(failure)
	if err != nil || strings.Contains(string(encoded), "private failure detail") {
		t.Fatalf("database error reached response: %s", encoded)
	}
	list := businessRequest[struct{ Items []budget.Record }](t, router, "GET", businessBudgets, nil, 200)
	if len(list.Items) != 0 {
		t.Fatal("failed operation left a budget behind")
	}
	if err := db.Exec("DROP TRIGGER fail_business_operation").Error; err != nil {
		t.Fatal(err)
	}
	created := businessRequest[budget.Record](t, router, "POST", businessBudgets, payload, 201)
	payload.Title = "Different request"
	businessRequest[apiResponse](t, router, "POST", businessBudgets, payload, 409)

	payload.OperationID, payload.ExpectedRevision = uuid.New(), 2
	payload.Items[0].ID = created.Items[0].ID
	path := businessBudgets + "/" + created.ID.String()
	businessRequest[apiResponse](t, router, "PUT", path, payload, 409)
	payload.ExpectedRevision = 1
	businessRequest[budget.Record](t, router, "PUT", path, payload, 200)
}

func TestBusinessBudgetBackupRoundTripAndValidation(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	customer := businessRequest[budget.Customer](t, source, "POST", businessCustomers, budget.CustomerCommand{
		Command:       budget.Command{OperationID: uuid.New()},
		CustomerInput: budget.CustomerInput{Name: "Backup sample"},
	}, 201)
	payload := budgetCommand()
	payload.CustomerID = &customer.ID
	created := businessRequest[budget.Record](t, source, "POST", businessBudgets, payload, 201)
	path := businessBudgets + "/" + created.ID.String()
	edit := budgetCommand()
	edit.CustomerID = &customer.ID
	edit.Items[0].ID = created.Items[0].ID
	edit.Items[0].Quantity = 5
	edit.ExpectedRevision = 1
	businessRequest[budget.Record](t, source, "PUT", path, edit, 200)
	archive := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	target, _ := setupDesktopTestRouter(t)
	importBackup := func(data []byte, status int) {
		t.Helper()
		result := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "business.zip", data)
		if result.Code != status {
			t.Fatalf("import: got %d, want %d: %s", result.Code, status, result.Body.String())
		}
	}
	for range 2 {
		importBackup(archive.Body.Bytes(), 200)
		restored := businessRequest[budget.Record](t, target, "GET", path, nil, 200)
		replayed := businessRequest[budget.Record](t, target, "POST", businessBudgets, payload, 201)
		restoredCustomer := businessRequest[budget.Customer](t, target, "GET", businessCustomers+"/"+customer.ID.String(), nil, 200)
		if restored.CustomerID == nil || *restored.CustomerID != customer.ID || restoredCustomer.Name != customer.Name ||
			restored.Revision != 2 || restored.Items[0].ID != created.Items[0].ID || restored.EstimatedCostCents != 750 ||
			replayed.ID != restored.ID || replayed.Revision != 1 || replayed.EstimatedCostCents != 300 {
			t.Fatal("restore lost revisions, cost, customer or retry history")
		}
	}

	if err := db.Model(&budget.Record{}).Where("id = ? AND revision = ?", created.ID, 2).
		Update("estimated_cost_cents", 1).Error; err != nil {
		t.Fatal(err)
	}
	corrupt := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if corrupt.Code != 200 {
		t.Fatal(corrupt.Body.String())
	}
	importBackup(corrupt.Body.Bytes(), 400)
	importBackup(rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 11), 400)
	retained := businessRequest[budget.Record](t, target, "GET", path, nil, 200)
	if retained.EstimatedCostCents != 750 {
		t.Fatal("invalid backup replaced target data")
	}

	legacy, legacyDB := setupDesktopTestRouter(t)
	for _, table := range budget.Tables() {
		if err := legacyDB.Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}
	legacyFixture := createSalesBackupFixture(t, legacy, legacyDB)
	legacyArchive := performRequest(t, legacy, "GET", "/desktop-data/export", nil)
	if legacyArchive.Code != 200 {
		t.Fatal(legacyArchive.Body.String())
	}
	importBackup(rewriteBackupSchemaVersion(t, legacyArchive.Body.Bytes(), 11), 200)
	businessRequest[apiResponse](t, target, "GET", path, nil, 404)
	order := performRequest(t, target, "GET", "/sales-orders/"+legacyFixture.order.ID.String(), nil)
	if order.Code != 200 {
		t.Fatal("schema 11 restore lost legacy Sales history")
	}
}
