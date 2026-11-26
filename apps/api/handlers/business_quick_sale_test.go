package handlers_test

import (
	"reflect"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/gin-gonic/gin"
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
	cost := budget.QuickSaleEstimateInput{
		Source: "manual", ManualUnitCents: 200, Target: &budget.PriceSelection{Mode: "markup", Value: new(int64(2500))},
	}
	input := budget.QuickSaleEstimateRequest{ModelID: product.ID, StockRunItemID: source, Quantity: 3, Cost: cost}
	quote := businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", quickSales+"/estimate", input, 200)
	if quote.SuggestedUnitCents == nil || *quote.SuggestedUnitCents != 250 || quote.TotalCostCents != 600 {
		t.Fatal(quote)
	}
	command.Cost, command.EstimateToken = &cost, quote.Token
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
	var replays int
	for _, candidate := range []budget.QuickSaleCommand{command, otherSale} {
		if response := performRequest(t, router, "POST", quickSales, candidate); response.Code == 201 {
			replays++
			var replay budget.OrderRevision
			decodeBody(t, response, &replay)
			if !reflect.DeepEqual(sale, replay) {
				t.Fatal("dispatch replay changed the frozen terms")
			}
		} else if response.Code != 409 {
			t.Fatal(response.Body.String())
		}
	}
	if replays != 1 {
		t.Fatal("expected one successful replay after stock was consumed")
	}
	businessRequest[apiResponse](t, router, "POST", quickSales+"/estimate", input, 409)
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
	saved := businessRequest[budget.OrderRevision](t, target, "GET", root, nil, 200)
	finance := businessRequest[budget.FinanceSummary](t, target, "GET", root+"/finance", nil, 200)
	if !reflect.DeepEqual(saved.QuickSalePricing, sale.QuickSalePricing) || finance.EstimatedProfit == nil || *finance.EstimatedProfit != 150 {
		t.Fatal("restore changed the stock sale pricing or profit", saved, finance)
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

func TestQuickSaleModelEstimateRechecksAndFreezesPricing(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	filament := models.Filament{Brand: "Synthetic", Type: "PLA", Color: "Blue", AvgPrice: 50}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	product := requestModel(t, router, "POST", "/models", gin.H{
		"name": "Pricing model", "print_time": 1,
		"material_requirements": []gin.H{{"material_type": "PLA", "color": "Blue", "part_weight_g": 100, "waste_weight_g": 0}},
	}, 201)
	input := budget.QuickSaleEstimateRequest{ModelID: product.ID, Quantity: 2, Cost: budget.QuickSaleEstimateInput{
		Source: "model", MinutesPerUnit: 1, FilamentMapping: map[uuid.UUID]uuid.UUID{product.MaterialRequirements[0].ID: filament.ID},
		Target: &budget.PriceSelection{Mode: "margin", Value: new(int64(4000))},
	}}
	path := quickSales + "/estimate"
	businessRequest[apiResponse](t, router, "POST", path, input, 409)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	legacy := input
	legacy.Cost.FilamentMapping = nil
	businessRequest[apiResponse](t, router, "POST", path, legacy, 409)
	invalid := input
	invalid.Cost.Target = &budget.PriceSelection{Mode: "margin", Value: new(int64(10_000))}
	businessRequest[apiResponse](t, router, "POST", path, invalid, 400)
	invalid.Cost = budget.QuickSaleEstimateInput{Source: "manual", ManualUnitCents: 1 << 52}
	businessRequest[apiResponse](t, router, "POST", path, invalid, 400)
	quote := businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", path, input, 200)
	if quote.Production == nil || quote.Production.Estimate.MaterialCents < 500 || quote.SuggestedUnitCents == nil {
		t.Fatal(quote)
	}
	manual := input
	manual.Cost = budget.QuickSaleEstimateInput{Source: "manual", ManualUnitCents: 500, Target: input.Cost.Target}
	manualQuote := businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", path, manual, 200)
	if manualQuote.TotalCostCents != 1000 || *manualQuote.SuggestedUnitCents != 833 {
		t.Fatal(manualQuote)
	}
	command := budget.QuickSaleCommand{
		Command: budget.Command{OperationID: uuid.New()}, Reference: "Estimated sale", ModelID: product.ID,
		Quantity: input.Quantity, UnitPriceCents: 800, Cost: &input.Cost, EstimateToken: quote.Token,
	}
	updatePrice := func(price float64) {
		t.Helper()
		if err := db.Model(&filament).Update("avg_price", price).Error; err != nil {
			t.Fatal(err)
		}
	}
	updatePrice(60)
	businessRequest[apiResponse](t, router, "POST", quickSales, command, 409)
	assertSalesOrderCounts(t, db, 0, 0)
	quote = businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", path, input, 200)
	command.EstimateToken = quote.Token
	sale := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201)
	if !reflect.DeepEqual(sale.QuickSalePricing.Production.Estimate, quote.Production.Estimate) || sale.QuickSalePricing.UnitCostCents != quote.UnitCostCents {
		t.Fatal("confirmation did not freeze the accepted estimate", sale)
	}
	updatePrice(70)
	replay := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201)
	if !reflect.DeepEqual(sale, replay) {
		t.Fatal("retry repriced a confirmed sale")
	}
	root := businessOrders + "/" + sale.ID.String()
	finance := businessRequest[budget.FinanceSummary](t, router, "GET", root+"/finance", nil, 200)
	if finance.Payable != 1600 || finance.EstimatedProfit == nil || *finance.EstimatedProfit != 1600-quote.TotalCostCents {
		t.Fatal(finance)
	}
	amendment := budget.AmendmentCommand{
		Command:       budget.Command{OperationID: uuid.New(), ExpectedRevision: 1},
		DeliveryTerms: new("Collection by arrangement"), Reason: "Confirm collection",
	}
	amended := businessRequest[budget.OrderRevision](t, router, "POST", root+"/amendments", amendment, 200)
	if !reflect.DeepEqual(amended.QuickSalePricing, sale.QuickSalePricing) {
		t.Fatal("amending delivery terms changed the cost snapshot")
	}

	target, _ := setupDesktopTestRouter(t)
	restore := func(expected int) {
		t.Helper()
		archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes())
		if archive.Code != 200 || res.Code != expected {
			t.Fatalf("restore: export %d, import %d: %s", archive.Code, res.Code, res.Body)
		}
	}
	restore(200)
	if restored := businessRequest[budget.OrderRevision](t, target, "GET", root, nil, 200); !reflect.DeepEqual(amended, restored) {
		t.Fatal("backup restore changed the pricing snapshot", restored)
	}
	if err := db.Exec("UPDATE business_order_revisions SET quick_sale_pricing = json_set(quick_sale_pricing, '$.total_cost_cents', 1) WHERE id = ?", sale.ID).Error; err != nil {
		t.Fatal(err)
	}
	restore(400)
	if restored := businessRequest[budget.FinanceSummary](t, target, "GET", root+"/finance", nil, 200); !reflect.DeepEqual(finance, restored) {
		t.Fatal("invalid backup changed confirmed finance")
	}
}

