package handlers_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The oracle uses fixed, independently calculated fixtures: 60 minutes cost
// 2 EUR electricity + 2 EUR wear; 100 g at 20 EUR/kg cost 2 EUR. Labor is a
// batch total at 12 EUR/hour, never implicitly multiplied by units.
func assertRegressionJobCost(t *testing.T, r *gin.Engine, db *gorm.DB, id string, units int, hours, scale float64) {
	t.Helper()
	wantLabor := hours * 12
	wantTotal := 6*scale + wantLabor
	var cost apiResponse
	response := performRequest(t, r, http.MethodGet, "/print-jobs/"+id+"/cost", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("cost: %d %s", response.Code, response.Body.String())
	}
	decodeBody(t, response, &cost)
	if cost["state"] != "calculated" {
		t.Fatalf("cost must be calculated: %#v", cost)
	}
	for key, want := range map[string]float64{
		"filament_cost": 2 * scale, "electricity_cost": 2 * scale,
		"wear_cost": 2 * scale, "labor_cost": wantLabor, "total_cost": wantTotal,
	} {
		t.Run(key, func(t *testing.T) { assertFloat(t, cost[key], want) })
	}
	var saved models.PrintJob
	if err := db.First(&saved, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.UnitsPrinted != units || float64(saved.LaborHours) != hours {
		t.Fatalf("persisted units/hours = %d/%v, want %d/%v", saved.UnitsPrinted, saved.LaborHours, units, hours)
	}
	var materialCount int64
	if err := db.Model(&models.PrintJobFilament{}).Where("print_job_id = ?", id).Count(&materialCount).Error; err != nil || materialCount != 1 {
		t.Fatalf("material rows = %d, want 1: %v", materialCount, err)
	}
	for _, path := range []string{"/print-jobs/" + id, "/print-jobs?page=1&pageSize=25", "/print-jobs/stats", "/print-jobs/dashboard-summary"} {
		t.Run(path, func(t *testing.T) {
			response := performRequest(t, r, http.MethodGet, path, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
			}
			var body apiResponse
			decodeBody(t, response, &body)
			switch path {
			case "/print-jobs?page=1&pageSize=25":
				items := body["items"].([]any)
				if len(items) != 1 {
					t.Fatalf("expected one job, got %d", len(items))
				}
				assertFloat(t, items[0].(map[string]any)["estimated_cost"], wantTotal)
			case "/print-jobs/stats":
				assertFloat(t, body["total_cost"], wantTotal)
				assertFloat(t, body["total_labor_cost"], wantLabor)
			case "/print-jobs/dashboard-summary":
				stats := body["cost_stats"].(map[string]any)
				assertFloat(t, stats["total_cost"], wantTotal)
				assertFloat(t, stats["total_labor_cost"], wantLabor)
			default:
				assertFloat(t, body["estimated_cost"], wantTotal)
				assertFloat(t, body["labor_hours"], hours)
				assertFloat(t, body["units_printed"], float64(units))
			}
		})
	}
}

func TestPrintJobLaborUnitsCostMatrix(t *testing.T) {
	for _, automaticTotals := range []bool{false, true} {
		for _, units := range []int{1, 3, 10} {
			for _, hours := range []float64{0, 0.25, 0.5, 1.5, 2.75} {
				t.Run(fmt.Sprintf("automatic=%t/units=%d/hours=%g", automaticTotals, units, hours), func(t *testing.T) {
					r, db := setupTestRouter(t)
					p := configurePrinter(t, r, addTestPrinter(t, r, "Regression printer", "Fixture"), 2, 2, 12)
					p.CostConfig["filament_waste_factor"] = 1
					configurePrinter(t, r, p, 2, 2, 12)
					filament := models.Filament{Brand: "Regression", Type: "PLA", Color: "Blue", AvgPrice: 20}
					if err := db.Create(&filament).Error; err != nil {
						t.Fatal(err)
					}
					scale := 1
					if automaticTotals {
						scale = units
					}
					payload := printerJobPayload(p.ID.String())
					payload["units_printed"], payload["labor_hours"], payload["print_time_minutes"] = units, hours, 60*scale
					created := performRequest(t, r, http.MethodPost, "/print-jobs", payload)
					if created.Code != http.StatusCreated {
						t.Fatalf("create: %d %s", created.Code, created.Body.String())
					}
					var job dto.PrintJobDTO
					decodeBody(t, created, &job)
					usage := performRequest(t, r, http.MethodPost, "/print-job-filaments", map[string]any{
						"print_job_id": job.ID, "filament_id": filament.ID, "weight_used": 100 * scale,
					})
					if usage.Code != http.StatusCreated {
						t.Fatalf("material: %d %s", usage.Code, usage.Body.String())
					}
					assertRegressionJobCost(t, r, db, job.ID.String(), units, hours, float64(scale))
				})
			}
		}
	}
}

func TestPrintJobLaborUnitsRepeatedEditsPreserveSnapshotAndDoNotAccumulateCost(t *testing.T) {
	r, db := setupTestRouter(t)
	p := configurePrinter(t, r, addTestPrinter(t, r, "Original", "Fixture"), 2, 2, 12)
	p.CostConfig["filament_waste_factor"] = 1
	configurePrinter(t, r, p, 2, 2, 12)
	job := createPrinterJob(t, r, p.ID.String())
	filament := models.Filament{Brand: "Regression", Type: "PLA", Color: "Blue", AvgPrice: 20}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	usage := performRequest(t, r, http.MethodPost, "/print-job-filaments", map[string]any{
		"print_job_id": job.ID, "filament_id": filament.ID, "weight_used": 100,
	})
	if usage.Code != http.StatusCreated {
		t.Fatal(usage.Body.String())
	}
	configurePrinter(t, r, p, 200, 200, 1200)
	for i, step := range []struct {
		units int
		hours float64
	}{
		{1, 0}, {1, 0.5}, {3, 0.5}, {3, 1.5}, {10, 1.5}, {1, 0}, {1, 0},
	} {
		t.Run(fmt.Sprintf("step=%d/units=%d/hours=%g", i, step.units, step.hours), func(t *testing.T) {
			payload := printerJobPayload(p.ID.String())
			payload["units_printed"], payload["labor_hours"] = step.units, step.hours
			updated := performRequest(t, r, http.MethodPut, "/print-jobs/"+job.ID.String(), payload)
			if updated.Code != http.StatusOK {
				t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
			}
			var current dto.PrintJobDTO
			decodeBody(t, updated, &current)
			if !reflect.DeepEqual(current.PrinterCost, job.PrinterCost) {
				t.Fatal("editing hours/units replaced the historical snapshot")
			}
			assertRegressionJobCost(t, r, db, job.ID.String(), step.units, step.hours, 1)
		})
	}
}

func TestPrintJobUnitsRejectNonPositiveValuesWithoutChangingData(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, units := range []int{0, -1, -100} {
			t.Run(fmt.Sprintf("%s/units=%d", method, units), func(t *testing.T) {
				r, db := setupTestRouter(t)
				p := addTestPrinter(t, r, "Validation", "Fixture")
				path := "/print-jobs"
				var original dto.PrintJobDTO
				if method == http.MethodPut {
					original = createPrinterJob(t, r, p.ID.String())
					path += "/" + original.ID.String()
				}
				payload := printerJobPayload(p.ID.String())
				payload["units_printed"] = units
				response := performRequest(t, r, method, path, payload)
				if response.Code != http.StatusBadRequest {
					t.Errorf("expected invalid units rejected with 400; got %d: %s", response.Code, response.Body.String())
				}
				var jobs []models.PrintJob
				if err := db.Find(&jobs).Error; err != nil {
					t.Fatal(err)
				}
				if method == http.MethodPost && len(jobs) != 0 {
					t.Errorf("invalid job persisted: %#v", jobs)
				}
				if method == http.MethodPut && (len(jobs) != 1 || jobs[0].UnitsPrinted != original.UnitsPrinted) {
					t.Errorf("invalid edit changed persisted units: %#v", jobs)
				}
			})
		}
	}
}

