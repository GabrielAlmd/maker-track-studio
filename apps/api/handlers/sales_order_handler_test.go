package handlers_test

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func salesOrderRequest(channelID uuid.UUID, items []map[string]any) map[string]any {
	return map[string]any{
		"reference":            "ORDER-1048",
		"order_date":           "2026-09-28T00:00:00Z",
		"sales_channel_id":     channelID.String(),
		"fulfilment_status":    "draft",
		"payment_status":       "unpaid",
		"packaging_cost_cents": 120,
		"shipping_cost_cents":  450,
		"platform_fee_cents":   215,
		"other_cost_cents":     0,
		"notes":                "  Pack carefully  ",
		"items":                items,
	}
}

func createSalesOrderReferences(t *testing.T, db *gorm.DB) (salesmodels.SalesChannel, models.Model, models.Model) {
	t.Helper()
	channel := salesmodels.SalesChannel{Name: "Online shop"}
	first := models.Model{Name: "Articulated dragon", MaterialWeight: 80, PrintTime: 240}
	second := models.Model{Name: "Plant pot", MaterialWeight: 120, PrintTime: 360}
	for _, value := range []any{&channel, &first, &second} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	return channel, first, second
}

func TestSalesOrderCreateAndDetail(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "model_name_snapshot": "CLIENT SPOOF", "quantity": 3, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	})

	createdResponse := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created salesdto.SalesOrderDTO
	decodeBody(t, createdResponse, &created)
	if created.ID == uuid.Nil || created.Reference != "ORDER-1048" || created.Notes != "Pack carefully" ||
		created.SalesChannel == nil || created.SalesChannel.ID != channel.ID || len(created.Items) != 2 {
		t.Fatalf("unexpected created order: %#v", created)
	}
	if created.Items[0].ModelNameSnapshot != first.Name || created.Items[1].ModelNameSnapshot != second.Name {
		t.Fatalf("snapshots were not generated from Models: %#v", created.Items)
	}

	if err := db.Model(&models.Model{}).Where("id = ?", first.ID).Update("name", "Renamed dragon").Error; err != nil {
		t.Fatal(err)
	}
	archive := performRequest(t, router, http.MethodPut, "/sales-channels/"+channel.ID.String()+"/archive", nil)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive channel: %d %s", archive.Code, archive.Body.String())
	}

	detailResponse := performRequest(t, router, http.MethodGet, "/sales-orders/"+created.ID.String(), nil)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", detailResponse.Code, detailResponse.Body.String())
	}
	var detail salesdto.SalesOrderDTO
	decodeBody(t, detailResponse, &detail)
	if len(detail.Items) != 2 || detail.Items[0].ModelNameSnapshot != first.Name {
		t.Fatalf("detail did not preserve item history: %#v", detail)
	}
	if detail.SalesChannel == nil || detail.SalesChannel.ID != channel.ID || !detail.SalesChannel.IsArchived {
		t.Fatalf("detail did not resolve archived channel: %#v", detail.SalesChannel)
	}
}

func TestSalesOrderCreateRollsBackForInvalidSecondModel(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, _ := createSalesOrderReferences(t, db)
	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": 800},
		{"model_id": uuid.New().String(), "quantity": 1, "unit_price_cents": 1200},
	})

	response := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if response.Code != http.StatusNotFound {
		t.Fatalf("invalid Model: %d %s", response.Code, response.Body.String())
	}
	assertSalesOrderCounts(t, db, 0, 0)
}

