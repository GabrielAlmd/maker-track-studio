package handlers_test

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func advanceSalesOrderToShipped(t *testing.T, router *gin.Engine, orderID uuid.UUID) {
	t.Helper()
	for _, status := range []string{"confirmed", "preparing", "ready_to_ship", "shipped"} {
		response := performRequest(t, router, http.MethodPut,
			"/sales-orders/"+orderID.String()+"/status",
			map[string]any{"fulfilment_status": status})
		if response.Code != http.StatusOK {
			t.Fatalf("transition to %s: %d %s", status, response.Code, response.Body.String())
		}
	}
}

func completeSalesOrderRequest(t *testing.T, router *gin.Engine, orderID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	return performRequest(t, router, http.MethodPut,
		"/sales-orders/"+orderID.String()+"/status",
		map[string]any{"fulfilment_status": "completed"})
}

func createSnapshotFixture(
	t *testing.T,
) (*gin.Engine, *gorm.DB, salesmodels.SalesChannel, models.Model, models.Printer, dto.PrintJobDTO, salesdto.SalesOrderDTO) {
	t.Helper()
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	printer := configurePrinter(t, router, addTestPrinter(t, router, "Snapshot printer", "A1"), 0, 0, 20)
	job := createCostedAllocationJob(t, router, printer.ID, product.ID, 10, 1)

	filament := models.Filament{Brand: "Snapshot", Type: "PLA", Color: "Blue", AvgPrice: 10}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	usage := models.PrintJobFilament{
		PrintJobID: job.ID,
		FilamentID: filament.ID,
		WeightUsed: 100,
	}
	if err := db.Create(&usage).Error; err != nil {
		t.Fatal(err)
	}

	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-SNAPSHOT", 5, 1000,
		[4]int64{100, 200, 300, 400},
	)
	allocation := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   3,
		}))
	if allocation.Code != http.StatusOK {
		t.Fatalf("allocate snapshot fixture: %d %s", allocation.Code, allocation.Body.String())
	}
	return router, db, channel, product, printer, job, order
}

func TestSalesCompletionCreatesFrozenSnapshotsWithoutRequiringFullAllocation(t *testing.T) {
	router, db, _, _, _, _, order := createSnapshotFixture(t)
	advanceSalesOrderToShipped(t, router, order.ID)

	response := completeSalesOrderRequest(t, router, order.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("complete order: %d %s", response.Code, response.Body.String())
	}
	var completed salesdto.SalesOrderDTO
	decodeBody(t, response, &completed)
	// Job total is €21 (€20 labor + €1 filament); 3/10 contributes €6.30.
	assertProfitabilityCents(t, completed.Profitability, 5000, 1000, 630, 1630, 3370, 67.4)
	if completed.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCompleted {
		t.Fatalf("order not completed: %#v", completed)
	}

	var persisted salesmodels.SalesOrder
	if err := db.First(&persisted, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ProductionCostCents == nil || *persisted.ProductionCostCents != 630 ||
		persisted.TotalCostCents == nil || *persisted.TotalCostCents != 1630 ||
		persisted.NetProfitCents == nil || *persisted.NetProfitCents != 3370 ||
		persisted.MarginBasisPoints == nil || *persisted.MarginBasisPoints != 6740 {
		t.Fatalf("order snapshot was not persisted: %#v", persisted)
	}

	var allocation salesmodels.SalesOrderAllocation
	if err := db.First(&allocation, "sales_order_item_id = ?", order.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.ProductionCostCents == nil || *allocation.ProductionCostCents != 630 {
		t.Fatalf("allocation cost snapshot was not persisted: %#v", allocation)
	}
}

func TestSalesCompletedOrderRejectsAllocationMutation(t *testing.T) {
	router, db, _, _, _, job, order := createSnapshotFixture(t)
	advanceSalesOrderToShipped(t, router, order.ID)
	completed := completeSalesOrderRequest(t, router, order.ID)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete order: %d %s", completed.Code, completed.Body.String())
	}

	response := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   2,
		}))
	if response.Code != http.StatusConflict {
		t.Fatalf("completed allocation mutation: %d %s", response.Code, response.Body.String())
	}

	var allocation salesmodels.SalesOrderAllocation
	if err := db.First(&allocation, "sales_order_item_id = ?", order.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.UnitsAllocated != 3 || allocation.ProductionCostCents == nil ||
		*allocation.ProductionCostCents != 630 {
		t.Fatalf("completed allocation snapshot changed: %#v", allocation)
	}
	result := getSalesOrderDTO(t, router, order.ID)
	assertProfitabilityCents(t, result.Profitability, 5000, 1000, 630, 1630, 3370, 67.4)
}

