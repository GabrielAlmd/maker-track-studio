package handlers_test

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func createAllocationPrintJob(t *testing.T, db *gorm.DB, modelID uuid.UUID, units int) models.PrintJob {
	t.Helper()
	job := models.PrintJob{
		ModelID:      &modelID,
		PrintDate:    time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC),
		UnitsPrinted: units,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func createAllocationOrder(
	t *testing.T,
	router *gin.Engine,
	channelID, modelID uuid.UUID,
	reference string,
	quantity int,
) salesdto.SalesOrderDTO {
	t.Helper()
	payload := salesOrderRequest(channelID, []map[string]any{{
		"model_id": modelID.String(), "quantity": quantity, "unit_price_cents": 1000,
	}})
	payload["reference"] = reference
	return createSalesOrderForTest(t, router, payload)
}

func allocationDocument(values ...salesdto.SalesOrderAllocationRequest) map[string]any {
	allocations := make([]map[string]any, 0, len(values))
	for _, value := range values {
		allocations = append(allocations, map[string]any{
			"sales_order_item_id": value.SalesOrderItemID.String(),
			"print_job_id":        value.PrintJobID.String(),
			"units_allocated":     value.UnitsAllocated,
		})
	}
	return map[string]any{"allocations": allocations}
}

func TestSalesAllocationSupportsManyToManyAndAvailability(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	firstJob := createAllocationPrintJob(t, db, product.ID, 6)
	secondJob := createAllocationPrintJob(t, db, product.ID, 4)
	firstOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-1041", 5)
	secondOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-1048", 3)

	firstResponse := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+firstOrder.ID.String()+"/allocations",
		allocationDocument(
			salesdto.SalesOrderAllocationRequest{SalesOrderItemID: firstOrder.Items[0].ID, PrintJobID: firstJob.ID, UnitsAllocated: 2},
			salesdto.SalesOrderAllocationRequest{SalesOrderItemID: firstOrder.Items[0].ID, PrintJobID: secondJob.ID, UnitsAllocated: 3},
		))
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first allocation: %d %s", firstResponse.Code, firstResponse.Body.String())
	}

	secondResponse := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+secondOrder.ID.String()+"/allocations",
		allocationDocument(
			salesdto.SalesOrderAllocationRequest{SalesOrderItemID: secondOrder.Items[0].ID, PrintJobID: firstJob.ID, UnitsAllocated: 3},
		))
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second allocation: %d %s", secondResponse.Code, secondResponse.Body.String())
	}

	orderResponse := performRequest(t, router, http.MethodGet,
		"/sales-orders/"+firstOrder.ID.String()+"/allocations", nil)
	if orderResponse.Code != http.StatusOK {
		t.Fatalf("order allocations: %d %s", orderResponse.Code, orderResponse.Body.String())
	}
	var orderAllocations salesdto.SalesOrderAllocationsDTO
	decodeBody(t, orderResponse, &orderAllocations)
	if len(orderAllocations.Items) != 1 || orderAllocations.Items[0].UnitsAllocated != 5 ||
		orderAllocations.Items[0].UnitsAvailable != 0 || len(orderAllocations.Items[0].Allocations) != 2 {
		t.Fatalf("unexpected order allocation metadata: %#v", orderAllocations)
	}

	jobResponse := performRequest(t, router, http.MethodGet,
		"/print-jobs/"+firstJob.ID.String()+"/allocations", nil)
	if jobResponse.Code != http.StatusOK {
		t.Fatalf("print job allocations: %d %s", jobResponse.Code, jobResponse.Body.String())
	}
	var jobAllocations salesdto.PrintJobAllocationsDTO
	decodeBody(t, jobResponse, &jobAllocations)
	if jobAllocations.PrintJob.UnitsPrinted != 6 ||
		jobAllocations.PrintJob.UnitsAllocated != 5 ||
		jobAllocations.PrintJob.UnitsAvailable != 1 ||
		len(jobAllocations.UsedBy) != 2 {
		t.Fatalf("unexpected Print Job availability: %#v", jobAllocations)
	}
}