func TestPrintJobLaborZeroSnapshotAndLegacyRateRemainDistinct(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		legacy    bool
		rate      float64
		wantLabor float64
	}{
		{"zero captured rate", false, 0, 0},
		{"positive captured rate", false, 12, 18},
		{"legacy global rate", true, 12, 18},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r, db := setupTestRouter(t)
			for key, value := range map[string]string{"electricity_cost_kwh": "2", "printer_wear_cost_hour": "2", "labor_cost_hour": "12"} {
				if err := db.Create(&models.AppConfig{Key: key, Value: value, Type: "number"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			var id string
			if scenario.legacy {
				minutes := 60
				job := models.PrintJob{UnitsPrinted: 3, LaborHours: 1.5, PrintTimeMinutes: &minutes, PrintDate: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)}
				if err := db.Create(&job).Error; err != nil {
					t.Fatal(err)
				}
				id = job.ID.String()
			} else {
				p := configurePrinter(t, r, addTestPrinter(t, r, "Rate fixture", "Fixture"), 2, 2, scenario.rate)
				payload := printerJobPayload(p.ID.String())
				payload["labor_hours"], payload["units_printed"] = 1.5, 3
				response := performRequest(t, r, http.MethodPost, "/print-jobs", payload)
				if response.Code != http.StatusCreated {
					t.Fatal(response.Body.String())
				}
				var job dto.PrintJobDTO
				decodeBody(t, response, &job)
				id = job.ID.String()
				// A profile change cannot silently replace the original zero or
				// positive tariff captured when this job was created.
				configurePrinter(t, r, p, 200, 200, 1200)
			}
			response := performRequest(t, r, http.MethodGet, "/print-jobs/"+id+"/cost", nil)
			if response.Code != http.StatusOK {
				t.Fatal(response.Body.String())
			}
			var cost apiResponse
			decodeBody(t, response, &cost)
			if cost["state"] != "calculated" {
				t.Fatalf("cost unavailable: %#v", cost)
			}
			assertFloat(t, cost["labor_cost"], scenario.wantLabor)
			assertFloat(t, cost["total_cost"], 4+scenario.wantLabor)
		})
	}
}

func TestPrintJobLaborWithoutDurationReportsUnavailableInsteadOfZeroCost(t *testing.T) {
	r, _ := setupTestRouter(t)
	p := configurePrinter(t, r, addTestPrinter(t, r, "Missing time", "Fixture"), 2, 2, 12)
	payload := printerJobPayload(p.ID.String())
	payload["labor_hours"] = 1.5
	delete(payload, "print_time_minutes")
	response := performRequest(t, r, http.MethodPost, "/print-jobs", payload)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	var job apiResponse
	decodeBody(t, response, &job)
	assertFloat(t, job["labor_hours"], 1.5)
	if job["estimated_cost"] != nil {
		t.Fatalf("expected unavailable total without duration, got %#v", job["estimated_cost"])
	}
	response = performRequest(t, r, http.MethodGet, "/print-jobs/"+job["id"].(string)+"/cost", nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var cost apiResponse
	decodeBody(t, response, &cost)
	if cost["state"] != "not_calculated" || cost["reason"] != "Print time is not available" || cost["total_cost"] != nil {
		t.Fatalf("expected a reason for unavailable cost, got %#v", cost)
	}
}