func TestSalesOrderCreateRollsBackForItemPersistenceFailure(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	itemWrites := 0
	callbackName := "test:fail_second_sales_order_item"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "sales_order_items" {
			itemWrites++
			if itemWrites == 2 {
				tx.AddError(errors.New("forced Sales Order Item failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(callbackName)

	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	})
	response := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("forced item failure: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "forced Sales Order Item failure") {
		t.Fatalf("persistence error leaked to client: %s", response.Body.String())
	}
	assertSalesOrderCounts(t, db, 0, 0)
}

func TestSalesOrderCreateRejectsReferencesAndMalformedValues(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, _ := createSalesOrderReferences(t, db)
	archived := salesmodels.SalesChannel{Name: "Old marketplace", IsArchived: true}
	if err := db.Create(&archived).Error; err != nil {
		t.Fatal(err)
	}
	validItems := []map[string]any{{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": 800}}
	invalidReference := salesOrderRequest(channel.ID, validItems)
	invalidReference["reference"] = "   "
	invalidDate := salesOrderRequest(channel.ID, validItems)
	invalidDate["order_date"] = "not-a-date"
	invalidFulfilment := salesOrderRequest(channel.ID, validItems)
	invalidFulfilment["fulfilment_status"] = "unknown"
	invalidPayment := salesOrderRequest(channel.ID, validItems)
	invalidPayment["payment_status"] = "pending"

	tests := []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"missing channel", salesOrderRequest(uuid.New(), validItems), http.StatusNotFound},
		{"archived channel", salesOrderRequest(archived.ID, validItems), http.StatusConflict},
		{"no items", salesOrderRequest(channel.ID, []map[string]any{}), http.StatusBadRequest},
		{"negative cost", salesOrderRequest(channel.ID, validItems), http.StatusBadRequest},
		{"empty reference", invalidReference, http.StatusBadRequest},
		{"invalid date", invalidDate, http.StatusBadRequest},
		{"invalid fulfilment", invalidFulfilment, http.StatusBadRequest},
		{"invalid payment", invalidPayment, http.StatusBadRequest},
		{"malformed channel UUID", map[string]any{
			"reference": "ORDER", "order_date": "2026-09-28T00:00:00Z", "sales_channel_id": "invalid",
			"fulfilment_status": "draft", "payment_status": "unpaid", "items": validItems,
		}, http.StatusBadRequest},
		{"zero quantity", salesOrderRequest(channel.ID, []map[string]any{{"model_id": first.ID.String(), "quantity": 0, "unit_price_cents": 800}}), http.StatusBadRequest},
		{"negative item price", salesOrderRequest(channel.ID, []map[string]any{{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": -1}}), http.StatusBadRequest},
		{"malformed Model UUID", salesOrderRequest(channel.ID, []map[string]any{{"model_id": "invalid", "quantity": 1, "unit_price_cents": 800}}), http.StatusBadRequest},
		{"fractional quantity", salesOrderRequest(channel.ID, []map[string]any{{"model_id": first.ID.String(), "quantity": 1.5, "unit_price_cents": 800}}), http.StatusBadRequest},
		{"string price", salesOrderRequest(channel.ID, []map[string]any{{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": "800"}}), http.StatusBadRequest},
	}
	tests[3].body["shipping_cost_cents"] = -1

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(t, router, http.MethodPost, "/sales-orders", test.body)
			if response.Code != test.status {
				t.Errorf("got %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}
	assertSalesOrderCounts(t, db, 0, 0)
}

func TestSalesOrderDetailErrorsArePredictable(t *testing.T) {
	router, _ := setupTestRouter(t)
	for path, expected := range map[string]int{
		"/sales-orders/not-a-uuid":             http.StatusBadRequest,
		"/sales-orders/" + uuid.New().String(): http.StatusNotFound,
	} {
		response := performRequest(t, router, http.MethodGet, path, nil)
		if response.Code != expected {
			t.Errorf("GET %s: got %d, want %d: %s", path, response.Code, expected, response.Body.String())
		}
	}
}

func TestSalesOrderUpdateReplacesItemsAtomically(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	created := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 2, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	}))
	newChannel := salesmodels.SalesChannel{Name: "Craft market"}
	if err := db.Create(&newChannel).Error; err != nil {
		t.Fatal(err)
	}

	payload := salesOrderRequest(newChannel.ID, []map[string]any{
		{"model_id": second.ID.String(), "quantity": 4, "unit_price_cents": 1500},
	})
	payload["reference"] = "ORDER-UPDATED"
	payload["packaging_cost_cents"] = 225
	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), payload)
	if response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body.String())
	}
	var updated salesdto.SalesOrderDTO
	decodeBody(t, response, &updated)
	if updated.ID != created.ID || updated.Reference != "ORDER-UPDATED" || updated.PackagingCostCents != 225 ||
		updated.SalesChannel == nil || updated.SalesChannel.ID != newChannel.ID || len(updated.Items) != 1 {
		t.Fatalf("unexpected updated order: %#v", updated)
	}
	if updated.Items[0].ModelNameSnapshot != second.Name || updated.Items[0].Quantity != 4 || updated.Items[0].UnitPriceCents != 1500 {
		t.Fatalf("unexpected replacement item: %#v", updated.Items)
	}
	assertSalesOrderCounts(t, db, 1, 1)
}

func TestSalesOrderUpdateRollsBackForInvalidModel(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	created := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 2, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	}))
	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 9, "unit_price_cents": 999},
		{"model_id": uuid.New().String(), "quantity": 1, "unit_price_cents": 500},
	})
	payload["reference"] = "MUST-ROLL-BACK"

	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), payload)
	if response.Code != http.StatusNotFound {
		t.Fatalf("invalid Model update: %d %s", response.Code, response.Body.String())
	}
	assertUnchangedSalesOrder(t, router, created)
	assertSalesOrderCounts(t, db, 1, 2)
}

