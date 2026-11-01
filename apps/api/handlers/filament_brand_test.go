package handlers_test

import (
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestFilamentBrandsAreTrimmedDeduplicatedAndSorted(t *testing.T) {
	r, db := setupTestRouter(t)
	for _, brand := range []string{" Prusament ", "bambu lab", "Bambu Lab", "Polymaker"} {
		filament := models.Filament{Brand: brand, Type: "PLA", Color: brand, AvgPrice: 20}
		if err := db.Create(&filament).Error; err != nil {
			t.Fatal(err)
		}
	}

	w := performRequest(t, r, http.MethodGet, "/filament-brands", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var brands []string
	decodeBody(t, w, &brands)
	want := []string{"bambu lab", "Polymaker", "Prusament"}
	if len(brands) != len(want) {
		t.Fatalf("brands = %#v, want %#v", brands, want)
	}
	for i := range want {
		if brands[i] != want[i] {
			t.Fatalf("brands = %#v, want %#v", brands, want)
		}
	}
}

func TestFilamentWritesReuseExistingBrandCasing(t *testing.T) {
	r, db := setupTestRouter(t)
	existing := models.Filament{Brand: "Bambu Lab", Type: "PLA", Color: "Black", AvgPrice: 20}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	w := performRequest(t, r, http.MethodPost, "/filaments", map[string]any{
		"brand": "  bambu lab ", "type": "PETG", "color": "Blue", "avg_price": 25, "personal_score": 4,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var created models.Filament
	if err := db.Where("type = ?", "PETG").First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created.Brand != "Bambu Lab" {
		t.Fatalf("brand = %q, want %q", created.Brand, "Bambu Lab")
	}
}
