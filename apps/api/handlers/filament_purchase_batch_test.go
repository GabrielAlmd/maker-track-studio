package handlers_test

import (
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

func TestPurchaseBatch(t *testing.T) {
	r, db := setupTestRouter(t)
	existing := models.Filament{Brand: "Existing", Type: "PLA", Color: "Black", AvgPrice: 20}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"purchase_date": "2026-04-01T00:00:00Z",
		"items": []map[string]any{
			{"filament_id": existing.ID, "price": 20, "amount": 1},
			{"new_filament": map[string]any{"brand": " New ", "type": "PETG", "color": "White"}, "price": 30, "amount": 2},
		},
	}
	w := performRequest(t, r, http.MethodPost, "/filament-purchases/batch", payload)
	if w.Code != 201 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var purchases []apiResponse
	decodeBody(t, w, &purchases)
	if len(purchases) != 2 {
		t.Fatalf("expected two purchases: %v", purchases)
	}
	var filament models.Filament
	if err := db.Where("brand = ?", "New").First(&filament).Error; err != nil {
		t.Fatal(err)
	}
	if filament.AvgPrice != 15 {
		t.Fatalf("expected 15 per kg, got %v", filament.AvgPrice)
	}
}

func TestPurchaseBatchReusesExistingBrandCasing(t *testing.T) {
	r, db := setupTestRouter(t)
	existing := models.Filament{Brand: "Prusament", Type: "PLA", Color: "Black", AvgPrice: 20}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	w := performRequest(t, r, http.MethodPost, "/filament-purchases/batch", map[string]any{
		"purchase_date": "2026-04-01T00:00:00Z",
		"items":         []map[string]any{{"new_filament": map[string]any{"brand": " prusament ", "type": "PETG", "color": "White"}, "price": 30, "amount": 1}},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var created models.Filament
	if err := db.Where("type = ?", "PETG").First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created.Brand != "Prusament" {
		t.Fatalf("brand = %q, want Prusament", created.Brand)
	}
}

func TestPurchaseBatchRollback(t *testing.T) {
	r, db := setupTestRouter(t)
	w := performRequest(t, r, http.MethodPost, "/filament-purchases/batch", map[string]any{
		"purchase_date": "2026-04-01T00:00:00Z",
		"items": []map[string]any{
			{"new_filament": map[string]any{"brand": "New", "type": "PLA", "color": "White"}, "price": 20, "amount": 1},
			{"filament_id": uuid.New(), "price": 20, "amount": 1},
		},
	})
	if w.Code != 404 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	for _, model := range []any{&models.Filament{}, &models.FilamentPurchase{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial records persisted: %d", count)
		}
	}
}

func TestPurchaseBatchValidation(t *testing.T) {
	for _, items := range []any{
		[]any{},
		[]map[string]any{{"filament_id": uuid.New(), "price": -1, "amount": 1}},
		[]map[string]any{{"filament_id": uuid.New(), "price": 20, "amount": 0}},
		[]map[string]any{{"price": 20, "amount": 1}},
		[]map[string]any{{"new_filament": map[string]any{"brand": " ", "type": "PLA", "color": "White"}, "price": 20, "amount": 1}},
		[]map[string]any{{"filament_id": uuid.New(), "new_filament": map[string]any{"brand": "New", "type": "PLA", "color": "White"}, "price": 20, "amount": 1}},
	} {
		r, _ := setupTestRouter(t)
		w := performRequest(t, r, http.MethodPost, "/filament-purchases/batch", map[string]any{"purchase_date": "2026-04-01T00:00:00Z", "items": items})
		if w.Code != 400 {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	}
}
