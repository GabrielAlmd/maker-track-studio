package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"net/http"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func createCostedAllocationJob(
	t *testing.T,
	router *gin.Engine,
	printerID, modelID uuid.UUID,
	units int,
	laborHours float64,
) dto.PrintJobDTO {
	t.Helper()
	response := performRequest(t, router, http.MethodPost, "/print-jobs", map[string]any{
		"printer_id":         printerID.String(),
		"model_id":           modelID.String(),
		"print_date":         "2026-09-28T10:00:00Z",
		"units_printed":      units,
		"labor_hours":        laborHours,
		"print_time_minutes": 60,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create costed Print Job: %d %s", response.Code, response.Body.String())
	}
	var job dto.PrintJobDTO
	decodeBody(t, response, &job)
	return job
}

func createProfitabilityOrder(
	t *testing.T,
	router *gin.Engine,
	channelID, modelID uuid.UUID,
	reference string,
	quantity int,
	unitPriceCents int64,
	costs [4]int64,
) salesdto.SalesOrderDTO {
	t.Helper()
	payload := salesOrderRequest(channelID, []map[string]any{{
		"model_id": modelID.String(), "quantity": quantity, "unit_price_cents": unitPriceCents,
	}})
	payload["reference"] = reference
	payload["packaging_cost_cents"] = costs[0]
	payload["shipping_cost_cents"] = costs[1]
	payload["platform_fee_cents"] = costs[2]
	payload["other_cost_cents"] = costs[3]
	return createSalesOrderForTest(t, router, payload)
}

func getSalesOrderDTO(t *testing.T, router *gin.Engine, orderID uuid.UUID) salesdto.SalesOrderDTO {
	t.Helper()
	response := performRequest(t, router, http.MethodGet, "/sales-orders/"+orderID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get Sales Order: %d %s", response.Code, response.Body.String())
	}
	var order salesdto.SalesOrderDTO
	decodeBody(t, response, &order)
	return order
}

func assertProfitabilityCents(
	t *testing.T,
	actual salesdto.SalesOrderProfitabilityDTO,
	revenue, additional, production, total, net int64,
	margin float64,
) {
	t.Helper()
	if actual.State != "calculated" ||
		actual.RevenueCents != revenue ||
		actual.AdditionalCostsCents != additional ||
		actual.ProductionCostCents == nil || *actual.ProductionCostCents != production ||
		actual.TotalCostCents == nil || *actual.TotalCostCents != total ||
		actual.NetProfitCents == nil || *actual.NetProfitCents != net ||
		actual.Margin.State != "calculated" ||
		actual.Margin.Percent == nil || *actual.Margin.Percent != margin {
		t.Fatalf("unexpected profitability: %#v", actual)
	}
}

func TestSalesProfitabilityUsesLiveProportionalPrintJobCosts(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	printer := configurePrinter(t, router, addTestPrinter(t, router, "Sales costs", "A1"), 0, 0, 20)
	firstJob := createCostedAllocationJob(t, router, printer.ID, product.ID, 10, 1)
	secondJob := createCostedAllocationJob(t, router, printer.ID, product.ID, 5, 0.5)
	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-PROFIT", 5, 1000,
		[4]int64{100, 200, 300, 400},
	)

	allocationResponse := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(
			salesdto.SalesOrderAllocationRequest{
				SalesOrderItemID: order.Items[0].ID,
				PrintJobID:       firstJob.ID,
				UnitsAllocated:   3,
			},
			salesdto.SalesOrderAllocationRequest{
				SalesOrderItemID: order.Items[0].ID,
				PrintJobID:       secondJob.ID,
				UnitsAllocated:   2,
			},
		))
	if allocationResponse.Code != http.StatusOK {
		t.Fatalf("allocate production: %d %s", allocationResponse.Code, allocationResponse.Body.String())
	}

	live := getSalesOrderDTO(t, router, order.ID)
	// €20 / 10 × 3 = €6 and €10 / 5 × 2 = €4.
	assertProfitabilityCents(t, live.Profitability, 5000, 1000, 1000, 2000, 3000, 60)

	updateResponse := performRequest(t, router, http.MethodPut,
		"/print-jobs/"+firstJob.ID.String(), map[string]any{
			"printer_id":         printer.ID.String(),
			"model_id":           product.ID.String(),
			"print_date":         "2026-09-28T10:00:00Z",
			"units_printed":      10,
			"labor_hours":        2,
			"print_time_minutes": 60,
		})
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update live cost input: %d %s", updateResponse.Code, updateResponse.Body.String())
	}

	updated := getSalesOrderDTO(t, router, order.ID)
	// The first job is now €40, so its allocated share changes from €6 to €12.
	assertProfitabilityCents(t, updated.Profitability, 5000, 1000, 1600, 2600, 2400, 48)
}

func TestSalesProfitabilityKeepsUnavailableProductionCostDistinctFromZero(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	printTime := 60
	job := models.PrintJob{
		ModelID:          &product.ID,
		PrinterCost:      &models.PrinterCostSnapshot{},
		PrintDate:        time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC),
		UnitsPrinted:     1,
		PrintTimeMinutes: &printTime,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-UNAVAILABLE", 1, 1500,
		[4]int64{100, 0, 0, 0},
	)
	allocationResponse := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   1,
		}))
	if allocationResponse.Code != http.StatusOK {
		t.Fatalf("allocate unavailable job: %d %s", allocationResponse.Code, allocationResponse.Body.String())
	}

	result := getSalesOrderDTO(t, router, order.ID).Profitability
	if result.State != "not_calculated" || result.Reason == "" ||
		result.RevenueCents != 1500 || result.AdditionalCostsCents != 100 ||
		result.ProductionCostCents != nil || result.TotalCostCents != nil ||
		result.NetProfitCents != nil || result.Margin.State != "not_calculated" ||
		result.Margin.Percent != nil {
		t.Fatalf("unavailable production cost appeared calculated: %#v", result)
	}
}

func TestSalesProfitabilityZeroRevenueHasUnavailableFiniteMargin(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-ZERO-REVENUE", 2, 0,
		[4]int64{25, 25, 25, 25},
	)

	result := getSalesOrderDTO(t, router, order.ID).Profitability
	if result.State != "calculated" ||
		result.RevenueCents != 0 || result.AdditionalCostsCents != 100 ||
		result.ProductionCostCents == nil || *result.ProductionCostCents != 0 ||
		result.TotalCostCents == nil || *result.TotalCostCents != 100 ||
		result.NetProfitCents == nil || *result.NetProfitCents != -100 ||
		result.Margin.State != "not_calculated" || result.Margin.Reason == "" ||
		result.Margin.Percent != nil {
		t.Fatalf("zero-revenue result is invalid: %#v", result)
	}
}
