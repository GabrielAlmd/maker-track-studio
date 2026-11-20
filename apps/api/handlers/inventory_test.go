package handlers_test

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
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
	if err = db.AutoMigrate(&models.Model{}, &inventory.Movement{}); err != nil {
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
