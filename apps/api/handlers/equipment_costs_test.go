package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestEquipmentMigrationAndTotal(t *testing.T) {
	r, db := setupTestRouter(t)
	if err := db.Create(&models.AppConfig{Key: "printer_upfront_cost", Value: "42.50", Type: "number"}).Error; err != nil {
		t.Fatal(err)
	}
	res := performRequest(t, r, http.MethodGet, "/app-config/printer_equipment_costs", nil)
	var cfg models.AppConfig
	decodeBody(t, res, &cfg)
	var expenses []map[string]any
	if err := json.Unmarshal([]byte(cfg.Value), &expenses); err != nil {
		t.Fatal(err)
	}
	if len(expenses) != 1 || expenses[0]["amount"] != 42.5 || expenses[0]["date"] != "" {
		t.Fatalf("legacy value lost: %s", cfg.Value)
	}
	res = performRequest(t, r, http.MethodPut, "/app-config/printer_equipment_costs", map[string]any{"type": "json", "value": `[{"description":"Old costs","date":"","amount":42.5},{"description":"PEI plate","date":"2026-09-17","amount":25}]`})
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var aggregate models.AppConfig
	if err := db.First(&aggregate, "key = ?", "printer_upfront_cost").Error; err != nil || aggregate.Value != "67.5" {
		t.Fatalf("wrong total: %#v %v", aggregate, err)
	}
	res = performRequest(t, r, http.MethodPut, "/app-config/printer_upfront_cost", map[string]any{"type": "number", "value": "999"})
	if res.Code != 400 {
		t.Fatal("managed total must not be independently editable")
	}
	for _, invalid := range []string{`[{"description":"x"}]`, `[{"description":"x","amount":null}]`, `null`, `{}`, `[{"description":"","amount":1}]`, `[{"description":"x","amount":-1}]`, `[{"description":"x","date":"2026-02-30","amount":1}]`, `[{"description":"x","amount":1000000001}]`, `[{"description":"x","amount":1,"category":"unknown"}]`} {
		res = performRequest(t, r, http.MethodPut, "/app-config/printer_equipment_costs", map[string]any{"type": "json", "value": invalid})
		if res.Code != 400 {
			t.Fatalf("invalid input accepted: %s: %s", invalid, res.Body.String())
		}
	}
	// A database failure must roll back both the list and its derived total.
	if err := db.Exec(`CREATE TRIGGER reject_equipment_total BEFORE UPDATE ON app_config WHEN NEW.key = 'printer_upfront_cost' BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	res = performRequest(t, r, http.MethodPut, "/app-config/printer_equipment_costs", map[string]any{"type": "json", "value": "[]"})
	if res.Code != 500 {
		t.Fatalf("expected rollback: %s", res.Body.String())
	}
	db.First(&cfg, "key = ?", "printer_equipment_costs")
	if err := json.Unmarshal([]byte(cfg.Value), &expenses); err != nil || len(expenses) != 2 {
		t.Fatal("list was not rolled back")
	}
	db.Exec("DROP TRIGGER reject_equipment_total")
	res = performRequest(t, r, http.MethodPut, "/app-config/printer_equipment_costs", map[string]any{"type": "json", "value": "[]"})
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	db.First(&aggregate, "key = ?", "printer_upfront_cost")
	if aggregate.Value != "0" {
		t.Fatal("removing last expense must clear total")
	}
}

func TestEquipmentBackupRoundTrip(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	original := `[{"description":"Plate","date":"2026-09-17","amount":25}]`
	for _, cfg := range []models.AppConfig{{Key: "printer_equipment_costs", Value: original, Type: "json"}, {Key: "printer_upfront_cost", Value: "25", Type: "number"}} {
		if err := db.Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	res := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var restored models.AppConfig
	targetDB.First(&restored, "key = ?", "printer_equipment_costs")
	if restored.Value != original {
		t.Fatal("backup lost equipment details")
	}
}
