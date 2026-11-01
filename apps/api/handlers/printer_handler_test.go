package handlers_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func addTestPrinter(t *testing.T, r *gin.Engine, name, model string) models.Printer {
	t.Helper()
	res := performRequest(t, r, http.MethodPost, "/printers", map[string]any{
		"name": name, "manufacturer": "Test manufacturer", "model": model,
	})
	if res.Code != 201 {
		t.Fatalf("create printer: %d %s", res.Code, res.Body.String())
	}
	var p models.Printer
	decodeBody(t, res, &p)
	if p.ID == uuid.Nil {
		t.Fatal("missing printer UUID")
	}
	return p
}

func TestPrintersPhysicalIdentityAndDefaultLifecycle(t *testing.T) {
	r, db := setupTestRouter(t)
	first := addTestPrinter(t, r, "Workshop", "A1")
	second := addTestPrinter(t, r, "Office", "A1")
	third := addTestPrinter(t, r, "Enclosed", "P1S")
	if !first.IsDefault || second.IsDefault || third.IsDefault || first.ID == second.ID {
		t.Fatal("physical printer identity/default mismatch")
	}
	// Cannot abandon other active printers without explicitly selecting a replacement.
	res := performRequest(t, r, http.MethodPut, "/printers/"+first.ID.String()+"/archive", nil)
	if res.Code != 409 {
		t.Fatalf("archive default: %d %s", res.Code, res.Body.String())
	}
	for _, path := range []string{
		"/printers/" + second.ID.String() + "/default",
		"/printers/" + first.ID.String() + "/archive",
		"/printers/" + first.ID.String() + "/archive", // idempotent
	} {
		if res := performRequest(t, r, http.MethodPut, path, nil); res.Code != 200 {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
	}
	res = performRequest(t, r, http.MethodPut, "/printers/"+first.ID.String()+"/default", nil)
	if res.Code != 409 {
		t.Fatalf("archived default: %d", res.Code)
	}
	for state, want := range map[string]int{"active": 2, "archived": 1, "all": 3} {
		res := performRequest(t, r, http.MethodGet, "/printers?state="+state, nil)
		var printers []models.Printer
		decodeBody(t, res, &printers)
		if res.Code != 200 || len(printers) != want {
			t.Fatalf("%s: %d %s", state, res.Code, res.Body.String())
		}
	}
	var defaults int64
	if err := db.Model(&models.Printer{}).Where("is_default = ?", true).Count(&defaults).Error; err != nil || defaults != 1 {
		t.Fatalf("default count = %d, err = %v", defaults, err)
	}
	// Even direct database writes cannot create a second default.
	duplicate := models.Printer{Name: "Duplicate", Manufacturer: "Test", Model: "A1", IsDefault: true}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("database accepted two defaults")
	}
}

