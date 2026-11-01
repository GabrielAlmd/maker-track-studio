package handlers_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func configurePrinter(t *testing.T, r *gin.Engine, p models.Printer, electricity, wear, labor float64) models.Printer {
	t.Helper()
	p.CostConfig["electricity_cost_kwh"] = electricity
	p.CostConfig["printer_wear_cost_hour"] = wear
	p.CostConfig["labor_cost_hour"] = labor
	res := performRequest(t, r, "PUT", "/printers/"+p.ID.String()+"/cost-config", map[string]any{"cost_config": p.CostConfig})
	if res.Code != 200 {
		t.Fatalf("configure printer: %d %s", res.Code, res.Body.String())
	}
	decodeBody(t, res, &p)
	return p
}

func printerJobPayload(printerID any) map[string]any {
	return map[string]any{
		"printer_id": printerID, "print_date": "2026-09-19T12:00:00Z",
		"units_printed": 1, "labor_hours": 1, "print_time_minutes": 60,
	}
}

func createPrinterJob(t *testing.T, r *gin.Engine, printerID any) dto.PrintJobDTO {
	t.Helper()
	res := performRequest(t, r, "POST", "/print-jobs", printerJobPayload(printerID))
	if res.Code != 201 {
		t.Fatalf("create job: %d %s", res.Code, res.Body.String())
	}
	var job dto.PrintJobDTO
	decodeBody(t, res, &job)
	return job
}

func assertPrinterJobCost(t *testing.T, r *gin.Engine, job dto.PrintJobDTO, expected float64) {
	t.Helper()
	res := performRequest(t, r, "GET", "/print-jobs/"+job.ID.String()+"/cost", nil)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var cost apiResponse
	decodeBody(t, res, &cost)
	assertFloat(t, cost["total_cost"], expected)
}

func TestPrinterCostSnapshotsStableAcrossProfileAndGlobalChanges(t *testing.T) {
	r, db := setupTestRouter(t)
	first := configurePrinter(t, r, addTestPrinter(t, r, "A1 workshop", "A1"), 2, 1, 3)
	second := configurePrinter(t, r, addTestPrinter(t, r, "A1 office", "A1"), 5, 4, 6)
	job := createPrinterJob(t, r, nil) // default selected by the API
	if job.PrinterID == nil || *job.PrinterID != first.ID || job.PrinterCost == nil {
		t.Fatalf("default or snapshot missing: %#v", job)
	}
	assertPrinterJobCost(t, r, job, 6)
	configurePrinter(t, r, first, 20, 10, 30)
	if err := db.Create(&models.AppConfig{Key: "labor_cost_hour", Value: "999", Type: "number"}).Error; err != nil {
		t.Fatal(err)
	}
	assertPrinterJobCost(t, r, job, 6)
	for _, selection := range []any{nil, first.ID.String()} {
		res := performRequest(t, r, "PUT", "/print-jobs/"+job.ID.String(), printerJobPayload(selection))
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		var updated dto.PrintJobDTO
		decodeBody(t, res, &updated)
		if !reflect.DeepEqual(updated.PrinterCost, job.PrinterCost) {
			t.Fatal("editing unchanged assignment replaced historical snapshot")
		}
		assertPrinterJobCost(t, r, job, 6)
	}
	res := performRequest(t, r, "PUT", "/print-jobs/"+job.ID.String(), printerJobPayload(second.ID.String()))
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	decodeBody(t, res, &job)
	if job.PrinterID == nil || *job.PrinterID != second.ID {
		t.Fatal("explicit reassignment failed")
	}
	assertPrinterJobCost(t, r, job, 15)
	// Every cost consumer must use the same snapshot.
	list := performRequest(t, r, "GET", "/print-jobs", nil)
	var jobs []apiResponse
	decodeBody(t, list, &jobs)
	assertFloat(t, jobs[0]["estimated_cost"], 15)
	stats := performRequest(t, r, "GET", "/print-jobs/stats", nil)
	var totals apiResponse
	decodeBody(t, stats, &totals)
	assertFloat(t, totals["total_cost"], 15)

	backup := performRequest(t, r, "GET", "/desktop-data/export", nil)
	target, _ := setupTestRouter(t)
	imported := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if imported.Code != 200 {
		t.Fatal(imported.Body.String())
	}
	assertPrinterJobCost(t, target, job, 15)
	restored := performRequest(t, target, "GET", "/print-jobs/"+job.ID.String(), nil)
	var restoredJob dto.PrintJobDTO
	decodeBody(t, restored, &restoredJob)
	if !reflect.DeepEqual(job.PrinterCost, restoredJob.PrinterCost) {
		t.Fatal("backup changed historical inputs")
	}
}

