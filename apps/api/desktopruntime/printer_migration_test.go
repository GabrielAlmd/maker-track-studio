package desktopruntime

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestPrinterMigrationPreservesExistingJobsAndSettings(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "legacy.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Migrator().DropTable(&models.Printer{}); err != nil {
		t.Fatal(err)
	}
	job := models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 1}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AppConfig{}).Where("key = ?", "printer_wear_cost_hour").Update("value", "0.42").Error; err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		migrated, err := initSQLiteDB()
		if err != nil {
			t.Fatal(err)
		}
		connection, err := migrated.DB()
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if !migrated.Migrator().HasTable(&models.Printer{}) {
			t.Fatal("printers table missing")
		}
		var printers []models.Printer
		if err := migrated.Find(&printers).Error; err != nil || len(printers) != 1 {
			t.Fatalf("migration must create one default printer: %#v %v", printers, err)
		}
		if !printers[0].IsDefault || printers[0].CostConfig["printer_wear_cost_hour"] != 0.42 {
			t.Fatalf("migrated printer did not preserve settings: %#v", printers[0])
		}
		var saved models.PrintJob
		if err := migrated.Where("id = ?", job.ID).First(&saved).Error; err != nil {
			t.Fatal(err)
		}
		if saved.UnitsPrinted != job.UnitsPrinted {
			t.Fatal("existing job changed")
		}
		if saved.PrinterID != nil || saved.PrinterCost != nil {
			t.Fatalf("legacy job was silently assigned or snapshotted: %#v", saved)
		}
		var config models.AppConfig
		if err := migrated.Where("key = ?", "printer_wear_cost_hour").First(&config).Error; err != nil {
			t.Fatal(err)
		}
		if config.Value != "0.42" {
			t.Fatal("existing cost configuration changed")
		}
	}
}

func TestPrinterMigrationCreatesDefaultForEmptyLegacyDatabase(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "empty-legacy.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	var initialPrinters int64
	if err := db.Model(&models.Printer{}).Count(&initialPrinters).Error; err != nil {
		t.Fatal(err)
	}
	if initialPrinters != 0 {
		t.Fatalf("brand-new database unexpectedly created %d printers", initialPrinters)
	}
	if err := db.Model(&models.AppConfig{}).Where("key = ?", "printer_wear_cost_hour").Update("value", "0.42").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&models.Printer{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := migrated.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var printers []models.Printer
	if err := migrated.Find(&printers).Error; err != nil {
		t.Fatal(err)
	}
	if len(printers) != 1 || !printers[0].IsDefault {
		t.Fatalf("empty legacy database did not create one default: %#v", printers)
	}
	if printers[0].CostConfig["printer_wear_cost_hour"] != 0.42 {
		t.Fatalf("legacy global costs were not copied: %#v", printers[0].CostConfig)
	}
}