func TestSalesAllocationUpdateAndRemovalReplaceAtomically(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	firstJob := createAllocationPrintJob(t, db, product.ID, 10)
	secondJob := createAllocationPrintJob(t, db, product.ID, 10)
	order := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-UPDATE", 8)

	initial := allocationDocument(
		salesdto.SalesOrderAllocationRequest{SalesOrderItemID: order.Items[0].ID, PrintJobID: firstJob.ID, UnitsAllocated: 2},
		salesdto.SalesOrderAllocationRequest{SalesOrderItemID: order.Items[0].ID, PrintJobID: secondJob.ID, UnitsAllocated: 3},
	)
	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/allocations", initial)
	if response.Code != http.StatusOK {
		t.Fatalf("initial allocations: %d %s", response.Code, response.Body.String())
	}

	replacement := allocationDocument(
		salesdto.SalesOrderAllocationRequest{SalesOrderItemID: order.Items[0].ID, PrintJobID: firstJob.ID, UnitsAllocated: 6},
	)
	response = performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/allocations", replacement)
	if response.Code != http.StatusOK {
		t.Fatalf("replacement allocations: %d %s", response.Code, response.Body.String())
	}

	var allocations []salesmodels.SalesOrderAllocation
	if err := db.Where("sales_order_item_id = ?", order.Items[0].ID).Find(&allocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 1 || allocations[0].PrintJobID == nil ||
		*allocations[0].PrintJobID != firstJob.ID || allocations[0].UnitsAllocated != 6 {
		t.Fatalf("allocation set was not replaced: %#v", allocations)
	}

	response = performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations", allocationDocument())
	if response.Code != http.StatusOK {
		t.Fatalf("remove allocations: %d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&salesmodels.SalesOrderAllocation{}).
		Where("sales_order_item_id = ?", order.Items[0].ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected all allocations removed, got %d", count)
	}
}

func TestSalesAllocationRejectsCapacityAndModelConflicts(t *testing.T) {
	tests := []struct {
		name        string
		itemUnits   int
		jobUnits    int
		requested   int
		useMismatch bool
	}{
		{name: "order item capacity", itemUnits: 2, jobUnits: 10, requested: 3},
		{name: "Print Job capacity", itemUnits: 10, jobUnits: 2, requested: 3},
		{name: "Model mismatch", itemUnits: 3, jobUnits: 3, requested: 1, useMismatch: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, db := setupTestRouter(t)
			channel, product, otherProduct := createSalesOrderReferences(t, db)
			jobModelID := product.ID
			if test.useMismatch {
				jobModelID = otherProduct.ID
			}
			job := createAllocationPrintJob(t, db, jobModelID, test.jobUnits)
			order := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-CONFLICT", test.itemUnits)

			response := performRequest(t, router, http.MethodPut,
				"/sales-orders/"+order.ID.String()+"/allocations",
				allocationDocument(salesdto.SalesOrderAllocationRequest{
					SalesOrderItemID: order.Items[0].ID,
					PrintJobID:       job.ID,
					UnitsAllocated:   test.requested,
				}))
			if response.Code != http.StatusConflict {
				t.Fatalf("got %d, want 409: %s", response.Code, response.Body.String())
			}
			var count int64
			if err := db.Model(&salesmodels.SalesOrderAllocation{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("invalid allocation persisted: %d", count)
			}
		})
	}
}

func TestSalesAllocationCancelledOrderReleasesCapacity(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 5)
	firstOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-CANCELLED", 5)
	secondOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-ACTIVE", 5)

	response := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+firstOrder.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: firstOrder.Items[0].ID, PrintJobID: job.ID, UnitsAllocated: 5,
		}))
	if response.Code != http.StatusOK {
		t.Fatalf("first allocation: %d %s", response.Code, response.Body.String())
	}
	response = performRequest(t, router, http.MethodPut,
		"/sales-orders/"+secondOrder.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: secondOrder.Items[0].ID, PrintJobID: job.ID, UnitsAllocated: 1,
		}))
	if response.Code != http.StatusConflict {
		t.Fatalf("capacity before cancellation: %d %s", response.Code, response.Body.String())
	}

	cancel := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+firstOrder.ID.String()+"/status",
		map[string]any{"fulfilment_status": "cancelled"})
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel order: %d %s", cancel.Code, cancel.Body.String())
	}
	response = performRequest(t, router, http.MethodPut,
		"/sales-orders/"+secondOrder.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: secondOrder.Items[0].ID, PrintJobID: job.ID, UnitsAllocated: 5,
		}))
	if response.Code != http.StatusOK {
		t.Fatalf("capacity after cancellation: %d %s", response.Code, response.Body.String())
	}

	availability := performRequest(t, router, http.MethodGet,
		"/print-jobs/"+job.ID.String()+"/allocations", nil)
	var result salesdto.PrintJobAllocationsDTO
	decodeBody(t, availability, &result)
	if result.PrintJob.UnitsAllocated != 5 || result.PrintJob.UnitsAvailable != 0 ||
		len(result.UsedBy) != 1 || result.UsedBy[0].SalesOrderID != secondOrder.ID {
		t.Fatalf("cancelled allocation consumed capacity: %#v", result)
	}

	var retained int64
	if err := db.Model(&salesmodels.SalesOrderAllocation{}).
		Joins("JOIN sales_order_items ON sales_order_items.id = sales_order_allocations.sales_order_item_id").
		Where("sales_order_items.sales_order_id = ?", firstOrder.ID).
		Count(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("cancelled order did not retain allocation history: %d", retained)
	}
}