func TestCreatePrinterPresetOverridesOnlyValidatedProfileInputs(t *testing.T) {
	r, db := setupTestRouter(t)
	if err := db.Create(&models.AppConfig{Key: "labor_cost_hour", Value: "31", Type: "number"}).Error; err != nil {
		t.Fatal(err)
	}
	preset := map[string]any{"machine_cost_mode": 1, "printer_purchase_cost": 299, "printer_upfront_cost": 35,
		"printer_annual_maintenance": 30, "printer_lifetime_years": 3, "printer_uptime_percent": 50,
		"printer_power_watts": 150, "printer_buffer_factor": 1.3}
	res := performRequest(t, r, http.MethodPost, "/printers", map[string]any{
		"name": "Reference A1", "manufacturer": "Bambu Lab", "model": "A1", "cost_config": preset,
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("create with preset: %d %s", res.Code, res.Body.String())
	}
	var p models.Printer
	decodeBody(t, res, &p)
	if p.CostConfig["printer_purchase_cost"] != 299 || p.CostConfig["printer_power_watts"] != 150 || p.CostConfig["labor_cost_hour"] != 31 {
		t.Fatalf("preset did not preserve workspace defaults: %#v", p.CostConfig)
	}
	invalid := map[string]any{"machine_cost_mode": 9}
	res = performRequest(t, r, http.MethodPost, "/printers", map[string]any{
		"name": "Invalid preset", "manufacturer": "Bambu Lab", "model": "A1", "cost_config": invalid,
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("invalid preset accepted: %d %s", res.Code, res.Body.String())
	}
	res = performRequest(t, r, http.MethodPost, "/printers", map[string]any{
		"name": "Null preset", "manufacturer": "Bambu Lab", "model": "A1", "cost_config": nil,
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("null preset accepted: %d %s", res.Code, res.Body.String())
	}
}

func TestPrinterArchiveLastAndRestore(t *testing.T) {
	r, _ := setupTestRouter(t)
	p := addTestPrinter(t, r, "Only printer", "A1")
	for _, action := range []string{"archive", "restore", "restore"} {
		res := performRequest(t, r, http.MethodPut, "/printers/"+p.ID.String()+"/"+action, nil)
		var result models.Printer
		decodeBody(t, res, &result)
		if res.Code != 200 || result.IsArchived != (action == "archive") || result.IsDefault != (action != "archive") {
			t.Fatalf("%s: %d %s", action, res.Code, res.Body.String())
		}
	}
}

func TestPrinterRoutesAreAvailableWithoutLicense(t *testing.T) {
	r, _ := setupTestRouter(t)
	if res := performRequest(t, r, http.MethodGet, "/printers", nil); res.Code != http.StatusOK {
		t.Fatalf("list printers: %d %s", res.Code, res.Body.String())
	}
	res := performRequest(t, r, http.MethodPost, "/printers", map[string]any{
		"name": "Open local printer", "manufacturer": "Test", "model": "A1",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("create printer: %d %s", res.Code, res.Body.String())
	}
}

func TestPrinterValidationAndMetadataIsolation(t *testing.T) {
	r, _ := setupTestRouter(t)
	p := addTestPrinter(t, r, " Original ", "A1")
	for _, test := range []struct {
		method, path string
		body         any
		status       int
	}{
		{"GET", "/printers/invalid", nil, 400},
		{"GET", "/printers/" + uuid.NewString(), nil, 404},
		{"GET", "/printers?state=invalid", nil, 400},
		{"POST", "/printers", map[string]any{"name": " ", "manufacturer": "Test", "model": "A1"}, 400},
		{"POST", "/printers", map[string]any{"name": "Printer"}, 400},
		{"PUT", "/printers/" + uuid.NewString() + "/default", nil, 404},
		{"DELETE", "/printers/" + p.ID.String(), nil, 404},
	} {
		res := performRequest(t, r, test.method, test.path, test.body)
		if res.Code != test.status {
			t.Fatalf("%s %s: %d %s", test.method, test.path, res.Code, res.Body.String())
		}
	}
	res := performRequest(t, r, "PUT", "/printers/"+p.ID.String(), map[string]any{
		"name": " Renamed ", "manufacturer": "Test", "model": "A1", "notes": "",
		"id": uuid.NewString(), "is_default": false, "is_archived": true,
	})
	var updated models.Printer
	decodeBody(t, res, &updated)
	if res.Code != 200 || updated.ID != p.ID || updated.Name != "Renamed" || !updated.IsDefault || updated.IsArchived {
		t.Fatalf("metadata update changed protected fields: %s", res.Body.String())
	}
}

func TestPrinterBackupRoundTripAndLegacyImport(t *testing.T) {
	source, sourceDB := setupTestRouter(t)
	p := addTestPrinter(t, source, "Backup machine", "A1")
	target, targetDB := setupTestRouter(t)
	addTestPrinter(t, target, "Replaced machine", "P1S")
	exported := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if exported.Code != 200 {
		t.Fatal(exported.Body.String())
	}
	imported := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "backup.zip", exported.Body.Bytes())
	if imported.Code != 200 {
		t.Fatal(imported.Body.String())
	}
	var restored []models.Printer
	if err := targetDB.Find(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].ID != p.ID || !restored[0].IsDefault {
		t.Fatalf("printer identity/default lost: %#v", restored)
	}

	// Construct an actual version-1 archive, with no printers table.
	legacyJob := models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 1}
	if err := sourceDB.Create(&legacyJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := sourceDB.Migrator().DropTable(&models.Printer{}); err != nil {
		t.Fatal(err)
	}
	legacyExport := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if legacyExport.Code != 200 {
		t.Fatal(legacyExport.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(legacyExport.Body.Bytes()), int64(legacyExport.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range reader.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == "manifest.json" {
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest["schema_version"] = 1
			data, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
		}
		files[entry.Name] = data
	}
	imported = performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "legacy.zip", buildZipArchive(t, files))
	if imported.Code != 200 {
		t.Fatal(imported.Body.String())
	}
	var restoredPrinters []models.Printer
	if err := targetDB.Find(&restoredPrinters).Error; err != nil || len(restoredPrinters) != 1 || !restoredPrinters[0].IsDefault {
		t.Fatalf("legacy restore did not create the migrated printer: %#v %v", restoredPrinters, err)
	}
	var restoredJob models.PrintJob
	if err := targetDB.First(&restoredJob, "id = ?", legacyJob.ID).Error; err != nil {
		t.Fatalf("legacy restore lost the job: %#v %v", restoredJob, err)
	}
	if restoredJob.PrinterID != nil || restoredJob.PrinterCost != nil {
		t.Fatalf("legacy restore changed the job's historical meaning: %#v", restoredJob)
	}
}
