package handlers_test

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const adjustments = "/v2/inventory/adjustments"

func posting(typ string, id uuid.UUID, qty int64) inventory.Posting {
	return inventory.Posting{OperationID: uuid.New(), SourceID: uuid.New(), ItemType: typ, ItemID: id, Kind: "adjustment", Quantity: qty, Actor: "operator", Reason: "physical count"}
}
func TestInventoryHistoryAndRestore(t *testing.T) {
	r, db := setupTestRouter(t)
	item := models.Filament{TimesPurchased: 42, TimesUsed: 17}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	path := "/v2/inventory/balances/filament/" + item.ID.String()
	empty := performRequest(t, r, "GET", path, nil)
	var b inventory.Balance
	decodeBody(t, empty, &b)
	if empty.Code != 200 || b != (inventory.Balance{}) {
		t.Fatal(empty.Body.String())
	}
	opening := posting("filament", item.ID, 1001)
	opening.Kind = "opening"
	first := performRequest(t, r, "POST", adjustments, opening)
	if first.Code != 201 {
		t.Fatal(first.Body.String())
	}
	change := posting("filament", item.ID, -1002)
	res := performRequest(t, r, "POST", adjustments, change)
	var receipt inventory.Receipt
	decodeBody(t, res, &receipt)
	if res.Code != 201 || receipt.Posting != change || receipt.CreatedAt.IsZero() || receipt.Balance.OnHand != -1 || !receipt.Balance.Negative || !receipt.Balance.Verified {
		t.Fatal(res.Body.String())
	}
	check := performRequest(t, r, "GET", path, nil)
	decodeBody(t, check, &b)
	if b != receipt.Balance {
		t.Fatalf("rebuilt balance %+v, receipt %+v", b, receipt)
	}
	var history []inventory.Movement
	audit := performRequest(t, r, "GET", "/v2/inventory/movements/filament/"+item.ID.String()+"?after="+strconv.FormatInt(receipt.Sequence-1, 10), nil)
	decodeBody(t, audit, &history)
	if audit.Code != 200 || len(history) != 1 || history[0].Posting != change || !history[0].CreatedAt.Equal(receipt.CreatedAt) {
		t.Fatal(audit.Body.String())
	}
	duplicate := change
	duplicate.OperationID = uuid.New()
	changed := change
	changed.Quantity = 4
	for _, bad := range []inventory.Posting{duplicate, changed} {
		if res := performRequest(t, r, "POST", adjustments, bad); res.Code != 409 {
			t.Fatal(res.Body.String())
		}
	}
	invalid := change
	invalid.Reason = " "
	if res := performRequest(t, r, "POST", adjustments, invalid); res.Code != 400 {
		t.Fatal(res.Body.String())
	}
	if res := performRequest(t, r, "DELETE", "/filaments/"+item.ID.String(), nil); res.Code != 409 {
		t.Fatal(res.Body.String())
	}
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	target, _ := setupTestRouter(t)
	for i := 0; i < 2; i++ {
		restore := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "ledger.zip", archive.Body.Bytes())
		if restore.Code != 200 {
			t.Fatal(restore.Body.String())
		}
		replay := performRequest(t, target, "POST", adjustments, opening)
		if replay.Code != 201 || replay.Body.String() != first.Body.String() {
			t.Fatalf("replay %s, original %s", replay.Body.String(), first.Body.String())
		}
		if after := performRequest(t, target, "GET", path, nil); after.Body.String() != check.Body.String() {
			t.Fatal(after.Body.String())
		}
	}
	if err := db.Exec("UPDATE inventory_movements SET actor = ''").Error; err != nil {
		t.Fatal(err)
	}
	corrupt := performRequest(t, r, "GET", "/desktop-data/export", nil)
	rejected := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", corrupt.Body.Bytes())
	if rejected.Code != 400 {
		t.Fatal(rejected.Body.String())
	}
	if after := performRequest(t, target, "GET", path, nil); after.Body.String() != check.Body.String() {
		t.Fatal(after.Body.String())
	}
	// A real pre-ledger schema drops the table; restored counters remain unverified.
	if err := db.Migrator().DropTable(&inventory.Movement{}); err != nil {
		t.Fatal(err)
	}
	old := performRequest(t, r, "GET", "/desktop-data/export", nil)
	legacy := rewriteBackupSchemaVersion(t, old.Body.Bytes(), 6)
	restore := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "legacy.zip", legacy)
	if restore.Code != 200 {
		t.Fatal(restore.Body.String())
	}
	if after := performRequest(t, target, "GET", path, nil); after.Body.String() != empty.Body.String() {
		t.Fatal(after.Body.String())
	}
}

