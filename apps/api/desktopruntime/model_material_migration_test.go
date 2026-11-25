package desktopruntime

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestModelMaterialMigrationPreservesUnknownLegacySplit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	t.Setenv("SQLITE_PATH", path)
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&models.Model{}, &models.PrintJob{}); err != nil {
		t.Fatal(err)
	}
	if legacy.Migrator().HasTable(&models.ModelMaterialRequirement{}) {
		t.Fatal("fixture must predate the requirement table")
	}
	model := models.Model{Name: "Legacy", MaterialWeight: 15, WasteMaterial: 2, PrintTime: 30, IsMulticolor: true}
	if err := legacy.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	job := models.PrintJob{PrintDate: time.Now().UTC(), ModelID: &model.ID, UnitsPrinted: 2}
	if err := legacy.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	legacyConn, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyConn.Close(); err != nil {
		t.Fatal(err)
	}

	for i := range 2 {
		migrated, err := initSQLiteDB()
		if err != nil {
			t.Fatal(err)
		}
		conn, err := migrated.DB()
		if err != nil {
			t.Fatal(err)
		}
		var saved models.Model
		if err := migrated.Preload("MaterialRequirements").Preload("PrintJobs").First(&saved, "id = ?", model.ID).Error; err != nil {
			t.Fatal(err)
		}
		if saved.MaterialWeight != model.MaterialWeight || saved.WasteMaterial != model.WasteMaterial ||
			saved.IsMulticolor != model.IsMulticolor || len(saved.PrintJobs) != 1 || saved.PrintJobs[0].ID != job.ID {
			t.Fatalf("legacy records changed: %+v", saved)
		}
		if i == 0 && len(saved.MaterialRequirements) != 0 {
			t.Fatalf("migration guessed the legacy split: %+v", saved.MaterialRequirements)
		}
		if i == 1 && (len(saved.MaterialRequirements) != 1 || saved.MaterialRequirements[0].PartWeightG != 15) {
			t.Fatalf("second migration changed requirements: %+v", saved.MaterialRequirements)
		}
		if i == 0 {
			row := models.ModelMaterialRequirement{ModelID: model.ID, MaterialType: "PLA", Color: "Blue", PartWeightG: 15}
			if err := migrated.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
