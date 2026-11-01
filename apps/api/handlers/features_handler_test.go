package handlers_test

import (
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestFeaturesDefaultWhenConfigIsMissing(t *testing.T) {
	router, db := setupTestRouter(t)
	response := performRequest(t, router, http.MethodGet, "/features", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get features: %d %s", response.Code, response.Body.String())
	}
	var features map[string]bool
	decodeBody(t, response, &features)
	if features["sales_orders"] {
		t.Fatal("expected Sales & Orders to default to disabled")
	}
	var count int64
	if err := db.Model(&models.AppConfig{}).Where("key = ?", models.AppFeaturesConfigKey).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("read created an unexpected feature row: %d", count)
	}
}

func TestFeaturesPersistEnabledAndDisabledStates(t *testing.T) {
	router, db := setupTestRouter(t)
	for _, enabled := range []bool{true, false} {
		response := performRequest(t, router, http.MethodPut, "/features", map[string]any{"sales_orders": enabled})
		if response.Code != http.StatusOK {
			t.Fatalf("update features: %d %s", response.Code, response.Body.String())
		}
		var updated map[string]bool
		decodeBody(t, response, &updated)
		if updated["sales_orders"] != enabled {
			t.Fatalf("expected sales_orders=%t, got %#v", enabled, updated)
		}

		response = performRequest(t, router, http.MethodGet, "/features", nil)
		var persisted map[string]bool
		decodeBody(t, response, &persisted)
		if persisted["sales_orders"] != enabled {
			t.Fatalf("persisted sales_orders=%t, got %#v", enabled, persisted)
		}
	}

	var config models.AppConfig
	if err := db.First(&config, "key = ?", models.AppFeaturesConfigKey).Error; err != nil {
		t.Fatal(err)
	}
	if config.Type != "json" || config.Value != `{"sales_orders":false}` {
		t.Fatalf("unexpected persisted feature config: %#v", config)
	}
}

func TestFeaturesRejectInvalidUpdates(t *testing.T) {
	router, _ := setupTestRouter(t)
	for _, payload := range []any{map[string]any{}, map[string]any{"sales_orders": "yes"}, map[string]any{"sales_orders": nil}} {
		response := performRequest(t, router, http.MethodPut, "/features", payload)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid update: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestFeatureUpdateRetainsUnavailableModulePreference(t *testing.T) {
	router, db := setupTestRouter(t)
	if err := db.Create(&models.AppConfig{Key: models.AppFeaturesConfigKey, Value: `{"sales_orders":false,"future_module":true}`, Type: "json"}).Error; err != nil {
		t.Fatal(err)
	}
	response := performRequest(t, router, http.MethodPut, "/features", map[string]any{"sales_orders": true})
	if response.Code != http.StatusOK {
		t.Fatalf("update features: %d %s", response.Code, response.Body.String())
	}
	var saved models.AppConfig
	if err := db.First(&saved, "key = ?", models.AppFeaturesConfigKey).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Value != `{"future_module":true,"sales_orders":true}` {
		t.Fatalf("unavailable module preference changed: %s", saved.Value)
	}
}

func TestDisablingFeaturesDoesNotChangeSalesDataOrRoutes(t *testing.T) {
	router, db := setupTestRouter(t)
	productionCost, totalCost, netProfit, margin := int64(100), int64(140), int64(860), int64(8600)
	order := salesmodels.SalesOrder{
		Reference: "FEATURE-HISTORY", OrderDate: time.Now(),
		FulfilmentStatus:    salesmodels.SalesOrderFulfilmentCompleted,
		PaymentStatus:       salesmodels.SalesOrderPaymentPaid,
		ProductionCostCents: &productionCost, TotalCostCents: &totalCost,
		NetProfitCents: &netProfit, MarginBasisPoints: &margin,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&salesmodels.SalesOrderItem{
		SalesOrderID: order.ID, ModelNameSnapshot: "Historical model", Quantity: 1, UnitPriceCents: 1000,
	}).Error; err != nil {
		t.Fatal(err)
	}

	response := performRequest(t, router, http.MethodPut, "/features", map[string]any{"sales_orders": false})
	if response.Code != http.StatusOK {
		t.Fatalf("disable features: %d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&salesmodels.SalesOrder{}).Where("id = ?", order.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("Sales Order changed after toggle: count=%d error=%v", count, err)
	}
	response = performRequest(t, router, http.MethodGet, "/sales-orders/"+order.ID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("Sales API was gated by feature state: %d %s", response.Code, response.Body.String())
	}
}