func TestSalesOrderUpdateRollsBackForItemPersistenceFailure(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	created := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 2, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	}))

	itemWrites := 0
	callbackName := "test:fail_second_sales_order_update_item"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "sales_order_items" {
			itemWrites++
			if itemWrites == 2 {
				tx.AddError(errors.New("forced replacement item failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(callbackName)

	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": second.ID.String(), "quantity": 3, "unit_price_cents": 1400},
		{"model_id": first.ID.String(), "quantity": 4, "unit_price_cents": 900},
	})
	payload["reference"] = "MUST-ROLL-BACK"
	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), payload)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("forced update failure: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "forced replacement item failure") {
		t.Fatalf("persistence error leaked to client: %s", response.Body.String())
	}
	assertUnchangedSalesOrder(t, router, created)
	assertSalesOrderCounts(t, db, 1, 2)
}

func TestSalesOrderUpdateEnforcesDraftAndStatusBoundaries(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, _ := createSalesOrderReferences(t, db)
	payload := salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": 800},
	})
	created := createSalesOrderForTest(t, router, payload)

	statusPayload := salesOrderRequest(channel.ID, payload["items"].([]map[string]any))
	statusPayload["fulfilment_status"] = "confirmed"
	statusResponse := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), statusPayload)
	if statusResponse.Code != http.StatusConflict {
		t.Fatalf("status through edit endpoint: %d %s", statusResponse.Code, statusResponse.Body.String())
	}
	archived := salesmodels.SalesChannel{Name: "Archived", IsArchived: true}
	if err := db.Create(&archived).Error; err != nil {
		t.Fatal(err)
	}
	archivedPayload := salesOrderRequest(archived.ID, payload["items"].([]map[string]any))
	archivedResponse := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), archivedPayload)
	if archivedResponse.Code != http.StatusConflict {
		t.Fatalf("archived channel update: %d %s", archivedResponse.Code, archivedResponse.Body.String())
	}
	assertUnchangedSalesOrder(t, router, created)

	if err := db.Model(&salesmodels.SalesOrder{}).Where("id = ?", created.ID).
		Update("fulfilment_status", salesmodels.SalesOrderFulfilmentConfirmed).Error; err != nil {
		t.Fatal(err)
	}
	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), payload)
	if response.Code != http.StatusConflict {
		t.Fatalf("non-draft edit: %d %s", response.Code, response.Body.String())
	}
}

