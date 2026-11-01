package handlers_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type salesBackupFixture struct {
	channel    salesmodels.SalesChannel
	order      salesmodels.SalesOrder
	item       salesmodels.SalesOrderItem
	allocation salesmodels.SalesOrderAllocation
}

func createSalesBackupFixture(t *testing.T, router *gin.Engine, db *gorm.DB) salesBackupFixture {
	t.Helper()

	productionModel := models.Model{Name: "Dragon", MaterialWeight: 85, PrintTime: 180}
	if err := db.Create(&productionModel).Error; err != nil {
		t.Fatal(err)
	}
	printer := addTestPrinter(t, router, "Sales backup printer", "A1")
	job := createPrinterJob(t, router, printer.ID.String())
	if err := db.Model(&models.PrintJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{"model_id": productionModel.ID, "units_printed": 2}).Error; err != nil {
		t.Fatal(err)
	}
	channel := salesmodels.SalesChannel{
		Name: "Craft marketplace", Notes: "Historical channel", IsArchived: true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	productionCost, totalCost, netProfit, marginBasisPoints := int64(630), int64(1510), int64(1088), int64(4188)
	order := salesmodels.SalesOrder{
		Reference: "SALE-2026-0042", OrderDate: time.Date(2026, time.September, 20, 12, 30, 0, 0, time.UTC),
		SalesChannelID: &channel.ID, FulfilmentStatus: salesmodels.SalesOrderFulfilmentCompleted,
		PaymentStatus: salesmodels.SalesOrderPaymentPaid, PackagingCostCents: 125,
		ShippingCostCents: 450, PlatformFeeCents: 230, OtherCostCents: 75,
		Notes: "Leave at reception", ProductionCostCents: &productionCost,
		TotalCostCents: &totalCost, NetProfitCents: &netProfit, MarginBasisPoints: &marginBasisPoints,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	item := salesmodels.SalesOrderItem{
		SalesOrderID: order.ID, ModelID: &productionModel.ID,
		ModelNameSnapshot: productionModel.Name, Quantity: 2, UnitPriceCents: 1299,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	allocation := salesmodels.SalesOrderAllocation{
		SalesOrderItemID: item.ID, PrintJobID: &job.ID, UnitsAllocated: 2,
		ProductionCostCents: &productionCost,
	}
	if err := db.Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}
	return salesBackupFixture{channel: channel, order: order, item: item, allocation: allocation}
}

func TestSalesBackupSchemaAndRoundTrip(t *testing.T) {
	source, sourceDB := setupDesktopTestRouter(t)
	expected := createSalesBackupFixture(t, source, sourceDB)
	if err := sourceDB.Create(&models.AppConfig{Key: models.AppFeaturesConfigKey, Value: `{"sales_orders":false}`, Type: "json"}).Error; err != nil {
		t.Fatal(err)
	}

	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != http.StatusOK {
		t.Fatalf("export: %d %s", backup.Code, backup.Body.String())
	}
	files := readBackupFiles(t, backup.Body.Bytes())
	var manifest map[string]any
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["schema_version"] != float64(4) {
		t.Fatalf("expected backup schema 4, got %#v", manifest["schema_version"])
	}

	target, targetDB := setupDesktopTestRouter(t)
	if err := targetDB.Create(&salesmodels.SalesChannel{Name: "Must be replaced"}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := targetDB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}

	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "sales.zip", backup.Body.Bytes())
	if result.Code != http.StatusOK {
		t.Fatalf("import: %d %s", result.Code, result.Body.String())
	}

	var channel salesmodels.SalesChannel
	if err := targetDB.First(&channel, "id = ?", expected.channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	if channel.Name != expected.channel.Name || channel.Notes != expected.channel.Notes || !channel.IsArchived {
		t.Fatalf("Sales Channel changed: %#v", channel)
	}
	var order salesmodels.SalesOrder
	if err := targetDB.First(&order, "id = ?", expected.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.Reference != expected.order.Reference || order.SalesChannelID == nil || *order.SalesChannelID != expected.channel.ID ||
		order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCompleted || order.PaymentStatus != salesmodels.SalesOrderPaymentPaid ||
		order.PackagingCostCents != 125 || order.ShippingCostCents != 450 ||
		order.PlatformFeeCents != 230 || order.OtherCostCents != 75 || order.Notes != expected.order.Notes ||
		order.ProductionCostCents == nil || *order.ProductionCostCents != 630 ||
		order.TotalCostCents == nil || *order.TotalCostCents != 1510 ||
		order.NetProfitCents == nil || *order.NetProfitCents != 1088 ||
		order.MarginBasisPoints == nil || *order.MarginBasisPoints != 4188 {
		t.Fatalf("Sales Order changed: %#v", order)
	}
	var item salesmodels.SalesOrderItem
	if err := targetDB.First(&item, "id = ?", expected.item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.SalesOrderID != expected.order.ID || item.ModelID == nil ||
		expected.item.ModelID == nil || *item.ModelID != *expected.item.ModelID ||
		item.ModelNameSnapshot != "Dragon" || item.Quantity != 2 || item.UnitPriceCents != 1299 {
		t.Fatalf("Sales Order Item changed: %#v", item)
	}
	var allocation salesmodels.SalesOrderAllocation
	if err := targetDB.First(&allocation, "id = ?", expected.allocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.SalesOrderItemID != expected.item.ID || allocation.PrintJobID == nil ||
		expected.allocation.PrintJobID == nil || *allocation.PrintJobID != *expected.allocation.PrintJobID ||
		allocation.UnitsAllocated != 2 || allocation.ProductionCostCents == nil ||
		*allocation.ProductionCostCents != 630 {
		t.Fatalf("Sales Order Allocation changed: %#v", allocation)
	}
	var features models.AppConfig
	if err := targetDB.First(&features, "key = ?", models.AppFeaturesConfigKey).Error; err != nil {
		t.Fatal(err)
	}
	if features.Value != `{"sales_orders":false}` || features.Type != "json" {
		t.Fatalf("feature settings changed: %#v", features)
	}

	result = performRequest(t, target, http.MethodGet, "/sales-orders/"+expected.order.ID.String(), nil)
	if result.Code != http.StatusOK {
		t.Fatalf("restored completed order: %d %s", result.Code, result.Body.String())
	}
	var restored salesdto.SalesOrderDTO
	decodeBody(t, result, &restored)
	assertProfitabilityCents(t, restored.Profitability, 2598, 880, 630, 1510, 1088, 41.88)
}

func TestSchemaFourBackupWithoutSnapshotColumnsRemainsCompatible(t *testing.T) {
	source, sourceDB := setupDesktopTestRouter(t)
	expected := createSalesBackupFixture(t, source, sourceDB)
	sourceSQLDB, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sourceSQLDB.SetMaxOpenConns(1)
	legacySchemaStatements := []string{
		`PRAGMA foreign_keys = OFF`,
		`CREATE TABLE sales_orders_legacy (
			id uuid PRIMARY KEY,
			reference text NOT NULL,
			order_date datetime NOT NULL,
			sales_channel_id uuid,
			fulfilment_status text NOT NULL DEFAULT 'draft',
			payment_status text NOT NULL DEFAULT 'unpaid',
			packaging_cost_cents integer NOT NULL DEFAULT 0,
			shipping_cost_cents integer NOT NULL DEFAULT 0,
			platform_fee_cents integer NOT NULL DEFAULT 0,
			other_cost_cents integer NOT NULL DEFAULT 0,
			notes text NOT NULL DEFAULT '',
			created_at datetime,
			updated_at datetime
		)`,
		`INSERT INTO sales_orders_legacy (
			id, reference, order_date, sales_channel_id, fulfilment_status, payment_status,
			packaging_cost_cents, shipping_cost_cents, platform_fee_cents, other_cost_cents,
			notes, created_at, updated_at
		) SELECT
			id, reference, order_date, sales_channel_id, fulfilment_status, payment_status,
			packaging_cost_cents, shipping_cost_cents, platform_fee_cents, other_cost_cents,
			notes, created_at, updated_at
		FROM sales_orders`,
		`CREATE TABLE sales_order_allocations_legacy (
			id uuid PRIMARY KEY,
			sales_order_item_id uuid NOT NULL,
			print_job_id uuid,
			units_allocated integer NOT NULL,
			created_at datetime,
			updated_at datetime
		)`,
		`INSERT INTO sales_order_allocations_legacy (
			id, sales_order_item_id, print_job_id, units_allocated, created_at, updated_at
		) SELECT
			id, sales_order_item_id, print_job_id, units_allocated, created_at, updated_at
			FROM sales_order_allocations`,
		`DROP TABLE sales_order_allocations`,
		`DROP TABLE sales_orders`,
		`ALTER TABLE sales_orders_legacy RENAME TO sales_orders`,
		`ALTER TABLE sales_order_allocations_legacy RENAME TO sales_order_allocations`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, statement := range legacySchemaStatements {
		if err := sourceDB.Exec(statement).Error; err != nil {
			t.Fatalf("rebuild legacy schema-4 tables: %v", err)
		}
	}

	backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
	if backup.Code != http.StatusOK {
		t.Fatalf("export legacy schema 4: %d %s", backup.Code, backup.Body.String())
	}
	target, targetDB := setupDesktopTestRouter(t)
	result := performMultipartFileRequest(t, target, http.MethodPost,
		"/desktop-data/import", "file", "schema-4-without-snapshots.zip", backup.Body.Bytes())
	if result.Code != http.StatusOK {
		t.Fatalf("legacy schema-4 import: %d %s", result.Code, result.Body.String())
	}

	var order salesmodels.SalesOrder
	if err := targetDB.First(&order, "id = ?", expected.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.ProductionCostCents != nil || order.TotalCostCents != nil ||
		order.NetProfitCents != nil || order.MarginBasisPoints != nil {
		t.Fatalf("legacy schema-4 import invented order snapshots: %#v", order)
	}
	var allocation salesmodels.SalesOrderAllocation
	if err := targetDB.First(&allocation, "id = ?", expected.allocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.ProductionCostCents != nil {
		t.Fatalf("legacy schema-4 import invented allocation snapshot: %#v", allocation)
	}
}

func TestPreviousSchemaBackupImportsWithEmptySalesData(t *testing.T) {
	source, sourceDB := setupDesktopTestRouter(t)
	filament := models.Filament{Brand: "Legacy", Type: "PLA", Color: "Blue", AvgPrice: 18}
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
	legacyBackup := rewriteBackupSchemaVersion(t, backup.Body.Bytes(), 3)

	target, targetDB := setupDesktopTestRouter(t)
	if err := targetDB.Create(&salesmodels.SalesChannel{Name: "Existing Sales data"}).Error; err != nil {
		t.Fatal(err)
	}
	result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "schema-3.zip", legacyBackup)
	if result.Code != http.StatusOK {
		t.Fatalf("schema-3 import: %d %s", result.Code, result.Body.String())
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
			t.Fatalf("schema-3 restore left %d Sales %s", count, name)
		}
	}
	var restored models.Filament
	if err := targetDB.First(&restored, "id = ?", filament.ID).Error; err != nil {
		t.Fatalf("schema-3 restore lost production data: %v", err)
	}
}

func TestSalesBackupRejectsInvalidRelationshipsWithoutReplacingTarget(t *testing.T) {
	tests := []struct {
		name       string
		expected   string
		invalidate func(*gorm.DB, salesBackupFixture) error
	}{
		{
			name: "order channel", expected: "missing Sales Channel",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrder{}).Where("id = ?", fixture.order.ID).
					UpdateColumn("sales_channel_id", uuid.New()).Error
			},
		},
		{
			name: "item order", expected: "missing Sales Order",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrderItem{}).Where("id = ?", fixture.item.ID).
					UpdateColumn("sales_order_id", uuid.New()).Error
			},
		},
		{
			name: "item model", expected: "missing Model",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrderItem{}).Where("id = ?", fixture.item.ID).
					UpdateColumn("model_id", uuid.New()).Error
			},
		},
		{
			name: "allocation item", expected: "missing Sales Order Item",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrderAllocation{}).Where("id = ?", fixture.allocation.ID).
					UpdateColumn("sales_order_item_id", uuid.New()).Error
			},
		},
		{
			name: "allocation print job", expected: "missing Print Job",
			invalidate: func(db *gorm.DB, fixture salesBackupFixture) error {
				return db.Model(&salesmodels.SalesOrderAllocation{}).Where("id = ?", fixture.allocation.ID).
					UpdateColumn("print_job_id", uuid.New()).Error
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, sourceDB := setupDesktopTestRouter(t)
			fixture := createSalesBackupFixture(t, source, sourceDB)
			if err := test.invalidate(sourceDB, fixture); err != nil {
				t.Fatal(err)
			}
			backup := performRequest(t, source, http.MethodGet, "/desktop-data/export", nil)
			if backup.Code != http.StatusOK {
				t.Fatalf("export: %d %s", backup.Code, backup.Body.String())
			}

			target, targetDB := setupDesktopTestRouter(t)
			kept := salesmodels.SalesChannel{Name: "Keep this Sales data"}
			if err := targetDB.Create(&kept).Error; err != nil {
				t.Fatal(err)
			}
			result := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "invalid-sales.zip", backup.Body.Bytes())
			if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), test.expected) {
				t.Fatalf("invalid relationship response: %d %s", result.Code, result.Body.String())
			}
			var restored salesmodels.SalesChannel
			if err := targetDB.First(&restored, "id = ?", kept.ID).Error; err != nil || restored.Name != kept.Name {
				t.Fatalf("invalid import replaced target data: %#v %v", restored, err)
			}
		})
	}
}

func readBackupFiles(t *testing.T, archive []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(reader.File))
	for _, entry := range reader.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		files[entry.Name] = content
	}
	return files
}

func rewriteBackupSchemaVersion(t *testing.T, archive []byte, schemaVersion int) []byte {
	t.Helper()
	files := readBackupFiles(t, archive)
	var manifest map[string]any
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["schema_version"] = schemaVersion
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = manifestJSON
	return buildZipArchive(t, files)
}
