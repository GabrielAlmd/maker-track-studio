package handlers_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/gorm/logger"
)

func TestDesktopBackupMixedPrintTimes(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	if err := db.Create(&models.AppConfig{Key: "labor_cost_hour", Value: "15", Type: "number"}).Error; err != nil {
		t.Fatal(err)
	}
	minutes := 60
	var expected []models.PrintJob
	for _, duration := range []*int{nil, &minutes} {
		job := models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 1, PrintTimeMinutes: duration}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		expected = append(expected, job)
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if result.Code != 200 {
		t.Fatalf("import: %d %s", result.Code, result.Body.String())
	}
	var jobs []models.PrintJob
	if err := targetDB.Order("created_at asc").Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	for i, job := range jobs {
		if job.ID != expected[i].ID || !reflect.DeepEqual(job.PrintTimeMinutes, expected[i].PrintTimeMinutes) {
			t.Fatalf("job ID or nullable time changed: %#v", job)
		}
	}
}

func TestDesktopBackupLargeDatasetAndEmptyConfig(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	filaments := make([]models.Filament, 3000)
	for i := range filaments {
		filaments[i] = models.Filament{Brand: "Backup", Type: "PLA", Color: "Black", AvgPrice: 20}
	}
	if err := db.CreateInBatches(&filaments, 50).Error; err != nil {
		t.Fatal(err)
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if result.Code != 200 {
		t.Fatalf("import: %d %s", result.Code, result.Body.String())
	}
	var restored []models.Filament
	if err := targetDB.Order("id").Find(&restored).Error; err != nil {
		t.Fatal(err)
	}
	var original []models.Filament
	if err := db.Order("id").Find(&original).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatal("restored dataset differs from exported data")
	}
}

func TestDesktopBackupFailureRollsBackExistingData(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	if err := db.Create(&models.Filament{Brand: "Imported", Type: "PLA", Color: "Red"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 1}).Error; err != nil {
		t.Fatal(err)
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	targetDB.Logger = logger.Default.LogMode(logger.Silent)
	previous := models.Filament{Brand: "Keep me", Type: "PLA", Color: "Blue"}
	if err := targetDB.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	// Fail after the filament batch has already been replaced.
	if err := targetDB.Exec(`CREATE TRIGGER reject_import BEFORE INSERT ON print_jobs BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "backup.zip", backup.Body.Bytes())
	if result.Code != 500 {
		t.Fatalf("expected failure, got %d", result.Code)
	}
	var restored []models.Filament
	if err := targetDB.Find(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].ID != previous.ID || restored[0].Brand != previous.Brand {
		t.Fatal("existing data was not restored after failure")
	}
}