func TestSalesCompletedProfitabilityIgnoresMutableProductionData(t *testing.T) {
	router, db, _, product, printer, job, order := createSnapshotFixture(t)
	advanceSalesOrderToShipped(t, router, order.ID)
	response := completeSalesOrderRequest(t, router, order.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("complete order: %d %s", response.Code, response.Body.String())
	}
	before := getSalesOrderDTO(t, router, order.ID).Profitability
	assertProfitabilityCents(t, before, 5000, 1000, 630, 1630, 3370, 67.4)

	configurePrinter(t, router, printer, 50, 50, 999)
	if err := db.Model(&models.Filament{}).Where("brand = ?", "Snapshot").
		Update("avg_price", 1000).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Model{}).Where("id = ?", product.ID).
		Updates(map[string]any{"name": "Changed model", "print_time": 9999}).Error; err != nil {
		t.Fatal(err)
	}
	updateJob := performRequest(t, router, http.MethodPut,
		"/print-jobs/"+job.ID.String(), map[string]any{
			"printer_id":         printer.ID.String(),
			"model_id":           product.ID.String(),
			"print_date":         "2026-09-29T10:00:00Z",
			"units_printed":      10,
			"labor_hours":        10,
			"print_time_minutes": 600,
		})
	if updateJob.Code != http.StatusOK {
		t.Fatalf("mutate Print Job inputs: %d %s", updateJob.Code, updateJob.Body.String())
	}
	if err := db.Migrator().DropTable(&models.AppConfig{}); err != nil {
		t.Fatal(err)
	}

	after := getSalesOrderDTO(t, router, order.ID).Profitability
	assertProfitabilityCents(t, after, 5000, 1000, 630, 1630, 3370, 67.4)
}

func TestSalesCompletionRejectsUnavailableProductionCostWithoutChangingStatus(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 1)
	job.PrinterCost = &models.PrinterCostSnapshot{}
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-COST-MISSING", 1, 1000,
		[4]int64{},
	)
	allocation := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   1,
		}))
	if allocation.Code != http.StatusOK {
		t.Fatalf("allocate unavailable job: %d %s", allocation.Code, allocation.Body.String())
	}
	advanceSalesOrderToShipped(t, router, order.ID)

	response := completeSalesOrderRequest(t, router, order.ID)
	if response.Code != http.StatusConflict {
		t.Fatalf("unavailable completion: %d %s", response.Code, response.Body.String())
	}
	assertCompletionWasNotPersisted(t, db, order.ID, order.Items[0].ID)
}

func TestSalesCompletionRevalidatesPersistedAllocations(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, otherProduct := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 2)
	order := createProfitabilityOrder(
		t, router, channel.ID, product.ID, "ORDER-REVALIDATE", 2, 1000,
		[4]int64{},
	)
	allocation := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   2,
		}))
	if allocation.Code != http.StatusOK {
		t.Fatalf("allocate valid job: %d %s", allocation.Code, allocation.Body.String())
	}
	advanceSalesOrderToShipped(t, router, order.ID)
	if err := db.Model(&models.PrintJob{}).Where("id = ?", job.ID).
		Update("model_id", otherProduct.ID).Error; err != nil {
		t.Fatal(err)
	}

	response := completeSalesOrderRequest(t, router, order.ID)
	if response.Code != http.StatusConflict {
		t.Fatalf("invalid allocation completion: %d %s", response.Code, response.Body.String())
	}
	assertCompletionWasNotPersisted(t, db, order.ID, order.Items[0].ID)
}

func TestSalesCompletionSnapshotAndStatusAreAtomic(t *testing.T) {
	router, db, _, _, _, _, order := createSnapshotFixture(t)
	advanceSalesOrderToShipped(t, router, order.ID)

	callbackName := "test:fail_completed_sales_order_update"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "sales_orders" {
			tx.AddError(errors.New("forced completion failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(callbackName)

	response := completeSalesOrderRequest(t, router, order.ID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("forced completion failure: %d %s", response.Code, response.Body.String())
	}
	assertCompletionWasNotPersisted(t, db, order.ID, order.Items[0].ID)
}

func assertCompletionWasNotPersisted(t *testing.T, db *gorm.DB, orderID, itemID uuid.UUID) {
	t.Helper()
	var order salesmodels.SalesOrder
	if err := db.First(&order, "id = ?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	if order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentShipped ||
		order.ProductionCostCents != nil || order.TotalCostCents != nil ||
		order.NetProfitCents != nil || order.MarginBasisPoints != nil {
		t.Fatalf("failed completion changed order: %#v", order)
	}
	var allocation salesmodels.SalesOrderAllocation
	if err := db.First(&allocation, "sales_order_item_id = ?", itemID).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.ProductionCostCents != nil {
		t.Fatalf("failed completion left allocation snapshot: %#v", allocation)
	}
}
