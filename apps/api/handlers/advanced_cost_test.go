package handlers_test

import (
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestAdvancedSettingsDefaultsAndValidation(t *testing.T) {
	r, db := setupTestRouter(t)
	for key, value := range models.AdvancedConfigDefaults {
		res := performRequest(t, r, http.MethodGet, "/app-config/"+key, nil)
		if res.Code != 200 {
			t.Fatalf("default %s: %s", key, res.Body.String())
		}
		var body apiResponse
		decodeBody(t, res, &body)
		if body["value"] != value {
			t.Fatalf("default %s = %v", key, body)
		}
	}
	var count int64
	db.Model(&models.AppConfig{}).Count(&count)
	if count != 0 {
		t.Fatal("reading defaults must not mutate an old database")
	}
	for _, tc := range []struct{ key, value string }{
		{"printer_lifetime_years", "0"}, {"printer_uptime_percent", "0"},
		{"printer_uptime_percent", "101"}, {"printer_power_watts", "-1"},
		{"printer_buffer_factor", "0.9"}, {"machine_cost_mode", "2"},
		{"printer_purchase_cost", "NaN"}, {"labor_cost_hour", "+Inf"},
		{"printer_purchase_cost", "1000000001"},
	} {
		// POST and PUT both validate, including unsaved virtual defaults.
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			if method == http.MethodPut && tc.key == "labor_cost_hour" {
				continue
			}
			res := performRequest(t, r, method, "/app-config/"+tc.key, map[string]any{"value": tc.value, "type": "number"})
			if res.Code != 400 {
				t.Fatalf("%s %s=%s: %d %s", method, tc.key, tc.value, res.Code, res.Body.String())
			}
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		res := performRequest(t, r, method, "/app-config/margin_tier_1", map[string]any{"value": "50", "type": "number"})
		if res.Code != http.StatusNotFound && !(method == http.MethodPost && res.Code == http.StatusBadRequest) {
			t.Fatalf("removed margin setting remained available through %s: %d %s", method, res.Code, res.Body.String())
		}
	}
	res := performRequest(t, r, http.MethodPut, "/app-config/printer_power_watts", map[string]any{"value": "0", "type": "number"})
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var config models.AppConfig
	if err := db.First(&config, "key = ?", "printer_power_watts").Error; err != nil || config.Value != "0" {
		t.Fatalf("edit must persist virtual default: %#v %v", config, err)
	}
}

func TestSpreadsheetCostConsistentAcrossEndpoints(t *testing.T) {
	r, db := setupTestRouter(t)
	for key, value := range map[string]string{
		"electricity_cost_kwh": "0.20", "printer_wear_cost_hour": "0.08", "labor_cost_hour": "15", "filament_waste_factor": "1.1",
	} {
		if err := db.Create(&models.AppConfig{Key: key, Value: value, Type: "number"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	printer := addTestPrinter(t, r, "Spreadsheet profile", "A1")
	printer.CostConfig["filament_waste_factor"] = 1.1
	printer = configurePrinter(t, r, printer, 0.20, 0.08, 15)
	job := createPrinterJob(t, r, printer.ID.String())
	costPath := "/print-jobs/" + job.ID.String() + "/cost"
	res := performRequest(t, r, http.MethodGet, costPath, nil)
	var manual apiResponse
	decodeBody(t, res, &manual)
	assertFloat(t, manual["total_cost"], 15.28)

	res = performRequest(t, r, http.MethodPut, "/app-config/machine_cost_mode", map[string]any{"value": "1", "type": "number"})
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	// Global settings changed after creation must not replace the snapshot.
	res = performRequest(t, r, http.MethodGet, costPath, nil)
	var cost apiResponse
	decodeBody(t, res, &cost)
	assertFloat(t, cost["wear_cost"], 0.08)
	assertFloat(t, cost["electricity_cost"], 0.20)
	assertFloat(t, cost["total_cost"], 15.28)
	res = performRequest(t, r, http.MethodGet, "/print-jobs", nil)
	var jobs []apiResponse
	decodeBody(t, res, &jobs)
	assertFloat(t, jobs[0]["estimated_cost"], 15.28)
	res = performRequest(t, r, http.MethodGet, "/print-jobs/"+job.ID.String(), nil)
	var detail apiResponse
	decodeBody(t, res, &detail)
	assertFloat(t, detail["estimated_cost"], 15.28)
	res = performRequest(t, r, http.MethodGet, "/print-jobs/stats", nil)
	var stats apiResponse
	decodeBody(t, res, &stats)
	assertFloat(t, stats["total_cost"], 15.28)
	res = performRequest(t, r, http.MethodGet, "/print-jobs/dashboard-summary", nil)
	var dashboard apiResponse
	decodeBody(t, res, &dashboard)
	assertFloat(t, dashboard["cost_stats"].(map[string]any)["total_cost"], 15.28)

	// Further global changes also leave every snapshot consumer stable.
	for key, value := range map[string]string{"printer_purchase_cost": "1000", "printer_upfront_cost": "200", "printer_annual_maintenance": "100", "printer_lifetime_years": "5", "printer_uptime_percent": "25", "printer_power_watts": "200", "printer_buffer_factor": "2"} {
		res = performRequest(t, r, http.MethodPut, "/app-config/"+key, map[string]any{"value": value, "type": "number"})
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
	}
	res = performRequest(t, r, http.MethodGet, costPath, nil)
	decodeBody(t, res, &cost)
	assertFloat(t, cost["total_cost"], 15.28)
	assertFloat(t, cost["electricity_cost"], 0.20)
	assertFloat(t, cost["wear_cost"], 0.08)
	// Switching the global mode back still leaves the snapshot unchanged.
	performRequest(t, r, http.MethodPut, "/app-config/machine_cost_mode", map[string]any{"value": "0", "type": "number"})
	res = performRequest(t, r, http.MethodGet, costPath, nil)
	decodeBody(t, res, &cost)
	assertFloat(t, cost["total_cost"], 15.28)
}

func TestAdvancedSettingsBackupRoundTrip(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	for key, value := range map[string]string{"machine_cost_mode": "1", "printer_purchase_cost": "500", "printer_uptime_percent": "20"} {
		if err := db.Create(&models.AppConfig{Key: key, Value: value, Type: "number"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	for key, value := range map[string]string{"machine_cost_mode": "1", "printer_purchase_cost": "500", "printer_uptime_percent": "20"} {
		var cfg models.AppConfig
		if err := targetDB.First(&cfg, "key = ?", key).Error; err != nil || cfg.Value != value {
			t.Fatalf("restore %s: %#v %v", key, cfg, err)
		}
	}
}
