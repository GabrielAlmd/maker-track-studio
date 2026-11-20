package handlers_test

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"testing"
)

func assertStock(t *testing.T, db *gorm.DB, id uuid.UUID, quantity int64) {
	t.Helper()
	b, err := inventory.Availability(db, "filament", id)
	if err != nil || b.OnHand != quantity || b.Verified {
		t.Fatalf("stock: %+v, want %d unverified, err %v", b, quantity, err)
	}
}

func TestPurchaseStockCorrectionsRollbackAndRestore(t *testing.T) {
	r, db := setupTestRouter(t)
	a, b := models.Filament{}, models.Filament{}
	db.Create(&a)
	db.Create(&b)
	body := map[string]any{"filament_id": a.ID, "amount": 1.234567, "price": 20, "purchase_date": "2026-11-19T00:00:00Z"}
	res := performRequest(t, r, "POST", "/filament-purchases", body)
	if res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	var p map[string]any
	decodeBody(t, res, &p)
	path := "/filament-purchases/" + p["id"].(string)
	assertStock(t, db, a.ID, 1234567)
	body["amount"] = 0.25
	body["filament_id"] = b.ID
	if err := db.Exec("CREATE TRIGGER reject_stock BEFORE INSERT ON inventory_movements BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	if res := performRequest(t, r, "PUT", path, body); res.Code != 500 {
		t.Fatal(res.Body.String())
	}
	assertStock(t, db, a.ID, 1234567)
	assertStock(t, db, b.ID, 0)
	var stored models.FilamentPurchase
	db.First(&stored, "id = ?", p["id"])
	if stored.Amount != 1.234567 || stored.FilamentID != a.ID {
		t.Fatal("partial purchase correction")
	}
	db.Exec("DROP TRIGGER reject_stock")
	for i := 0; i < 2; i++ {
		if res := performRequest(t, r, "PUT", path, body); res.Code != 200 {
			t.Fatal(res.Body.String())
		}
	}
	assertStock(t, db, a.ID, 0)
	assertStock(t, db, b.ID, 250000)
	var count int64
	db.Model(&inventory.Movement{}).Count(&count)
	if count != 3 {
		t.Fatalf("duplicate movements: %d", count)
	}
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	target, restoredDB := setupTestRouter(t)
	restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "stock.zip", archive.Body.Bytes())
	if restored.Code != 200 {
		t.Fatal(restored.Body.String())
	}
	if res := performRequest(t, target, "PUT", path, body); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	assertStock(t, restoredDB, b.ID, 250000)
	for i := 0; i < 2; i++ {
		if res := performRequest(t, target, "DELETE", path, nil); res.Code != 204 {
			t.Fatal(res.Body.String())
		}
	}
	assertStock(t, restoredDB, b.ID, 0)
	// A legacy purchase has never contributed stock; editing/deleting it cannot replay history.
	legacy := models.FilamentPurchase{FilamentID: a.ID, Amount: 1, Price: 20}
	restoredDB.Create(&legacy)
	body["filament_id"] = a.ID
	if res := performRequest(t, target, "PUT", "/filament-purchases/"+legacy.ID.String(), body); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	performRequest(t, target, "DELETE", "/filament-purchases/"+legacy.ID.String(), nil)
	assertStock(t, restoredDB, a.ID, 0)
}