func TestQuickSaleRestoresSchema21WithoutPricing(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	_, product, _ := createSalesOrderReferences(t, db)
	command := budget.QuickSaleCommand{
		Command: budget.Command{OperationID: uuid.New()}, Reference: "Legacy manual price",
		ModelID: product.ID, UnitPriceCents: 1500, Quantity: 1,
	}
	sale := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201)
	for _, statement := range []string{
		"ALTER TABLE business_order_revisions DROP COLUMN quick_sale_pricing",
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
	old := rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 21)
	if restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", old); restored.Code != 200 {
		t.Fatal(restored.Body.String())
	}
	if replay := businessRequest[budget.OrderRevision](t, target, "POST", quickSales, command, 201); !reflect.DeepEqual(sale, replay) {
		t.Fatal("migration changed a manual sale or its retry identity")
	}
	finance := businessRequest[budget.FinanceSummary](t, target, "GET", businessOrders+"/"+sale.ID.String()+"/finance", nil, 200)
	if finance.Payable != 1500 || finance.EstimatedProfit != nil {
		t.Fatal(finance)
	}
}

func TestQuickSalePricesExplicitFilamentChoices(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	legacyOrderRequest[map[string]bool](t, router, "PUT", "/features", map[string]bool{"sales_orders": true}, 200)
	red := models.Filament{Brand: "Synthetic", Type: "PLA", Color: "Red", AvgPrice: 50}
	badType := models.Filament{Brand: "Synthetic", Type: "PETG", Color: "Blue", AvgPrice: 50}
	unpriced := models.Filament{Brand: "Synthetic", Type: "PLA", Color: "Blue"}
	archived := models.Filament{Brand: "Former", Type: "PLA", Color: "Blue", AvgPrice: 50, Archived: true}
	for _, filament := range []*models.Filament{&red, &badType, &unpriced, &archived} {
		if err := db.Create(filament).Error; err != nil {
			t.Fatal(err)
		}
	}
	structured := requestModel(t, router, "POST", "/models", gin.H{
		"name": "Structured model", "print_time": 15,
		"material_requirements": []gin.H{{"material_type": "PLA", "color": "Blue", "part_weight_g": 100, "waste_weight_g": 0}},
	}, 201)
	legacy := requestModel(t, router, "POST", "/models", gin.H{
		"name": "Older model", "print_time": 15, "material_weight": 100, "waste_material": 5,
	}, 201)
	multi := requestModel(t, router, "POST", "/models", gin.H{
		"name": "Older multicolor model", "print_time": 15, "material_weight": 100, "is_multicolor": true,
	}, 201)

	path := quickSales + "/estimate"
	structuredInput := budget.QuickSaleEstimateRequest{ModelID: structured.ID, Quantity: 1, Cost: budget.QuickSaleEstimateInput{
		Source: "model", MinutesPerUnit: 15, FilamentMapping: map[uuid.UUID]uuid.UUID{structured.MaterialRequirements[0].ID: red.ID},
	}}
	businessRequest[apiResponse](t, router, "POST", path, structuredInput, 409)
	structuredInput.Cost.AllowColorSubstitution = true
	quote := businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", path, structuredInput, 200)
	if quote.Production.Assumptions.Materials[0].FilamentID != red.ID {
		t.Fatal("the estimate did not use the chosen color")
	}
	for _, rejected := range []uuid.UUID{badType.ID, unpriced.ID, archived.ID} {
		structuredInput.Cost.FilamentMapping[structured.MaterialRequirements[0].ID] = rejected
		businessRequest[apiResponse](t, router, "POST", path, structuredInput, 409)
	}

	input := budget.QuickSaleEstimateRequest{ModelID: legacy.ID, Quantity: 2, Cost: budget.QuickSaleEstimateInput{
		Source: "model", MinutesPerUnit: 15,
	}}
	businessRequest[apiResponse](t, router, "POST", path, input, 409)
	input.Cost.LegacyFilamentID = &red.ID
	preview := businessRequest[budget.QuickSaleEstimateResult](t, router, "POST", path, input, 200)
	assumptions := preview.Production.Assumptions.Materials
	if len(assumptions) != 1 || assumptions[0].FilamentID != red.ID ||
		assumptions[0].WeightG != 105 || assumptions[0].WasteG == nil || *assumptions[0].WasteG != 5 {
		t.Fatalf("legacy cost did not use the full saved weight: %+v", assumptions)
	}
	input.ModelID = multi.ID
	businessRequest[apiResponse](t, router, "POST", path, input, 409)

	command := budget.QuickSaleCommand{
		Command: budget.Command{OperationID: uuid.New()}, Reference: "Chosen filament sale", ModelID: legacy.ID,
		Quantity: input.Quantity, UnitPriceCents: 1000, Cost: &input.Cost, EstimateToken: preview.Token,
	}
	sale := businessRequest[budget.OrderRevision](t, router, "POST", quickSales, command, 201)
	if sale.QuickSalePricing == nil || !reflect.DeepEqual(sale.QuickSalePricing.Production.Assumptions.Materials, assumptions) {
		t.Fatal("confirmation did not retain the chosen material cost")
	}
	unchanged := requestModel(t, router, "GET", "/models/"+legacy.ID.String(), nil, 200)
	if len(unchanged.MaterialRequirements) != 0 || unchanged.MaterialWeight != 100 || unchanged.WasteMaterial != 5 {
		t.Fatal("pricing changed the saved model materials")
	}
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	target, _ := setupDesktopTestRouter(t)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	if restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes()); restored.Code != 200 {
		t.Fatal(restored.Body.String())
	}
	if replay := businessRequest[budget.OrderRevision](t, target, "POST", quickSales, command, 201); !reflect.DeepEqual(sale, replay) {
		t.Fatal("restore changed the chosen material or retry identity")
	}
}
