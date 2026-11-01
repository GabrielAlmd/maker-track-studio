package handlers_test

import (
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/gorm"
)

func TestSchemaTwoBackupPreservesProductionAndInitializesSalesEmpty(t *testing.T) {
	source, sourceDB := setupDesktopTestRouter(t)
	filament := models.Filament{Brand: "Schema two", Type: "PLA", Color: "Green", AvgPrice: 21}
	if err := sourceDB.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{
		&salesmodels.SalesOrderAllocation{},
		&salesmodels.SalesOrderItem{},
		&salesmodels.SalesOrder{},
		&salesmodels.SalesChannel{},
	} {
		if err := sourceDB.Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}
	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != http.StatusOK {
		t.Fatalf("export: %d %s", backup.Code, backup.Body.String())
	}
	legacyBackup := rewriteBackupSchemaVersion(t, backup.Body.Bytes(), 2)

	target, targetDB := setupDesktopTestRouter(t)
	if err := targetDB.Create(&salesmodels.SalesChannel{Name: "Replace me"}).Error; err != nil {
		t.Fatal(err)
	}
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "schema-2.zip", legacyBackup)
	if result.Code != http.StatusOK {
		t.Fatalf("schema-2 import: %d %s", result.Code, result.Body.String())
	}

	var restored models.Filament
	if err := targetDB.First(&restored, "id = ?", filament.ID).Error; err != nil {
		t.Fatalf("schema-2 restore lost production data: %v", err)
	}
	for name, model := range map[string]any{
		"channels": &salesmodels.SalesChannel{}, "orders": &salesmodels.SalesOrder{},
		"items": &salesmodels.SalesOrderItem{}, "allocations": &salesmodels.SalesOrderAllocation{},
	} {
		var count int64
		if err := targetDB.Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("schema-2 restore left %d Sales %s", count, name)
		}
	}
}

func TestSalesBackupRejectsInvalidSnapshotsBeforeReplacingTarget(t *testing.T) {
	tests := []struct {
		name       string
		invalidate func(*gorm.DB, salesBackupFixture) error
	}{
		{
			name: "negative order production snapshot",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrder{}).Where("id = ?", fixture.order.ID).
					UpdateColumn("production_cost_cents", -1).Error
			},
		},
		{
			name: "negative allocation production snapshot",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrderAllocation{}).Where("id = ?", fixture.allocation.ID).
					UpdateColumn("production_cost_cents", -1).Error
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, sourceDB := setupDesktopTestRouter(t)
			fixture := createSalesBackupFixture(t, source, sourceDB)
			if err := sourceDB.Exec("PRAGMA ignore_check_constraints = ON").Error; err != nil {
				t.Fatal(err)
			}
			if err := test.invalidate(sourceDB, fixture); err != nil {
				t.Fatal(err)
			}
			if err := sourceDB.Exec("PRAGMA ignore_check_constraints = OFF").Error; err != nil {
				t.Fatal(err)
			}
			backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
			if backup.Code != http.StatusOK {
				t.Fatalf("export: %d %s", backup.Code, backup.Body.String())
			}

			target, targetDB := setupDesktopTestRouter(t)
			kept := salesmodels.SalesChannel{Name: "Keep after invalid snapshot"}
			if err := targetDB.Create(&kept).Error; err != nil {
				t.Fatal(err)
			}
			result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "invalid-snapshot.zip", backup.Body.Bytes())
			if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "is invalid") {
				t.Fatalf("invalid snapshot response: %d %s", result.Code, result.Body.String())
			}
			var restored salesmodels.SalesChannel
			if err := targetDB.First(&restored, "id = ?", kept.ID).Error; err != nil || restored.Name != kept.Name {
				t.Fatalf("invalid snapshot replaced target data: %#v %v", restored, err)
			}
		})
	}
}
