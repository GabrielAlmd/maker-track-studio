package handlers_test

import (
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

func TestPurchaseSupplierHistoryAndLegacyUpdates(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	filamentBody := apiResponse{"brand": "Example", "type": "PLA", "color": "Blue", "avg_price": 20}
	filamentBody["product_url"] = " https://catalog.example.test/blue "
	created := performRequest(t, router, "POST", "/filaments", filamentBody)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var filament dto.FilamentDTO
	decodeBody(t, created, &filament)
	if filament.ProductURL != "https://catalog.example.test/blue" {
		t.Fatal(filament.ProductURL)
	}

	for _, field := range []string{"javascript:alert(1)", "//shop.example.test", "https:///missing-host", "https://[not-an-ip]", "https://user@shop.example.test", "https://shop.example.test:65536", "https://shop.example.test\\@other.example.test", "https://shop.example.test/" + strings.Repeat("x", 2048)} {
		filamentBody["product_url"] = field
		if bad := performRequest(t, router, "PUT", "/filaments/"+filament.ID.String(), filamentBody); bad.Code != 400 {
			t.Fatalf("invalid URL accepted: %s", bad.Body.String())
		}
	}
	delete(filamentBody, "product_url")
	if updated := performRequest(t, router, "PUT", "/filaments/"+filament.ID.String(), filamentBody); updated.Code != 200 || !strings.Contains(updated.Body.String(), filament.ProductURL) {
		t.Fatal(updated.Body.String())
	}

	purchaseBody := apiResponse{"filament_id": filament.ID, "purchase_date": "2026-11-20T00:00:00Z", "price": 20, "amount": 1}
	purchaseBody["supplier"], purchaseBody["supplier_reference"], purchaseBody["purchase_url"] = " Store A ", " A-1 ", " https://store-a.example.test/blue "
	first := performRequest(t, router, "POST", "/filament-purchases", purchaseBody)
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	var purchase dto.FilamentPurchaseDTO
	decodeBody(t, first, &purchase)
	if purchase.Supplier != "Store A" || purchase.SupplierReference != "A-1" || purchase.PurchaseURL != "https://store-a.example.test/blue" {
		t.Fatal(first.Body.String())
	}

	purchaseBody["supplier"], purchaseBody["supplier_reference"], purchaseBody["purchase_url"] = "Store B", "B-1", "http://store-b.example.test/blue"
	second := performRequest(t, router, "POST", "/filament-purchases", purchaseBody)
	if second.Code != 201 {
		t.Fatal(second.Body.String())
	}
	filamentBody["product_url"] = ""
	if cleared := performRequest(t, router, "PUT", "/filaments/"+filament.ID.String(), filamentBody); cleared.Code != 200 || strings.Contains(cleared.Body.String(), "product_url") {
		t.Fatal(cleared.Body.String())
	}

	path := "/filament-purchases/" + purchase.ID.String()
	purchaseBody["purchase_url"] = "file:///tmp/spool"
	if invalid := performRequest(t, router, "PUT", path, purchaseBody); invalid.Code != 400 {
		t.Fatal(invalid.Body.String())
	}
	for _, key := range []string{"supplier", "supplier_reference", "purchase_url"} {
		delete(purchaseBody, key)
	}
	updated := performRequest(t, router, "PUT", path, purchaseBody)
	if updated.Code != 200 || !strings.Contains(updated.Body.String(), purchase.PurchaseURL) || !strings.Contains(updated.Body.String(), purchase.Supplier) {
		t.Fatal(updated.Body.String())
	}
	purchaseBody["supplier_reference"] = ""
	if cleared := performRequest(t, router, "PUT", path, purchaseBody); cleared.Code != 200 || strings.Contains(cleared.Body.String(), "supplier_reference") {
		t.Fatal(cleared.Body.String())
	}
	history := performRequest(t, router, "GET", "/filament-purchases?page=1", nil)
	if history.Code != 200 || !strings.Contains(history.Body.String(), "Store A") || !strings.Contains(history.Body.String(), "Store B") {
		t.Fatal(history.Body.String())
	}
	assertStock(t, db, filament.ID, 2000000)
}

func TestPurchaseSupplierBatchRollbackAndBackupVersions(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	batch := func(items ...apiResponse) apiResponse {
		return apiResponse{"purchase_date": "2026-11-20T00:00:00Z", "items": items}
	}
	newLine := apiResponse{"new_filament": apiResponse{"brand": "Example", "type": "PLA", "color": "Blue", "product_url": "https://catalog.example.test/blue"}, "price": 21.50, "amount": 0.5, "supplier": "Supplier", "supplier_reference": "SKU", "purchase_url": "https://shop.example.test/blue"}
	if res := performRequest(t, router, "POST", "/filament-purchases/batch", batch(newLine, newLine)); res.Code != 409 {
		t.Fatal(res.Code, res.Body.String())
	}
	for _, entity := range []any{&models.Filament{}, &models.FilamentPurchase{}, &inventory.Movement{}} {
		var count int64
		if err := db.Model(entity).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("batch leaked %T: %d %v", entity, count, err)
		}
	}
	res := performRequest(t, router, "POST", "/filament-purchases/batch", batch(newLine))
	if res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	var purchases []dto.FilamentPurchaseDTO
	decodeBody(t, res, &purchases)
	if len(purchases) != 1 || purchases[0].SupplierReference != "SKU" {
		t.Fatal(res.Body.String())
	}
	purchase := purchases[0]
	materials := performRequest(t, router, "GET", "/v2/inventory/materials?page=1", nil)
	if materials.Code != 200 || !strings.Contains(materials.Body.String(), "https://catalog.example.test/blue") {
		t.Fatal(materials.Body.String())
	}

	report := performRequest(t, router, "GET", "/reports/export?mode=full", nil)
	if report.Code != 200 {
		t.Fatal(report.Body.String())
	}
	entries := reportEntries(t, report.Body.Bytes())
	if !strings.Contains(string(entries["filaments.csv"]), "https://catalog.example.test/blue") ||
		!strings.Contains(string(entries["filament_purchases.csv"]), "Supplier,SKU,https://shop.example.test/blue") {
		t.Fatal("supplier fields missing from CSV")
	}

	target, restored := setupDesktopTestRouter(t)
	restore := func(archive []byte, status int) {
		t.Helper()
		response := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "workspace.zip", archive)
		if response.Code != status {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	restore(archive.Body.Bytes(), 200)
	var got models.FilamentPurchase
	if err := restored.First(&got, "id = ?", purchase.ID).Error; err != nil || got.Supplier != purchase.Supplier || got.SupplierReference != purchase.SupplierReference || got.PurchaseURL != purchase.PurchaseURL {
		t.Fatal(err, got)
	}
	for _, ddl := range []string{
		"ALTER TABLE filaments DROP COLUMN product_url",
		"ALTER TABLE filament_purchases DROP COLUMN supplier",
		"ALTER TABLE filament_purchases DROP COLUMN supplier_reference",
		"ALTER TABLE filament_purchases DROP COLUMN purchase_url",
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	legacy := performRequest(t, router, "GET", "/desktop-data/export", nil)
	if legacy.Code != 200 {
		t.Fatal(legacy.Body.String())
	}
	restore(legacy.Body.Bytes(), 400)
	restore(rewriteBackupSchemaVersion(t, legacy.Body.Bytes(), 22), 200)
	if err := restored.First(&got, "id = ?", purchase.ID).Error; err != nil || got.Supplier != "" || got.PurchaseURL != "" || got.Price != 21.50 || got.Amount != 0.5 {
		t.Fatal(err, got)
	}
	assertStock(t, restored, purchase.FilamentID, 500000)
}

func TestMigrationPreservesUnmappedPurchaseSupplierDetails(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	filament := models.Filament{Brand: "Example", Type: "PLA", Color: "Blue"}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	plan := migrationPlan(map[string]any{"section": "purchases", "source": "purchases", "duplicate_strategy": "update"})
	header := "filament,price,amount,purchase_date"
	record := "Example | PLA | Blue,19.50,1,2026-11-20"
	submit := func(csv string, status int) {
		t.Helper()
		res := performMigrationRequest(t, router, "/data-migration/import", "purchases.csv", []byte(csv), plan)
		if res.Code != status {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	submit(header+",supplier,supplier_reference,purchase_url\n"+record+",Store,SKU,https://shop.example.test/blue\n", 201)
	submit(header+",purchase_url\n"+record+",javascript:alert(1)\n", 422)
	submit(header+"\n"+record+"\n", 201)
	var purchases []models.FilamentPurchase
	if err := db.Find(&purchases).Error; err != nil || len(purchases) != 1 {
		t.Fatal(err, purchases)
	}
	if purchase := purchases[0]; purchase.Supplier != "Store" || purchase.SupplierReference != "SKU" || purchase.PurchaseURL != "https://shop.example.test/blue" {
		t.Fatal(purchase)
	}
	assertStock(t, db, filament.ID, 0)
}
