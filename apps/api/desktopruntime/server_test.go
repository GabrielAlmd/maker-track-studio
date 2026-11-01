package desktopruntime

import (
	"path/filepath"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

func TestInitDBSQLiteSeedsDefaultsAndSupportsUUIDInserts(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "app.db"))

	db, err := initSQLiteDB()
	if err != nil {
		t.Fatalf("failed to init sqlite db: %v", err)
	}

	expectedKeys := []string{
		models.AppFeaturesConfigKey,
		"electricity_cost_kwh",
		"printer_wear_cost_hour",
		"labor_cost_hour",
		"filament_waste_factor",
	}

	for _, key := range expectedKeys {
		var count int64
		if err := db.Model(&models.AppConfig{}).Where("key = ?", key).Count(&count).Error; err != nil {
			t.Fatalf("failed to query app_config key %s: %v", key, err)
		}
		if count != 1 {
			t.Fatalf("expected app_config key %s to be seeded exactly once, got %d", key, count)
		}
	}
	var features models.AppConfig
	if err := db.First(&features, "key = ?", models.AppFeaturesConfigKey).Error; err != nil {
		t.Fatal(err)
	}
	if features.Value != `{"sales_orders":false}` || features.Type != "json" {
		t.Fatalf("unexpected default feature config: %#v", features)
	}

	filament := models.Filament{
		Brand:          "Desktop",
		Type:           "PLA",
		Color:          "White",
		AvgPrice:       20.0,
		PersonalScore:  4,
		TimesUsed:      0,
		TimesPurchased: 0,
	}

	if err := db.Create(&filament).Error; err != nil {
		t.Fatalf("failed to create filament in sqlite db: %v", err)
	}

	if filament.ID == uuid.Nil {
		t.Fatal("expected filament UUID to be generated before insert")
	}
}