func TestSalesOrderStatusTransitionsAndPaymentIndependence(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	order := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": product.ID.String(), "quantity": 2, "unit_price_cents": 800},
	}))

	paymentResponse := performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/status", map[string]any{
		"payment_status": "paid",
	})
	if paymentResponse.Code != http.StatusOK {
		t.Fatalf("payment status: %d %s", paymentResponse.Code, paymentResponse.Body.String())
	}
	var paymentUpdated salesdto.SalesOrderDTO
	decodeBody(t, paymentResponse, &paymentUpdated)
	if paymentUpdated.PaymentStatus != salesmodels.SalesOrderPaymentPaid ||
		paymentUpdated.FulfilmentStatus != salesmodels.SalesOrderFulfilmentDraft {
		t.Fatalf("payment change affected fulfilment: %#v", paymentUpdated)
	}

	for _, status := range []salesmodels.SalesOrderFulfilmentStatus{
		salesmodels.SalesOrderFulfilmentConfirmed,
		salesmodels.SalesOrderFulfilmentPreparing,
		salesmodels.SalesOrderFulfilmentReadyToShip,
		salesmodels.SalesOrderFulfilmentShipped,
		salesmodels.SalesOrderFulfilmentCompleted,
	} {
		response := performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/status", map[string]any{
			"fulfilment_status": status,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("transition to %s: %d %s", status, response.Code, response.Body.String())
		}
		var updated salesdto.SalesOrderDTO
		decodeBody(t, response, &updated)
		if updated.FulfilmentStatus != status || updated.PaymentStatus != salesmodels.SalesOrderPaymentPaid {
			t.Fatalf("unexpected status response: %#v", updated)
		}
	}

	terminalResponse := performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/status", map[string]any{
		"fulfilment_status": "cancelled",
	})
	if terminalResponse.Code != http.StatusConflict {
		t.Fatalf("completed transition: %d %s", terminalResponse.Code, terminalResponse.Body.String())
	}
	deleteResponse := performRequest(t, router, http.MethodDelete, "/sales-orders/"+order.ID.String(), nil)
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("completed delete: %d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
}

func TestSalesOrderStatusRejectsInvalidRequests(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	order := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": product.ID.String(), "quantity": 1, "unit_price_cents": 800},
	}))

	tests := []struct {
		path   string
		body   any
		status int
	}{
		{"/sales-orders/not-a-uuid/status", map[string]any{"payment_status": "paid"}, http.StatusBadRequest},
		{"/sales-orders/" + uuid.New().String() + "/status", map[string]any{"payment_status": "paid"}, http.StatusNotFound},
		{"/sales-orders/" + order.ID.String() + "/status", map[string]any{}, http.StatusBadRequest},
		{"/sales-orders/" + order.ID.String() + "/status", map[string]any{"payment_status": "pending"}, http.StatusBadRequest},
		{"/sales-orders/" + order.ID.String() + "/status", map[string]any{"fulfilment_status": "shipped"}, http.StatusConflict},
	}
	for _, test := range tests {
		response := performRequest(t, router, http.MethodPut, test.path, test.body)
		if response.Code != test.status {
			t.Errorf("PUT %s: got %d, want %d: %s", test.path, response.Code, test.status, response.Body.String())
		}
	}
}