func TestPrintJobRepresentationHidesLegacyMarginTiersWithoutRewritingSnapshot(t *testing.T) {
	r, db := setupTestRouter(t)
	printer := addTestPrinter(t, r, "Legacy pricing", "A1")
	printer.CostConfig["margin_tier_1"] = 50
	printer = configurePrinter(t, r, printer, 1, 2, 3)
	job := createPrinterJob(t, r, printer.ID.String())
	if _, exposed := job.PrinterCost.Config["margin_tier_1"]; exposed {
		t.Fatal("legacy margin tier exposed in Print Job response")
	}

	var persisted models.PrintJob
	if err := db.First(&persisted, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.PrinterCost.Config["margin_tier_1"] != 50 {
		t.Fatal("legacy snapshot was rewritten instead of preserved for compatibility")
	}
}

func TestPrinterAdvancedSnapshotNormalizesExactlyOnce(t *testing.T) {
	r, _ := setupTestRouter(t)
	p := addTestPrinter(t, r, "Advanced", "P1S")
	p.CostConfig["machine_cost_mode"] = 1
	p.CostConfig["printer_purchase_cost"] = 8760
	p.CostConfig["printer_upfront_cost"] = 0
	p.CostConfig["printer_annual_maintenance"] = 0
	p.CostConfig["printer_lifetime_years"] = 1
	p.CostConfig["printer_uptime_percent"] = 100
	p.CostConfig["printer_buffer_factor"] = 2
	p.CostConfig["printer_power_watts"] = 500
	p = configurePrinter(t, r, p, 2, 999, 3)
	job := createPrinterJob(t, r, p.ID.String())
	// Electricity = 2 * 0.5 * 2 = 2; wear = 8760 / 8760 * 2 = 2; labor = 3.
	assertPrinterJobCost(t, r, job, 7)
	assertPrinterJobCost(t, r, job, 7)
	if job.PrinterCost.Config["printer_wear_cost_hour"] != 999 {
		t.Fatal("normalization mutated raw snapshot")
	}
}

func TestPrinterCostsValidationAndArchivedAssignment(t *testing.T) {
	r, _ := setupTestRouter(t)
	p := addTestPrinter(t, r, "First", "A1")
	q := addTestPrinter(t, r, "Second", "A1")
	job := createPrinterJob(t, r, q.ID.String())
	res := performRequest(t, r, "PUT", "/printers/"+q.ID.String()+"/archive", nil)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	res = performRequest(t, r, "PUT", "/print-jobs/"+job.ID.String(), printerJobPayload(q.ID.String()))
	if res.Code != 200 {
		t.Fatal("existing archived assignment should remain editable")
	}
	for _, id := range []string{q.ID.String(), uuid.NewString(), uuid.Nil.String()} {
		res = performRequest(t, r, "POST", "/print-jobs", printerJobPayload(id))
		if res.Code != 400 {
			t.Fatalf("invalid/archived assignment: %d %s", res.Code, res.Body.String())
		}
	}
	for _, key := range []string{"printer_wear_cost_hour", "printer_lifetime_years", "printer_uptime_percent", "printer_buffer_factor", "machine_cost_mode"} {
		config := map[string]float64{}
		for k, v := range p.CostConfig {
			config[k] = v
		}
		config[key] = -1
		res = performRequest(t, r, http.MethodPut, "/printers/"+p.ID.String()+"/cost-config", map[string]any{"cost_config": config})
		if res.Code != 400 {
			t.Fatalf("invalid %s accepted: %s", key, res.Body.String())
		}
	}
	delete(p.CostConfig, "labor_cost_hour")
	res = performRequest(t, r, "PUT", "/printers/"+p.ID.String()+"/cost-config", map[string]any{"cost_config": p.CostConfig})
	if res.Code != 400 {
		t.Fatal("incomplete config accepted")
	}
}

func TestPhaseOnePrinterInitializesCostsWithoutAssigningOldJobs(t *testing.T) {
	r, db := setupTestRouter(t)
	old := models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 1, LaborHours: 1}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if old.PrinterID != nil || old.PrinterCost != nil {
		t.Fatal("legacy fixture unexpectedly has a printer assignment")
	}
	p := addTestPrinter(t, r, "Phase one", "A1")
	current := createPrinterJob(t, r, p.ID.String())
	if current.PrinterCost == nil {
		t.Fatal("phase-one printer did not acquire a snapshot")
	}
	var saved models.Printer
	if err := db.First(&saved, "id = ?", p.ID).Error; err != nil || saved.CostConfig == nil {
		t.Fatalf("profile was not initialized: %v", err)
	}
	var legacy models.PrintJob
	if err := db.First(&legacy, "id = ?", old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.PrinterID != nil || legacy.PrinterCost != nil {
		t.Fatal("old job was silently assigned")
	}
}

func TestCreateJobWithoutPrinterUsesDefaultPrinter(t *testing.T) {
	r, _ := setupTestRouter(t)
	defaultPrinter := configurePrinter(t, r, addTestPrinter(t, r, "Default", "A1"), 1, 2, 10)
	addTestPrinter(t, r, "Other", "P1S")

	job := createPrinterJob(t, r, nil)
	if job.PrinterID == nil || *job.PrinterID != defaultPrinter.ID {
		t.Fatalf("expected default printer %s, got %#v", defaultPrinter.ID, job.PrinterID)
	}
	if job.PrinterCost == nil || job.PrinterCost.Config["labor_cost_hour"] != 10 {
		t.Fatalf("expected default printer snapshot, got %#v", job.PrinterCost)
	}
}

func TestCreateJobWithExplicitPrinterOverridesDefault(t *testing.T) {
	r, _ := setupTestRouter(t)
	configurePrinter(t, r, addTestPrinter(t, r, "Default", "A1"), 1, 2, 10)
	selected := configurePrinter(t, r, addTestPrinter(t, r, "Selected", "P1S"), 3, 4, 20)

	job := createPrinterJob(t, r, selected.ID.String())
	if job.PrinterID == nil || *job.PrinterID != selected.ID {
		t.Fatalf("expected explicit printer %s, got %#v", selected.ID, job.PrinterID)
	}
	if job.PrinterCost == nil || job.PrinterCost.Config["labor_cost_hour"] != 20 {
		t.Fatalf("expected selected printer snapshot, got %#v", job.PrinterCost)
	}
}

func TestCreateJobFailsWithoutActivePrinter(t *testing.T) {
	r, _ := setupTestRouter(t)
	res := performRequest(t, r, http.MethodPost, "/print-jobs", printerJobPayload(nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", res.Code, res.Body.String())
	}
	var body apiResponse
	decodeBody(t, res, &body)
	if body["error"] != "Select an existing active printer" {
		t.Fatalf("unexpected error: %#v", body["error"])
	}
}

func TestPrinterCostChangesOnlyAffectFutureJobs(t *testing.T) {
	r, _ := setupTestRouter(t)
	printer := configurePrinter(t, r, addTestPrinter(t, r, "Changing costs", "A1"), 0, 0, 10)
	jobA := createPrinterJob(t, r, printer.ID.String())

	printer = configurePrinter(t, r, printer, 0, 0, 20)
	jobB := createPrinterJob(t, r, printer.ID.String())

	if jobA.PrinterCost.Config["labor_cost_hour"] != 10 || jobB.PrinterCost.Config["labor_cost_hour"] != 20 {
		t.Fatalf("unexpected snapshots: A=%#v B=%#v", jobA.PrinterCost, jobB.PrinterCost)
	}
	assertPrinterJobCost(t, r, jobA, 10)
	assertPrinterJobCost(t, r, jobB, 20)
}

func TestPrinterReassignmentCreatesNewSnapshot(t *testing.T) {
	r, _ := setupTestRouter(t)
	printerA := configurePrinter(t, r, addTestPrinter(t, r, "Printer A", "A1"), 0, 0, 10)
	printerB := configurePrinter(t, r, addTestPrinter(t, r, "Printer B", "P1S"), 0, 0, 20)
	job := createPrinterJob(t, r, printerA.ID.String())
	original := job.PrinterCost

	res := performRequest(t, r, http.MethodPut, "/print-jobs/"+job.ID.String(), printerJobPayload(nil))
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	var unchanged dto.PrintJobDTO
	decodeBody(t, res, &unchanged)
	if !reflect.DeepEqual(unchanged.PrinterCost, original) {
		t.Fatal("editing without changing printer replaced the snapshot")
	}

	res = performRequest(t, r, http.MethodPut, "/print-jobs/"+job.ID.String(), printerJobPayload(printerB.ID.String()))
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	var reassigned dto.PrintJobDTO
	decodeBody(t, res, &reassigned)
	if reassigned.PrinterID == nil || *reassigned.PrinterID != printerB.ID || reassigned.PrinterCost == nil {
		t.Fatalf("reassignment failed: %#v", reassigned)
	}
	if reflect.DeepEqual(reassigned.PrinterCost, original) || reassigned.PrinterCost.Config["labor_cost_hour"] != 20 {
		t.Fatalf("reassignment did not capture printer B: %#v", reassigned.PrinterCost)
	}
}

func TestArchivedPrinterCannotBeUsedForNewJob(t *testing.T) {
	r, _ := setupTestRouter(t)
	addTestPrinter(t, r, "Default", "A1")
	archived := addTestPrinter(t, r, "Archived", "P1S")
	res := performRequest(t, r, http.MethodPut, "/printers/"+archived.ID.String()+"/archive", nil)
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	res = performRequest(t, r, http.MethodPost, "/print-jobs", printerJobPayload(archived.ID.String()))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("archived printer accepted for new job: %d %s", res.Code, res.Body.String())
	}
}

func TestExistingJobWithArchivedPrinterRemainsCalculable(t *testing.T) {
	r, _ := setupTestRouter(t)
	addTestPrinter(t, r, "Default", "A1")
	archived := configurePrinter(t, r, addTestPrinter(t, r, "Later archived", "P1S"), 0, 0, 5)
	job := createPrinterJob(t, r, archived.ID.String())
	res := performRequest(t, r, http.MethodPut, "/printers/"+archived.ID.String()+"/archive", nil)
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	assertPrinterJobCost(t, r, job, 5)
	res = performRequest(t, r, http.MethodGet, "/print-jobs/"+job.ID.String(), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("archived printer job is not readable: %d %s", res.Code, res.Body.String())
	}
}

func TestReassignmentToArchivedPrinterIsRejected(t *testing.T) {
	r, _ := setupTestRouter(t)
	active := addTestPrinter(t, r, "Active", "A1")
	archived := addTestPrinter(t, r, "Archived", "P1S")
	job := createPrinterJob(t, r, active.ID.String())
	res := performRequest(t, r, http.MethodPut, "/printers/"+archived.ID.String()+"/archive", nil)
	if res.Code != http.StatusOK {
		t.Fatal(res.Body.String())
	}
	res = performRequest(t, r, http.MethodPut, "/print-jobs/"+job.ID.String(), printerJobPayload(archived.ID.String()))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("reassignment to archived printer accepted: %d %s", res.Code, res.Body.String())
	}
}

func TestInvalidPrinterSnapshotImportPreservesTarget(t *testing.T) {
	source, db := setupTestRouter(t)
	p := addTestPrinter(t, source, "Invalid snapshot", "A1")
	job := createPrinterJob(t, source, p.ID.String())
	var stored models.PrintJob
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	stored.PrinterCost.Version = 999
	if err := db.Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	backup := performRequest(t, source, "GET", "/desktop-data/export", nil)
	target, targetDB := setupTestRouter(t)
	original := addTestPrinter(t, target, "Keep this machine", "A1")
	result := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", backup.Body.Bytes())
	if result.Code != 500 {
		t.Fatalf("invalid snapshot import: %d %s", result.Code, result.Body.String())
	}
	var kept []models.Printer
	if err := targetDB.Find(&kept).Error; err != nil || len(kept) != 1 || kept[0].ID != original.ID {
		t.Fatalf("invalid import modified target: %v", err)
	}
}