func TestProductionMaterialStock(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			r, db, printer := setupTestRouterWithDefaultPrinter(t)
			a, b, c := models.Filament{}, models.Filament{}, models.Filament{}
			db.Create(&a)
			db.Create(&b)
			db.Create(&c)
			input := map[string]any{"operation_id": uuid.NewString(), "expected_revision": 0, "printer_id": printer.ID, "print_date": "2026-11-19T00:00:00Z", "runs": []any{map[string]any{"status": "planned", "items": []any{map[string]any{"planned": 1}}, "materials": []any{}}}}
			res := performRequest(t, r, "POST", "/v2/production/jobs", input)
			if res.Code != 201 {
				t.Fatal(res.Body.String())
			}
			var data map[string]any
			decodeBody(t, res, &data)
			path := "/v2/production/jobs/" + data["id"].(string)
			for _, state := range []string{"in_progress", status} {
				input["operation_id"] = uuid.NewString()
				input["expected_revision"] = data["revision"]
				input["runs"] = data["runs"]
				run := input["runs"].([]any)[0].(map[string]any)
				run["status"] = state
				if state == status {
					item := run["items"].([]any)[0].(map[string]any)
					item["attempted"] = 1
					if status == "completed" {
						item["accepted"] = 1
					} else {
						item["rejected"] = 1
					}
					run["materials"] = []any{map[string]any{"filament_id": a.ID, "weight_used": 12.345, "waste_weight_g": 2, "measurement_state": "verified"}, map[string]any{"filament_id": b.ID, "weight_used": 3.001, "measurement_state": "verified"}, map[string]any{"filament_id": c.ID, "weight_used": 99, "estimated_weight_g": 99, "measurement_state": "estimated"}}
					db.Exec("CREATE TRIGGER fail_stock_receipt BEFORE INSERT ON production_operations BEGIN SELECT RAISE(ABORT, 'injected'); END")
					if failed := performRequest(t, r, "PUT", path, input); failed.Code != 500 {
						t.Fatal(failed.Body.String())
					}
					assertStock(t, db, a.ID, 0)
					assertStock(t, db, b.ID, 0)
					db.Exec("DROP TRIGGER fail_stock_receipt")
				}
				res = performRequest(t, r, "PUT", path, input)
				if res.Code != 200 {
					t.Fatal(res.Body.String())
				}
				decodeBody(t, res, &data)
			}
			assertStock(t, db, a.ID, -12345)
			assertStock(t, db, b.ID, -3001)
			assertStock(t, db, c.ID, 0)
			if retry := performRequest(t, r, "PUT", path, input); retry.Code != 200 || retry.Body.String() != res.Body.String() {
				t.Fatal(retry.Body.String())
			}
			// A fresh command retaining immutable terminal runs must also be a no-op for stock.
			input["operation_id"] = uuid.NewString()
			input["expected_revision"] = data["revision"]
			input["runs"] = data["runs"]
			if retry := performRequest(t, r, "PUT", path, input); retry.Code != 200 {
				t.Fatal(retry.Body.String())
			}
			assertStock(t, db, a.ID, -12345)
			var count int64
			db.Model(&inventory.Movement{}).Count(&count)
			if count != 2 {
				t.Fatalf("duplicate deductions %d", count)
			}

			archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
			target, restoredDB := setupTestRouter(t)
			restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "production-stock.zip", archive.Body.Bytes())
			if restored.Code != 200 {
				t.Fatal(restored.Body.String())
			}
			assertStock(t, restoredDB, a.ID, -12345)
			if retry := performRequest(t, target, "PUT", path, input); retry.Code != 200 {
				t.Fatal(retry.Body.String())
			}
			assertStock(t, restoredDB, b.ID, -3001)
		})
	}
}

func TestLowStockUsesVerifiedLedger(t *testing.T) {
	r, db := setupTestRouter(t)
	unknown, empty, positive := models.Filament{TimesPurchased: 1, TimesUsed: 99}, models.Filament{}, models.Filament{TimesPurchased: 1, TimesUsed: 99}
	db.Create(&unknown)
	db.Create(&empty)
	db.Create(&positive)
	for id, qty := range map[uuid.UUID]int64{empty.ID: 0, positive.ID: 10} {
		p := posting("filament", id, qty)
		p.Kind = "opening"
		if res := performRequest(t, r, "POST", adjustments, p); res.Code != 201 {
			t.Fatal(res.Body.String())
		}
	}
	res := performRequest(t, r, "GET", "/filaments?page=1&page_size=20&low_stock=true", nil)
	var page struct {
		Items []struct {
			ID uuid.UUID `json:"id"`
		} `json:"items"`
	}
	decodeBody(t, res, &page)
	if res.Code != 200 || len(page.Items) != 1 || page.Items[0].ID != empty.ID {
		t.Fatal(res.Body.String())
	}
}

func TestPurchaseBatchPostsStockAndRejectsUnrepresentableAmounts(t *testing.T) {
	r, db := setupTestRouter(t)
	f := models.Filament{}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	for _, amount := range []float64{0.0000001, 1e20} {
		res := performRequest(t, r, "POST", "/filament-purchases/batch", map[string]any{"purchase_date": "2026-11-19T00:00:00Z", "items": []any{map[string]any{"filament_id": f.ID, "amount": amount, "price": 20}}})
		if res.Code != 400 {
			t.Fatal(res.Body.String())
		}
	}
	res := performRequest(t, r, "POST", "/filament-purchases/batch", map[string]any{"purchase_date": "2026-11-19T00:00:00Z", "items": []any{map[string]any{"filament_id": f.ID, "amount": 0.5, "price": 20}, map[string]any{"filament_id": f.ID, "amount": 0.25, "price": 10}}})
	if res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	assertStock(t, db, f.ID, 750000)
}

func TestSchemaSevenInventoryBackupAndAdditiveUpgrade(t *testing.T) {
	r, db := setupTestRouter(t)
	f := models.Filament{}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	p := posting("filament", f.ID, 123)
	if res := performRequest(t, r, "POST", adjustments, p); res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	if err := db.Migrator().DropColumn(&inventory.Movement{}, "source_key"); err != nil {
		t.Fatal(err)
	}
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	target, targetDB := setupTestRouter(t)
	old := rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 7)
	restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "schema7.zip", old)
	if restored.Code != 200 {
		t.Fatal(restored.Body.String())
	}
	assertStock(t, targetDB, f.ID, 123)
	// Repeated additive migration leaves quantities and identities unchanged.
	for i := 0; i < 2; i++ {
		if err := db.AutoMigrate(&inventory.Movement{}); err != nil {
			t.Fatal(err)
		}
	}
	assertStock(t, db, f.ID, 123)
	replay := performRequest(t, r, "POST", adjustments, p)
	if replay.Code != 201 {
		t.Fatal(replay.Body.String())
	}
	assertStock(t, db, f.ID, 123)
}
