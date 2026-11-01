package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"math"
	"net/http"
	"testing"
)

func TestSalesOrderCreateAndUpdateWithoutChannel(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	payload := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 3, "unit_price_cents": 1299,
	}})
	delete(payload, "sales_channel_id")
	payload["packaging_cost_cents"] = int64(101)
	payload["shipping_cost_cents"] = int64(202)
	payload["platform_fee_cents"] = int64(303)
	payload["other_cost_cents"] = int64(404)

	createdResponse := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create without channel: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created salesdto.SalesOrderDTO
	decodeBody(t, createdResponse, &created)
	if created.SalesChannelID != nil || created.SalesChannel != nil ||
		created.Items[0].UnitPriceCents != 1299 || created.PackagingCostCents != 101 ||
		created.ShippingCostCents != 202 || created.PlatformFeeCents != 303 || created.OtherCostCents != 404 {
		t.Fatalf("optional channel or integer cents changed: %#v", created)
	}

	withChannel := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 3, "unit_price_cents": 1299,
	}})
	updatedResponse := performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), withChannel)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("assign channel: %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	withoutChannel := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 3, "unit_price_cents": 1299,
	}})
	delete(withoutChannel, "sales_channel_id")
	updatedResponse = performRequest(t, router, http.MethodPut, "/sales-orders/"+created.ID.String(), withoutChannel)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("clear channel: %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var cleared salesdto.SalesOrderDTO
	decodeBody(t, updatedResponse, &cleared)
	if cleared.SalesChannelID != nil || cleared.SalesChannel != nil {
		t.Fatalf("channel was not cleared: %#v", cleared)
	}
}

func TestSalesOrderArchivedChannelCanRemainButCannotBeAssigned(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	withChannel := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 1, "unit_price_cents": 800,
	}})
	existing := createSalesOrderForTest(t, router, withChannel)
	archive := performRequest(t, router, http.MethodPut, "/sales-channels/"+channel.ID.String()+"/archive", nil)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive channel: %d %s", archive.Code, archive.Body.String())
	}

	withChannel["reference"] = "EDIT-HISTORICAL-CHANNEL"
	preserved := performRequest(t, router, http.MethodPut, "/sales-orders/"+existing.ID.String(), withChannel)
	if preserved.Code != http.StatusOK {
		t.Fatalf("preserve archived channel: %d %s", preserved.Code, preserved.Body.String())
	}

	withoutChannel := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 1, "unit_price_cents": 800,
	}})
	delete(withoutChannel, "sales_channel_id")
	other := createSalesOrderForTest(t, router, withoutChannel)
	assignArchived := performRequest(t, router, http.MethodPut, "/sales-orders/"+other.ID.String(), withChannel)
	if assignArchived.Code != http.StatusConflict {
		t.Fatalf("assign archived channel: got %d, want 409: %s", assignArchived.Code, assignArchived.Body.String())
	}
}

func TestSalesOrderRejectsRevenueOverflowBeforePersistence(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	payload := salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 2, "unit_price_cents": int64(math.MaxInt64),
	}})

	response := performRequest(t, router, http.MethodPost, "/sales-orders", payload)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("overflow: got %d, want 400: %s", response.Code, response.Body.String())
	}
	assertSalesOrderCounts(t, db, 0, 0)
}

func TestSalesOrderPaymentCanBeRefundedIndependently(t *testing.T) {
	router, db := setupTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	order := createSalesOrderForTest(t, router, salesOrderRequest(channel.ID, []map[string]any{{
		"model_id": product.ID.String(), "quantity": 1, "unit_price_cents": 800,
	}}))

	response := performRequest(t, router, http.MethodPut, "/sales-orders/"+order.ID.String()+"/status", map[string]any{
		"payment_status": "refunded",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("refund payment: %d %s", response.Code, response.Body.String())
	}
	var refunded salesdto.SalesOrderDTO
	decodeBody(t, response, &refunded)
	if refunded.PaymentStatus != salesmodels.SalesOrderPaymentRefunded || refunded.FulfilmentStatus != salesmodels.SalesOrderFulfilmentDraft {
		t.Fatalf("refund changed unrelated lifecycle state: %#v", refunded)
	}
}