func TestSalesAllocationReplacementRollsBackOnPersistenceFailure(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	firstJob := createAllocationPrintJob(t, db, product.ID, 5)
	secondJob := createAllocationPrintJob(t, db, product.ID, 5)
	order := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-ROLLBACK", 5)

	response := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID, PrintJobID: firstJob.ID, UnitsAllocated: 2,
		}))
	if response.Code != http.StatusOK {
		t.Fatalf("initial allocation: %d %s", response.Code, response.Body.String())
	}

	callbackName := "test:fail_sales_allocation_create"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "sales_order_allocations" {
			tx.AddError(errors.New("forced allocation failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(callbackName)

	response = performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID, PrintJobID: secondJob.ID, UnitsAllocated: 4,
		}))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("forced failure: %d %s", response.Code, response.Body.String())
	}

	var allocations []salesmodels.SalesOrderAllocation
	if err := db.Where("sales_order_item_id = ?", order.Items[0].ID).Find(&allocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 1 || allocations[0].PrintJobID == nil ||
		*allocations[0].PrintJobID != firstJob.ID || allocations[0].UnitsAllocated != 2 {
		t.Fatalf("failed replacement left partial state: %#v", allocations)
	}
}

func TestSalesAllocationConcurrentRequestsCannotOverAllocate(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 10)
	firstOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-RACE-A", 7)
	secondOrder := createAllocationOrder(t, router, channel.ID, product.ID, "ORDER-RACE-B", 7)

	start := make(chan struct{})
	results := make(chan int, 2)
	var wait sync.WaitGroup
	for _, order := range []salesdto.SalesOrderDTO{firstOrder, secondOrder} {
		order := order
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			response := performRequest(t, router, http.MethodPut,
				"/sales-orders/"+order.ID.String()+"/allocations",
				allocationDocument(salesdto.SalesOrderAllocationRequest{
					SalesOrderItemID: order.Items[0].ID,
					PrintJobID:       job.ID,
					UnitsAllocated:   7,
				}))
			results <- response.Code
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	for status := range results {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected concurrent allocation status: %d", status)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one allocation to commit, got %d", successes)
	}

	var committed int64
	if err := db.Table("sales_order_allocations AS allocations").
		Select("COALESCE(SUM(allocations.units_allocated), 0)").
		Where("allocations.print_job_id = ?", job.ID).
		Scan(&committed).Error; err != nil {
		t.Fatal(err)
	}
	if committed > int64(job.UnitsPrinted) {
		t.Fatalf("committed over-allocation: %d > %d", committed, job.UnitsPrinted)
	}
}
