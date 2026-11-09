package handlers_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestIndividualPurchaseRejectsNonPositiveCostsWithoutChangingData(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, field := range []string{"price", "amount"} {
			for _, value := range []float64{0, -0.5} {
				t.Run(fmt.Sprintf("%s/%s=%g", method, field, value), func(t *testing.T) {
					r, db := setupTestRouter(t)
					filament := models.Filament{Brand: "Validation", Type: "PLA", Color: "Blue", AvgPrice: 20}
					if err := db.Create(&filament).Error; err != nil {
						t.Fatal(err)
					}
					path := "/filament-purchases"
					if method == http.MethodPut {
						purchase := models.FilamentPurchase{FilamentID: filament.ID, Price: 20, Amount: 1, PurchaseDate: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
						if err := db.Create(&purchase).Error; err != nil {
							t.Fatal(err)
						}
						path += "/" + purchase.ID.String()
					}
					payload := map[string]any{"filament_id": filament.ID, "price": 20, "amount": 1, "purchase_date": "2026-09-01T12:00:00Z"}
					payload[field] = value
					response := performRequest(t, r, method, path, payload)
					if response.Code != http.StatusBadRequest {
						t.Errorf("expected 400, got %d: %s", response.Code, response.Body.String())
					}
					var purchases []models.FilamentPurchase
					if err := db.Find(&purchases).Error; err != nil {
						t.Fatal(err)
					}
					if method == http.MethodPost && len(purchases) != 0 {
						t.Errorf("invalid purchase persisted: %#v", purchases)
					}
					if method == http.MethodPut && (len(purchases) != 1 || purchases[0].Price != 20 || purchases[0].Amount != 1) {
						t.Errorf("invalid edit changed stored costs: %#v", purchases)
					}
				})
			}
		}
	}
}

func TestPurchaseBatchWriteFailureRollsBackEarlierLines(t *testing.T) {
	r, db := setupTestRouter(t)
	if err := db.Exec(`CREATE TRIGGER reject_second_purchase BEFORE INSERT ON filament_purchases WHEN NEW.price = 99 BEGIN SELECT RAISE(ABORT, 'synthetic write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	response := performRequest(t, r, http.MethodPost, "/filament-purchases/batch", map[string]any{
		"purchase_date": "2026-09-01T12:00:00Z",
		"items": []map[string]any{
			{"new_filament": map[string]any{"brand": "First", "type": "PLA", "color": "Blue"}, "price": 20, "amount": 1},
			{"new_filament": map[string]any{"brand": "Second", "type": "PETG", "color": "Red"}, "price": 99, "amount": 1},
		},
	})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected synthetic write failure, got %d: %s", response.Code, response.Body.String())
	}
	for _, model := range []any{&models.Filament{}, &models.FilamentPurchase{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Errorf("partial records after failed transaction: %d (%v)", count, err)
		}
	}
}
