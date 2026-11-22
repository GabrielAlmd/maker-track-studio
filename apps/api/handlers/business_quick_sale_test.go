package handlers_test

import (
	"reflect"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/google/uuid"
)

const quickSales = "/v2/business/quick-sales"

func TestQuickSaleMadeToOrderAndLegacyRoundTrip(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	legacy := createSalesBackupFixture(t, router, db)
	legacyPath := "/sales-orders/" + legacy.order.ID.String()
	oldOrder := getSalesOrderDTO(t, router, legacy.order.ID)
	oldAllocations := legacyOrderRequest[salesdto.SalesOrderAllocationsDTO](t, router, "GET", legacyPath+"/allocations", nil, 200)
	customer := businessRequest[budget.Customer](t, router, "POST", "/v2/business/customers", budget.CustomerCommand{
		Command: budget.Command{OperationID: uuid.New()}, CustomerInput: budget.CustomerInput{Name: "Synthetic buyer"},
	}, 201)
	command := budget.QuickSaleCommand{
		Command: budget.Command{OperationID: uuid.New()}, Reference: "No quote required",
		ModelID: *legacy.item.ModelID, CustomerID: customer.ID, Quantity: 3, UnitPriceCents: 1299,
	}
	invalid := command
	invalid.UnitPriceCents = 1 << 52
	businessRequest[apiResponse](t, router, "POST", quickSales, invalid, 400)
	invalid = command
	invalid.CustomerID = uuid.New()
	businessRequest[apiResponse](t, router, "POST", quickSales, invalid, 404)
	businessRequest[apiResponse](t, router, "POST", quickSales, map[string]any{"proposal_id": uuid.New()}, 400)
	sale := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201)
	if sale.ProposalID != uuid.Nil || sale.ConfirmedAt.IsZero() || sale.CustomerID != customer.ID || sale.CustomerName != "Synthetic buyer" {
		t.Fatalf("invalid direct confirmation: %+v", sale)
	}
	updated := budget.CustomerCommand{
		Command: budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, CustomerInput: budget.CustomerInput{Name: "Renamed buyer"},
	}
	businessRequest[budget.Customer](t, router, "PUT", "/v2/business/customers/"+customer.ID.String(), updated, 200)
	if replay := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201); !reflect.DeepEqual(sale, replay) {
		t.Fatal("retry changed the sale or customer snapshot")
	}
	invalid = command
	invalid.Reference = "Changed retry"
	businessRequest[apiResponse](t, router, "POST", quickSales, invalid, 409)
	detail := getSalesOrderDTO(t, router, sale.ID)
	if detail.Source != "quick_sale" || detail.WorkflowVersion != 2 || detail.LegacyWriteSupported ||
		detail.FulfilmentStatus != "confirmed" || detail.PaymentStatus != "unpaid" ||
		len(detail.Items) != 1 || detail.Items[0].UnitPriceCents != 1299 || detail.Profitability.RevenueCents != 3897 ||
		detail.Profitability.NetProfitCents != nil {
		t.Fatalf("unexpected sale %+v", detail)
	}
	legacyOrderRequest[apiResponse](t, router, "PUT", "/sales-orders/"+sale.ID.String()+"/status", map[string]string{"payment_status": "paid"}, 409)
	path := businessOrders + "/" + sale.ID.String()
	production := businessRequest[salesdto.OrderProduction](t, router, "GET", path+"/production", nil, 200)
	if len(production.Items) != 1 || production.Items[0].UnitsOutstanding != 3 || production.Items[0].UnitsAccepted != 0 {
		t.Fatal(production)
	}
	payment := budget.FinanceCommand{Command: budget.Command{OperationID: uuid.New()}, FinanceInput: budget.FinanceInput{
		Kind: "payment", Reference: "Synthetic receipt", Date: "2026-01-02", AmountCents: 3897,
	}}
	businessRequest[budget.FinanceEntry](t, router, "POST", path+"/finance", payment, 200)
	finance := businessRequest[budget.FinanceSummary](t, router, "GET", path+"/finance", nil, 200)
	if finance.Payable != 3897 || finance.Paid != 3897 || finance.EstimatedProfit != nil || len(finance.Entries) != 1 {
		t.Fatal(finance)
	}

	target, _ := setupDesktopTestRouter(t)
	restore := func(status int) {
		t.Helper()
		archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
		if archive.Code != 200 {
			t.Fatal(archive.Body.String())
		}
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes())
		if res.Code != status {
			t.Fatalf("restore %d: %s", res.Code, res.Body.String())
		}
	}
	for range 2 {
		restore(200)
	}
	if err := db.Exec("UPDATE sales_orders SET quick_sale_cents = quick_sale_cents + 1 WHERE id = ?", sale.ID).Error; err != nil {
		t.Fatal(err)
	}
	restore(400)
	if restored := getSalesOrderDTO(t, target, legacy.order.ID); !reflect.DeepEqual(oldOrder, restored) {
		t.Fatalf("legacy order changed on round trip: %+v", restored)
	}
	if restored := legacyOrderRequest[salesdto.SalesOrderAllocationsDTO](t, target, "GET", legacyPath+"/allocations", nil, 200); !reflect.DeepEqual(oldAllocations, restored) {
		t.Fatal("legacy allocations changed on round trip")
	}
	if restored := businessRequest[budget.FinanceSummary](t, target, "GET", path+"/finance", nil, 200); !reflect.DeepEqual(finance, restored) {
		t.Fatal("paid history changed on round trip")
	}
	if replay := businessRequest[budget.OrderRevision](t, target, "POST", quickSales, command, 201); !reflect.DeepEqual(sale, replay) {
		t.Fatal("operation identity changed on round trip")
	}
}

