package handlers_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestFilamentTypeCatalog(t *testing.T) {
	r, db := setupTestRouter(t)
	filament := models.Filament{Brand: "Custom", Type: "PLA Silk", Color: "Red", AvgPrice: 20}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	w := performRequest(t, r, http.MethodGet, "/filament-types", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "PLA Silk") || !strings.Contains(w.Body.String(), "PETG") {
		t.Fatalf("missing initial types: %s", w.Body.String())
	}
	for _, names := range [][]string{{" PLA ", "PETG-CF"}, {"ASA"}, {}} {
		w = performRequest(t, r, http.MethodPut, "/filament-types", map[string]any{"types": names})
		if w.Code != 200 {
			t.Fatalf("save status %d: %s", w.Code, w.Body.String())
		}
		var saved, loaded []string
		decodeBody(t, w, &saved)
		w = performRequest(t, r, http.MethodGet, "/filament-types", nil)
		decodeBody(t, w, &loaded)
		if !reflect.DeepEqual(saved, loaded) {
			t.Fatalf("catalog not persisted: %v != %v", saved, loaded)
		}
	}
	var unchanged models.Filament
	if err := db.First(&unchanged, "id = ?", filament.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Type != "PLA Silk" {
		t.Fatal("catalog edit changed historical filament")
	}
	w = performRequest(t, r, http.MethodGet, "/app-config", nil)
	if strings.Contains(w.Body.String(), "filament_types") {
		t.Fatal("catalog exposed as numeric setting")
	}
}

func TestFilamentTypeValidation(t *testing.T) {
	r, _ := setupTestRouter(t)
	for _, types := range []any{nil, []string{" "}, []string{"PLA", " pla "}, []string{strings.Repeat("a", 201)}, make([]string, 201)} {
		w := performRequest(t, r, http.MethodPut, "/filament-types", map[string]any{"types": types})
		if w.Code != 400 {
			t.Fatalf("expected invalid catalog rejection, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestFilamentTypeBackupRoundTrip(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	w := performRequest(t, r, http.MethodPut, "/filament-types", map[string]any{"types": []string{"PLA Silk", "PETG-CF"}})
	if w.Code != 200 {
		t.Fatalf("save failed: %s", w.Body.String())
	}
	backup := performRequest(t, r, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatalf("export failed: %s", backup.Body.String())
	}
	target, _ := setupDesktopTestRouter(t)
	imported := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if imported.Code != 200 {
		t.Fatalf("import failed: %s", imported.Body.String())
	}
	restored := performRequest(t, target, http.MethodGet, "/filament-types", nil)
	if restored.Code != 200 || restored.Body.String() != w.Body.String() {
		t.Fatalf("catalog not restored: %s", restored.Body.String())
	}
}
