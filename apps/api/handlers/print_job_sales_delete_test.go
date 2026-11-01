package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/gorm"
)

func TestPrintJobDeletionAllowsUnreferencedJob(t *testing.T) {
	router, db := setupTestRouter(t)
	_, product, _ := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 1)

	response := performRequest(t, router, http.MethodDelete, "/print-jobs/"+job.ID.String(), nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete unreferenced job: %d %s", response.Code, response.Body.String())
	}
	if err := db.First(&models.PrintJob{}, "id = ?", job.ID).Error; err != gorm.ErrRecordNotFound {
		t.Fatalf("unreferenced job was not deleted: %v", err)
	}
}

func TestPrintJobDeletionRejectsSalesReferencedJob(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	job := createAllocationPrintJob(t, db, product.ID, 2)
	order := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": product.ID.String(), "quantity": 1, "unit_price_cents": 1000},
	}))
	allocation := performRequest(t, router, http.MethodPut,
		"/sales-orders/"+order.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: order.Items[0].ID,
			PrintJobID:       job.ID,
			UnitsAllocated:   1,
		}))
	if allocation.Code != http.StatusOK {
		t.Fatalf("allocate job: %d %s", allocation.Code, allocation.Body.String())
	}

	response := performRequest(t, router, http.MethodDelete, "/print-jobs/"+job.ID.String(), nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete referenced job: %d %s", response.Code, response.Body.String())
	}
	assertPrintJobAndAllocationRemain(t, db, job.ID.String(), order.Items[0].ID.String())
}

func TestPrintJobDeletionRejectsCompletedHistoricalReference(t *testing.T) {
	router, db, _, _, _, job, order := createSnapshotFixture(t)
	advanceSalesOrderToShipped(t, router, order.ID)
	completed := completeSalesOrderRequest(t, router, order.ID)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete order: %d %s", completed.Code, completed.Body.String())
	}

	response := performRequest(t, router, http.MethodDelete, "/print-jobs/"+job.ID.String(), nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete completed-order job: %d %s", response.Code, response.Body.String())
	}
	assertPrintJobAndAllocationRemain(t, db, job.ID.String(), order.Items[0].ID.String())
}

func assertPrintJobAndAllocationRemain(t *testing.T, db *gorm.DB, jobID, itemID string) {
	t.Helper()
	if err := db.First(&models.PrintJob{}, "id = ?", jobID).Error; err != nil {
		t.Fatalf("protected Print Job changed: %v", err)
	}
	var count int64
	if err := db.Model(&salesmodels.SalesOrderAllocation{}).
		Where("sales_order_item_id = ? AND print_job_id = ?", itemID, jobID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("protected Sales allocation changed: %d", count)
	}
}