func TestSalesOrderDeleteDraftAndPreserveCancelledHistory(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, first, second := createSalesOrderReferences(t, db)
	draft := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 1, "unit_price_cents": 800},
		{"model_id": second.ID.String(), "quantity": 1, "unit_price_cents": 1200},
	}))
	deleteDraft := performRequest(t, router, http.MethodDelete, "/sales-orders/"+draft.ID.String(), nil)
	if deleteDraft.Code != http.StatusNoContent {
		t.Fatalf("delete draft: %d %s", deleteDraft.Code, deleteDraft.Body.String())
	}
	assertSalesOrderCounts(t, db, 0, 0)

	cancelled := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{
		{"model_id": first.ID.String(), "quantity": 2, "unit_price_cents": 900},
	}))
	for _, status := range []string{"confirmed", "cancelled"} {
		response := performRequest(t, router, http.MethodPut, "/sales-orders/"+cancelled.ID.String()+"/status", map[string]any{
			"fulfilment_status": status,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("transition to %s: %d %s", status, response.Code, response.Body.String())
		}
	}
	deleteCancelled := performRequest(t, router, http.MethodDelete, "/sales-orders/"+cancelled.ID.String(), nil)
	if deleteCancelled.Code != http.StatusConflict {
		t.Fatalf("delete cancelled: %d %s", deleteCancelled.Code, deleteCancelled.Body.String())
	}
	detail := performRequest(t, router, http.MethodGet, "/sales-orders/"+cancelled.ID.String(), nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("cancelled detail: %d %s", detail.Code, detail.Body.String())
	}
	var persisted salesdto.SalesOrderDTO
	decodeBody(t, detail, &persisted)
	if persisted.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCancelled || len(persisted.Items) != 1 {
		t.Fatalf("cancelled history was not preserved: %#v", persisted)
	}
	assertSalesOrderCounts(t, db, 1, 1)
}

func TestSalesOrderDeleteErrorsArePredictable(t *testing.T) {
	router, _ := setupTestRouter(t)
	for path, expected := range map[string]int{
		"/sales-orders/not-a-uuid":             http.StatusBadRequest,
		"/sales-orders/" + uuid.New().String(): http.StatusNotFound,
	} {
		response := performRequest(t, router, http.MethodDelete, path, nil)
		if response.Code != expected {
			t.Errorf("DELETE %s: got %d, want %d: %s", path, response.Code, expected, response.Body.String())
		}
	}
}

func createSalesOrderForTest(t *testing.T, router *gin.Engine, payload map[string]any) salesdto.SalesOrderDTO {
	t.Helper()
	response := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if response.Code != http.StatusCreated {
		t.Fatalf("create fixture order: %d %s", response.Code, response.Body.String())
	}
	var order salesdto.SalesOrderDTO
	decodeBody(t, response, &order)
	return order
}

func assertUnchangedSalesOrder(t *testing.T, router *gin.Engine, expected salesdto.SalesOrderDTO) {
	t.Helper()
	response := performRequest(t, router, http.MethodGet, "/sales-orders/"+expected.ID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("reload order: %d %s", response.Code, response.Body.String())
	}
	var actual salesdto.SalesOrderDTO
	decodeBody(t, response, &actual)
	if actual.Reference != expected.Reference || len(actual.Items) != len(expected.Items) {
		t.Fatalf("order changed after failed transaction: got %#v, want %#v", actual, expected)
	}
	for index := range expected.Items {
		if actual.Items[index].ID != expected.Items[index].ID ||
			actual.Items[index].Quantity != expected.Items[index].Quantity ||
			actual.Items[index].UnitPriceCents != expected.Items[index].UnitPriceCents {
			t.Fatalf("item changed after failed transaction: got %#v, want %#v", actual.Items, expected.Items)
		}
	}
}

func assertSalesOrderCounts(t *testing.T, db *gorm.DB, orders, items int64) {
	t.Helper()
	var orderCount, itemCount int64
	if err := db.Model(&salesmodels.SalesOrder{}).Count(&orderCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&salesmodels.SalesOrderItem{}).Count(&itemCount).Error; err != nil {
		t.Fatal(err)
	}
	if orderCount != orders || itemCount != items {
		t.Fatalf("unexpected persisted state: %d orders, %d items", orderCount, itemCount)
	}
}
