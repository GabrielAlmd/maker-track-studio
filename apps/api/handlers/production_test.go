package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

func TestProductionLifecycleRetriesAndRollback(t *testing.T) {
	r, db, printer := setupTestRouterWithDefaultPrinter(t)
	input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": time.Now().UTC().Format(time.RFC3339), "runs": []any{map[string]any{"status": "planned", "items": []any{map[string]any{"planned": 2}}, "materials": []any{}}}}
	created := performRequest(t, r, http.MethodPost, "/v2/production/jobs", input)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	replay := performRequest(t, r, http.MethodPost, "/v2/production/jobs", input)
	if replay.Code != 201 || replay.Body.String() != created.Body.String() {
		t.Fatalf("replay: %d %s", replay.Code, replay.Body.String())
	}
	var result map[string]any
	decodeBody(t, created, &result)
	id := result["id"].(string)
	path := "/v2/production/jobs/" + id
	input["runs"] = result["runs"]
	input["operation_id"] = uuid.NewString()
	input["expected_revision"] = result["revision"]
	run := input["runs"].([]any)[0].(map[string]any)
	run["status"] = "in_progress"
	started := performRequest(t, r, http.MethodPut, path, input)
	if started.Code != 200 {
		t.Fatalf("start: %d %s", started.Code, started.Body.String())
	}
	// A stale revision must not change any run or parent.
	input["operation_id"] = uuid.NewString()
	stale := performRequest(t, r, http.MethodPut, path, input)
	if stale.Code != 409 {
		t.Fatalf("stale: %d", stale.Code)
	}
	decodeBody(t, started, &result)
	input["expected_revision"] = result["revision"]
	input["runs"] = result["runs"]
	run = input["runs"].([]any)[0].(map[string]any)
	run["status"] = "failed"
	item := run["items"].([]any)[0].(map[string]any)
	item["attempted"] = 2
	item["rejected"] = 2
	filament := models.Filament{Brand: "Synthetic", Type: "PLA", Color: "Blue", AvgPrice: 20}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	run["materials"] = []any{map[string]any{"filament_id": filament.ID, "weight_used": 10, "measurement_state": "verified"}}
	// Fail after child writes; the entire operation, including revision, rolls back.
	if err := db.Exec("CREATE TRIGGER fail_production_receipt BEFORE INSERT ON production_operations BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	failed := performRequest(t, r, http.MethodPut, path, input)
	if failed.Code != 500 {
		t.Fatalf("injection: %d %s", failed.Code, failed.Body.String())
	}
	var stored models.PrintRun
	if err := db.Where("print_job_id = ?", id).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "in_progress" {
		t.Fatalf("partial write: %s", stored.Status)
	}
	if err := db.Exec("DROP TRIGGER fail_production_receipt").Error; err != nil {
		t.Fatal(err)
	}
	completed := performRequest(t, r, http.MethodPut, path, input)
	if completed.Code != 200 {
		t.Fatalf("failed run: %d %s", completed.Code, completed.Body.String())
	}
	var job models.PrintJob
	if err := db.First(&job, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	var material models.PrintJobFilament
	if err := db.Where("print_job_id = ?", id).First(&material).Error; err != nil {
		t.Fatal(err)
	}
	if material.WeightUsed != 10 || material.PrintRunID == nil {
		t.Fatal("failed consumption lost")
	}
	if job.UnitsPrinted != 0 {
		t.Fatal("failed output counted as accepted")
	}
	input["print_date"] = time.Now().Add(time.Hour).Format(time.RFC3339)
	conflict := performRequest(t, r, http.MethodPut, path, input)
	if conflict.Code != 409 {
		t.Fatalf("changed replay: %d", conflict.Code)
	}
	deleted := performRequest(t, r, http.MethodDelete, "/print-jobs/"+id, nil)
	if deleted.Code != 409 {
		t.Fatalf("delete terminal: %d", deleted.Code)
	}
}

func TestLegacyProductionNormalizationPreservesIdentity(t *testing.T) {
	r, db := setupTestRouter(t)
	fixture := createModernCostFixture(t, r)
	if err := db.Transaction(models.NormalizeLegacyProduction); err != nil {
		t.Fatal(err)
	}
	var runs []models.PrintRun
	if err := db.Preload("Items").Where("print_job_id = ?", fixture.JobID).Find(&runs).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(runs)
	if err := db.Transaction(models.NormalizeLegacyProduction); err != nil {
		t.Fatal(err)
	}
	var after []models.PrintRun
	if err := db.Preload("Items").Where("print_job_id = ?", fixture.JobID).Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(after)
	if string(before) != string(encoded) || len(after) != 1 || after[0].Items[0].Accepted != 0 || after[0].Provenance != "legacy_unverified" {
		t.Fatalf("unsafe legacy mapping: %s", encoded)
	}
}

func TestProductionBackupPreservesRetryResult(t *testing.T) {
	source, _ := setupDesktopTestRouter(t)
	printer := addTestPrinter(t, source, "Lifecycle backup printer", "A1")
	input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": "2026-11-17T00:00:00Z", "runs": []any{map[string]any{"status": "planned", "estimated_time_minutes": 63, "items": []any{map[string]any{"planned": 1}}, "materials": []any{}}}}
	created := performRequest(t, source, http.MethodPost, "/v2/production/jobs", input)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, _ := setupDesktopTestRouter(t)
	for n := 0; n < 2; n++ {
		restored := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "production.zip", backup.Body.Bytes())
		if restored.Code != 200 {
			t.Fatalf("restore: %d %s", restored.Code, restored.Body.String())
		}
		var saved map[string]any
		decodeBody(t, created, &saved)
		loaded := performRequest(t, target, http.MethodGet, "/v2/production/jobs/"+saved["id"].(string), nil)
		var restoredData map[string]any
		decodeBody(t, loaded, &restoredData)
		run := restoredData["runs"].([]any)[0].(map[string]any)
		if run["estimated_time_minutes"] != float64(63) || run["print_time_minutes"] != nil {
			t.Fatal("estimate lost or became actual after restore")
		}
		replay := performRequest(t, target, http.MethodPost, "/v2/production/jobs", input)
		if replay.Code != 201 || replay.Body.String() != created.Body.String() {
			t.Fatalf("restore changed retry: %d %s", replay.Code, replay.Body.String())
		}
	}
}

func TestProductionCompletedOutputAndLegacyGuards(t *testing.T) {
	r, db, printer := setupTestRouterWithDefaultPrinter(t)
	input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": "2026-11-17T00:00:00Z", "runs": []any{map[string]any{"status": "planned", "items": []any{map[string]any{"planned": 3}, map[string]any{"planned": 2}}, "materials": []any{}}}}
	result := performRequest(t, r, http.MethodPost, "/v2/production/jobs", input)
	if result.Code != 201 {
		t.Fatal(result.Body.String())
	}
	var data map[string]any
	decodeBody(t, result, &data)
	id := data["id"].(string)
	for _, status := range []string{"in_progress", "completed"} {
		input["runs"] = data["runs"]
		input["expected_revision"] = data["revision"]
		input["operation_id"] = uuid.NewString()
		run := input["runs"].([]any)[0].(map[string]any)
		run["status"] = status
		if status == "completed" {
			for _, raw := range run["items"].([]any) {
				item := raw.(map[string]any)
				item["attempted"] = item["planned"]
				item["accepted"] = item["planned"]
			}
		}
		result = performRequest(t, r, http.MethodPut, "/v2/production/jobs/"+id, input)
		if result.Code != 200 {
			t.Fatalf("%s: %d %s", status, result.Code, result.Body.String())
		}
		decodeBody(t, result, &data)
	}
	var job models.PrintJob
	if err := db.First(&job, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if job.UnitsPrinted != 5 || job.ModelID != nil {
		t.Fatal("incorrect mixed output projection")
	}
	legacy := performRequest(t, r, http.MethodPut, "/print-jobs/"+id, map[string]any{"print_date": "2026-11-17T00:00:00Z", "units_printed": 1, "labor_hours": 0, "printer_id": printer.ID})
	if legacy.Code != 409 {
		t.Fatalf("legacy write: %d", legacy.Code)
	}
	input["runs"] = data["runs"]
	input["expected_revision"] = data["revision"]
	input["operation_id"] = uuid.NewString()
	run := input["runs"].([]any)[0].(map[string]any)
	item := run["items"].([]any)[0].(map[string]any)
	item["accepted"] = 0
	item["rejected"] = item["attempted"]
	result = performRequest(t, r, http.MethodPut, "/v2/production/jobs/"+id, input)
	if result.Code != 409 {
		t.Fatalf("terminal correction: %d", result.Code)
	}
}

func TestProductionEstimatedTimeValidationAndRollback(t *testing.T) {
	r, db, printer := setupTestRouterWithDefaultPrinter(t)
	run := map[string]any{"status": "planned", "estimated_time_minutes": 63, "items": []any{map[string]any{"planned": 1}}, "materials": []any{}}
	input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": "2026-11-17T00:00:00Z", "runs": []any{run}}
	for _, bad := range []any{-1, 1000000001, 1.5} {
		run["estimated_time_minutes"] = bad
		response := performRequest(t, r, http.MethodPost, "/v2/production/jobs", input)
		if response.Code != 400 {
			t.Fatalf("invalid estimate accepted: %s", response.Body.String())
		}
	}
	var count int64
	db.Model(&models.PrintJob{}).Count(&count)
	if count != 0 {
		t.Fatal("invalid command wrote data")
	}
	run["estimated_time_minutes"] = 63
	response := performRequest(t, r, http.MethodPost, "/v2/production/jobs", input)
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	var result map[string]any
	decodeBody(t, response, &result)
	run = result["runs"].([]any)[0].(map[string]any)
	if run["estimated_time_minutes"] != float64(63) || run["print_time_minutes"] != nil {
		t.Fatal("estimate became actual")
	}
	input["runs"] = []any{run}
	input["expected_revision"] = result["revision"]
	input["operation_id"] = uuid.NewString()
	run["estimated_time_minutes"] = 80
	if err := db.Exec("CREATE TRIGGER fail_estimate BEFORE INSERT ON production_operations BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	response = performRequest(t, r, http.MethodPut, "/v2/production/jobs/"+result["id"].(string), input)
	if response.Code != 500 {
		t.Fatal(response.Code)
	}
	var stored models.PrintRun
	if err := db.First(&stored, "id = ?", run["id"]).Error; err != nil {
		t.Fatal(err)
	}
	if stored.EstimatedTimeMinutes == nil || *stored.EstimatedTimeMinutes != 63 {
		t.Fatal("failed update changed estimate")
	}
}

func TestProductionOlderBackupRestoresUnknownEstimateAndRejectsFalseSchema19(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	printer := addTestPrinter(t, source, "Synthetic legacy printer", "Test")
	input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": "2026-11-17T00:00:00Z", "runs": []any{map[string]any{"status": "planned", "items": []any{map[string]any{"planned": 1}}, "materials": []any{}}}}
	created := performRequest(t, source, http.MethodPost, "/v2/production/jobs", input)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var data map[string]any
	decodeBody(t, created, &data)
	if err := db.Migrator().DropColumn(&models.PrintRun{}, "estimated_time_minutes"); err != nil {
		t.Fatal(err)
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	invalid := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "layout.zip", backup.Body.Bytes())
	if invalid.Code != 400 {
		t.Fatal("schema 19 accepted missing column")
	}
	var count int64
	targetDB.Model(&models.PrintJob{}).Count(&count)
	if count != 0 {
		t.Fatal("failed restore wrote data")
	}
	older := rewriteBackupSchemaVersion(t, backup.Body.Bytes(), 18)
	for i := 0; i < 2; i++ {
		restored := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "older.zip", older)
		if restored.Code != 200 {
			t.Fatal(restored.Body.String())
		}
		loaded := performRequest(t, target, http.MethodGet, "/v2/production/jobs/"+data["id"].(string), nil)
		var result map[string]any
		decodeBody(t, loaded, &result)
		if result["runs"].([]any)[0].(map[string]any)["estimated_time_minutes"] != nil {
			t.Fatal("fabricated estimate for older backup")
		}
	}
}