func TestQuickSaleStockDispatchRollbackAndConcurrentSales(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	_, product, other := createSalesOrderReferences(t, db)
	plan, job := planProduction(t, router, printer.ID, []models.PrintRunItem{{ModelID: &product.ID, Planned: 4}})
	completeProduction(t, router, plan, job.ID, func(item *models.PrintRunItem) { item.Attempted, item.Accepted = 4, 4 })
	source := plan.Runs[0].Items[0].ID
	stockPath := quickSales + "/stock/" + product.ID.String()
	type lots struct{ Items []salesdto.ProductionSource }
	if stock := businessRequest[lots](t, router, "GET", stockPath+"?quantity=4", nil, 200); len(stock.Items) != 1 || stock.Items[0].UnitsAvailable != 4 {
		t.Fatal(stock)
	}
	if stock := businessRequest[lots](t, router, "GET", stockPath+"?quantity=5", nil, 200); len(stock.Items) != 0 {
		t.Fatal(stock)
	}

	command := budget.QuickSaleCommand{
		Command: budget.Command{OperationID: uuid.New()}, Reference: "Counter sale",
		ModelID: product.ID, Quantity: 3, UnitPriceCents: 250, StockRunItemID: source,
	}
	wrongModel := command
	wrongModel.ModelID = other.ID
	businessRequest[apiResponse](t, router, "POST", quickSales, wrongModel, 409)
	if err := db.Exec(`CREATE TRIGGER reject_quick_dispatch BEFORE INSERT ON inventory_movements
		WHEN NEW.kind = 'dispatch' BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[apiResponse](t, router, "POST", quickSales, command, 500)
	assertSalesOrderCounts(t, db, 0, 0)
	if balance, err := inventory.OutputAvailability(db, product.ID, source); err != nil || balance.OnHand != 4 || balance.Reserved != 0 {
		t.Fatal(balance, err)
	}
	if err := db.Exec("DROP TRIGGER reject_quick_dispatch").Error; err != nil {
		t.Fatal(err)
	}

	otherSale := command
	otherSale.OperationID = uuid.New()
	start, results := make(chan struct{}), make(chan int, 2)
	for _, sale := range []budget.QuickSaleCommand{command, otherSale} {
		go func() {
			<-start
			results <- performRequest(t, router, "POST", quickSales, sale).Code
		}()
	}
	close(start)
	first, second := <-results, <-results
	if !(first == 201 && second == 409 || first == 409 && second == 201) {
		t.Fatalf("concurrent Quick Sales returned %d and %d", first, second)
	}
	assertSalesOrderCounts(t, db, 1, 1)
	if balance, err := inventory.OutputAvailability(db, product.ID, source); err != nil || balance.OnHand != 1 || balance.Reserved != 0 {
		t.Fatal(balance, err)
	}

	var sale budget.OrderRevision
	if err := db.Take(&sale).Error; err != nil {
		t.Fatal(err)
	}
	root := businessOrders + "/" + sale.ID.String()
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": false}, 200)
	disabledSale := command
	disabledSale.OperationID = uuid.New()
	disabledSale.StockRunItemID = uuid.Nil
	businessRequest[apiResponse](t, router, "POST", quickSales, disabledSale, 409)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	target, _ := setupDesktopTestRouter(t)
	if imported := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes()); imported.Code != 200 {
		t.Fatal(imported.Body.String())
	}
	fulfilment := businessRequest[apiResponse](t, target, "GET", root+"/fulfilment", nil, 200)
	activity := businessRequest[activityTestPage](t, target, "GET", root+"/activity", nil, 200)
	kinds := map[string]int{}
	for _, event := range activity.Items {
		kinds[event.Kind]++
	}
	if fulfilment["status"] != "shipped" || !reflect.DeepEqual(kinds, map[string]int{"reserve": 1, "dispatch": 1, "quick_sale_confirmed": 1}) {
		t.Fatal(fulfilment, activity)
	}
}

func TestQuickSaleUpgradeImportsSchemaSeventeen(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacy := createSalesBackupFixture(t, router, db)
	oldOrder := getSalesOrderDTO(t, router, legacy.order.ID)
	order := confirmedProductionOrder(t, router)
	financePath := businessOrders + "/" + order.ID.String() + "/finance"
	businessRequest[budget.FinanceEntry](t, router, "POST", financePath, budget.FinanceCommand{
		Command: budget.Command{OperationID: uuid.New()}, FinanceInput: budget.FinanceInput{
			Kind: "payment", Reference: "Historic receipt", Date: "2026-01-02", AmountCents: order.Profitability.RevenueCents,
		},
	}, 200)
	original := businessRequest[budget.FinanceSummary](t, router, "GET", financePath, nil, 200)
	for _, statement := range []string{
		"ALTER TABLE sales_orders DROP COLUMN quick_sale_cents",
		"ALTER TABLE business_order_revisions DROP COLUMN customer_name",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	target, _ := setupDesktopTestRouter(t)
	oldArchive := rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 17)
	if result := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", oldArchive); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if saved := getSalesOrderDTO(t, target, legacy.order.ID); !reflect.DeepEqual(oldOrder, saved) {
		t.Fatal("schema 17 archive changed a legacy order")
	}
	if restored := businessRequest[budget.FinanceSummary](t, target, "GET", financePath, nil, 200); !reflect.DeepEqual(original, restored) {
		t.Fatal("schema 17 archive changed proposal-backed payment history")
	}
}