func TestInventoryConcurrentPostAndOuterRollback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = db.AutoMigrate(&models.Model{}, &models.ModelMaterialRequirement{}, &inventory.Movement{}); err != nil {
		t.Fatal(err)
	}
	model := models.Model{Name: "part"}
	if err = db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	p := posting("model", model.ID, 3)
	start := make(chan struct{})
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			results <- db.Transaction(func(tx *gorm.DB) error { _, err := inventory.Post(tx, p); return err })
		}()
	}
	close(start)
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	sentinel := errors.New("source transaction failed")
	err = db.Transaction(func(tx *gorm.DB) error {
		_, err := inventory.Post(tx, posting("model", model.ID, 9))
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	b, err := inventory.Availability(db, "model", model.ID)
	if err != nil || b.OnHand != 3 || b.Available != 3 || b.Reserved != 0 || b.Verified {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestInventoryValidation(t *testing.T) {
	r, db := setupTestRouter(t)
	m := models.Model{Name: "part"}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		edit func(*inventory.Posting)
		code int
	}{
		{func(p *inventory.Posting) { p.ItemID = uuid.New() }, 404},
		{func(p *inventory.Posting) { p.ItemType = "order" }, 400},
		{func(p *inventory.Posting) { p.Actor = "" }, 400},
		{func(p *inventory.Posting) { p.Quantity = 0 }, 400},
		{func(p *inventory.Posting) { p.Quantity = inventory.MaxQuantity + 1 }, 400},
		{func(p *inventory.Posting) { p.Kind = "opening"; p.Quantity = -1 }, 400},
	} {
		p := posting("model", m.ID, 1)
		tc.edit(&p)
		if res := performRequest(t, r, http.MethodPost, adjustments, p); res.Code != tc.code {
			t.Fatal(res.Body.String())
		}
	}
}

func TestPhysicalCountAfterRecordedMovements(t *testing.T) {
	r, db := setupTestRouter(t)
	item := models.Filament{}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	initial := performRequest(t, r, "POST", adjustments, posting("filament", item.ID, 1000))
	var prior inventory.Receipt
	decodeBody(t, initial, &prior)
	if initial.Code != 201 || prior.Balance.Verified {
		t.Fatal(initial.Body.String())
	}
	req := map[string]any{"kind": "count", "operation_id": uuid.New(), "source_id": uuid.New(),
		"item_type": "filament", "item_id": item.ID, "actor": "operator", "reason": "empty shelf"}
	for _, tc := range []struct {
		revision, counted any
		status            int
	}{
		{nil, 0, 400}, {prior.Sequence, nil, 400}, {prior.Sequence, -1, 400}, {0, 0, 409},
	} {
		req["expected_revision"], req["counted_quantity"] = tc.revision, tc.counted
		if res := performRequest(t, r, "POST", adjustments, req); res.Code != tc.status {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	req["expected_revision"], req["counted_quantity"] = prior.Sequence, 0
	first := performRequest(t, r, "POST", adjustments, req)
	var counted inventory.Receipt
	decodeBody(t, first, &counted)
	if first.Code != 201 || counted.Quantity != -1000 || counted.Balance.OnHand != 0 || !counted.Balance.Verified {
		t.Fatal(first.Body.String())
	}
	if res := performRequest(t, r, "POST", adjustments, posting("filament", item.ID, 25)); res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	target, targetDB := setupTestRouter(t)
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	for i := 0; i < 2; i++ {
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "count.zip", archive.Body.Bytes())
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		retry := performRequest(t, target, "POST", adjustments, req)
		if retry.Code != 201 || retry.Body.String() != first.Body.String() {
			t.Fatal(retry.Body.String())
		}
	}
	req["counted_quantity"] = 10
	if res := performRequest(t, target, "POST", adjustments, req); res.Code != 409 {
		t.Fatal(res.Body.String())
	}
	if err := db.Model(&inventory.Movement{}).Where("kind = ?", "count").Update("counted_quantity", 1).Error; err != nil {
		t.Fatal(err)
	}
	corrupt := performRequest(t, r, "GET", "/desktop-data/export", nil)
	res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", corrupt.Body.Bytes())
	if res.Code != 400 {
		t.Fatal(res.Body.String())
	}
	b, err := inventory.Availability(targetDB, "filament", item.ID)
	if err != nil || !b.Verified || b.OnHand != 25 {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestSchemaEightUpgradeAndRestore(t *testing.T) {
	r, db := setupTestRouter(t)
	item := models.Filament{}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	old := posting("filament", item.ID, 500)
	original := performRequest(t, r, "POST", adjustments, old)
	if original.Code != 201 {
		t.Fatal(original.Body.String())
	}
	for _, column := range []string{"counted_quantity", "expected_revision"} {
		if err := db.Migrator().DropColumn(&inventory.Movement{}, column); err != nil {
			t.Fatal(err)
		}
	}
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	legacy := rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 8)
	target, restored := setupTestRouter(t)
	for i := 0; i < 2; i++ {
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "schema8.zip", legacy)
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		assertStock(t, restored, item.ID, 500)
		if err := db.AutoMigrate(&inventory.Movement{}); err != nil {
			t.Fatal(err)
		}
		for _, router := range []*gin.Engine{r, target} {
			replay := performRequest(t, router, "POST", adjustments, old)
			if replay.Code != 201 || replay.Body.String() != original.Body.String() {
				t.Fatal(replay.Body.String())
			}
		}
	}
}

func TestInventoryThresholdsAndMaterialProjection(t *testing.T) {
	r, db := setupTestRouter(t)
	item := models.Filament{Brand: "Test", TimesUsed: 99, TimesPurchased: 40}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	path := "/v2/inventory/thresholds/" + item.ID.String()
	for _, input := range []any{map[string]any{}, map[string]any{"threshold_mg": -1}, map[string]any{"threshold_mg": 1.5}, map[string]any{"threshold_mg": 9007199254740992}} {
		if res := performRequest(t, r, "PUT", path, input); res.Code != 400 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	if res := performRequest(t, r, "PUT", path, map[string]any{"threshold_mg": 5000}); res.Code != 204 {
		t.Fatal(res.Body.String())
	}
	var rows []struct {
		Threshold int64             `json:"threshold_mg"`
		Balance   inventory.Balance `json:"balance"`
	}
	res := performRequest(t, r, "GET", "/v2/inventory/materials", nil)
	decodeBody(t, res, &rows)
	if res.Code != 200 || len(rows) != 1 || rows[0].Threshold != 5000 || rows[0].Balance.Verified || rows[0].Balance.OnHand != 0 {
		t.Fatal(res.Body.String())
	}
	opening := posting("filament", item.ID, 4000)
	opening.Kind = "opening"
	if res := performRequest(t, r, "POST", adjustments, opening); res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	low := performRequest(t, r, "GET", "/filaments?page=1&page_size=20&low_stock=true", nil)
	if low.Code != 200 || !strings.Contains(low.Body.String(), item.ID.String()) {
		t.Fatal(low.Body.String())
	}
}

func TestInventoryThresholdBackupRoundTrip(t *testing.T) {
	source, db := setupDesktopTestRouter(t)
	item := models.Filament{Brand: "Threshold fixture", StockThresholdMG: 123456}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	archive := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	target, restored := setupDesktopTestRouter(t)
	response := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "inventory.zip", archive.Body.Bytes())
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var got models.Filament
	if err := restored.First(&got, "id = ?", item.ID).Error; err != nil || got.StockThresholdMG != 123456 {
		t.Fatal(err, got)
	}
}

func TestFilamentLifecycleAndReconciliation(t *testing.T) {
	for _, business := range []bool{true, false} {
		t.Run("business="+strconv.FormatBool(business), func(t *testing.T) {
			router, db := setupTestRouter(t)
			item := models.Filament{Brand: "Workshop", StockThresholdMG: 1000000}
			if err := db.Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			id := item.ID.String()
			resource := "/v2/inventory/materials/" + id
			request := func(method, path string, body any, status int) string {
				t.Helper()
				res := performRequest(t, router, method, path, body)
				if res.Code != status {
					t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
				}
				return res.Body.String()
			}
			balance := func() inventory.Balance {
				t.Helper()
				b, err := inventory.Availability(db, "filament", item.ID)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			request("PUT", "/features", map[string]bool{"sales_orders": business}, 200)
			opening := posting("filament", item.ID, 0)
			opening.Kind = "opening"
			request("POST", adjustments, opening, 201)
			purchase := map[string]any{"filament_id": id, "amount": 1, "price": 21, "purchase_date": "2026-11-19T00:00:00Z"}
			create := func() string {
				t.Helper()
				p := legacyOrderRequest[struct {
					ID string `json:"id"`
				}](t, router, "POST", "/filament-purchases", purchase, 201)
				return "/filament-purchases/" + p.ID
			}
			posted := create()
			filter := "/filaments?page=1&low_stock=true"
			if !strings.Contains(request("GET", filter, nil, 200), id) {
				t.Fatal("missing low-stock warning")
			}
			request("PATCH", resource, map[string]bool{"warnings_enabled": false}, 204)
			if strings.Contains(request("GET", filter, nil, 200), id) || !balance().Verified {
				t.Fatal("warning control changed stock or failed to mute filter")
			}
			pause := map[string]any{"tracking_enabled": false, "expected_revision": balance().Revision, "archived": true}
			// Inject failure after the ledger transition. Neither setting may commit.
			if err := db.Exec("CREATE TRIGGER fail_archive BEFORE UPDATE OF archived ON filaments BEGIN SELECT RAISE(ABORT,'injected'); END").Error; err != nil {
				t.Fatal(err)
			}
			request("PATCH", resource, pause, 500)
			if balance().TrackingPaused || !balance().Verified {
				t.Fatal(balance())
			}
			if err := db.Exec("DROP TRIGGER fail_archive").Error; err != nil {
				t.Fatal(err)
			}
			request("PATCH", resource, pause, 204)
			paused := balance()
			request("PATCH", resource, pause, 204)
			if balance() != paused || paused.Verified || !paused.TrackingPaused {
				t.Fatal(paused)
			}
			request("POST", adjustments, posting("filament", item.ID, 10), 409)
			skipped := create()
			purchase["amount"] = 2
			request("PUT", posted, purchase, 200)
			purchase["amount"] = 1.75
			request("PUT", posted, purchase, 200)
			request("PUT", posted, purchase, 200)
			if balance().OnHand != 750000 {
				t.Fatal(balance())
			}
			archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
			if archive.Code != 200 {
				t.Fatal(archive.Body.String())
			}
			router, db = setupTestRouter(t)
			res := performMultipartFileRequest(t, router, "POST", "/desktop-data/import", "file", "stock.zip", archive.Body.Bytes())
			if res.Code != 200 {
				t.Fatal(res.Body.String())
			}
			var restored models.Filament
			if err := db.First(&restored, "id = ?", id).Error; err != nil || !restored.WarningsDisabled || !restored.Archived || !balance().TrackingPaused {
				t.Fatal(restored, err)
			}
			request("PATCH", resource, map[string]any{"tracking_enabled": true, "expected_revision": balance().Revision}, 204)
			request("PATCH", resource, pause, 409)
			if balance().Verified {
				t.Fatal("resumed without a count")
			}
			request("PUT", skipped, purchase, 200)
			request("PUT", posted, purchase, 200)
			if balance().OnHand != 750000 {
				t.Fatal(balance())
			}
			count := map[string]any{"item_type": "filament", "item_id": id, "kind": "count", "counted_quantity": 250000, "expected_revision": paused.Revision, "reason": "shelf count", "actor": "operator", "operation_id": uuid.New(), "source_id": uuid.New()}
			request("POST", adjustments, count, 409)
			count["expected_revision"] = balance().Revision
			request("POST", adjustments, count, 201)
			second := models.Filament{}
			if err := db.Create(&second).Error; err != nil {
				t.Fatal(err)
			}
			purchase["filament_id"] = second.ID
			request("PUT", posted, purchase, 200)
			assertStock(t, db, second.ID, 0)
			request("DELETE", posted, nil, 204)
			request("DELETE", skipped, nil, 204)
			if b := balance(); !b.Verified || b.OnHand != 250000 {
				t.Fatal(b)
			}
			request("PATCH", resource, map[string]bool{"warnings_enabled": true, "archived": false}, 204)
			if !strings.Contains(request("GET", filter, nil, 200), id) {
				t.Fatal("missing resumed warning")
			}
		})
	}
}

func TestSchema20FilamentLifecycleDefaults(t *testing.T) {
	source, db := setupTestRouter(t)
	item := models.Filament{Brand: "Existing", AvgPrice: 21.25, StockThresholdMG: 5000}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	p := posting("filament", item.ID, 4500)
	p.Kind = "opening"
	original := performRequest(t, source, "POST", adjustments, p)
	if original.Code != 201 {
		t.Fatal(original.Body.String())
	}
	for _, ddl := range []string{"ALTER TABLE filaments DROP COLUMN archived", "ALTER TABLE filaments DROP COLUMN warnings_disabled"} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	archive := performRequest(t, source, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	target, restored := setupTestRouter(t)
	importFile := func(zip []byte, status int) {
		t.Helper()
		res := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "legacy.zip", zip)
		if res.Code != status {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	importFile(archive.Body.Bytes(), 400)
	legacy := rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 20)
	for i := 0; i < 2; i++ {
		importFile(legacy, 200)
		var got models.Filament
		if err := restored.First(&got, "id = ?", item.ID).Error; err != nil || got.Archived || got.WarningsDisabled || got.AvgPrice != item.AvgPrice || got.StockThresholdMG != item.StockThresholdMG {
			t.Fatal(got, err)
		}
		if res := performRequest(t, target, "POST", adjustments, p); res.Code != 201 || res.Body.String() != original.Body.String() {
			t.Fatal(res.Body.String())
		}
	}
}
