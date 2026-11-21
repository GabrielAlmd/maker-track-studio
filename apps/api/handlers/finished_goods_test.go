package handlers_test

import (
	"errors"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestFinishedGoodsFromProductionToFulfilment(t *testing.T) {
	r, db, printer := setupTestRouterWithDefaultPrinter(t)
	channel, product, other := createSalesOrderReferences(t, db)
	createSalesOrderForTest(t, r, salesOrderRequest(channel.ID, []map[string]any{{"model_id": product.ID, "quantity": 20, "unit_price_cents": 500}}))
	input := map[string]any{"operation_id": uuid.New(), "printer_id": printer.ID, "print_date": "2026-11-19T00:00:00Z", "runs": []any{map[string]any{"status": "planned", "items": []any{map[string]any{"planned": 12, "model_id": product.ID}, map[string]any{"planned": 1, "model_id": other.ID}}}}}
	res := performRequest(t, r, "POST", "/v2/production/jobs", input)
	if res.Code != 201 {
		t.Fatal(res.Body.String())
	}
	var job map[string]any
	decodeBody(t, res, &job)
	path := "/v2/production/jobs/" + job["id"].(string)
	var itemID uuid.UUID
	for _, status := range []string{"in_progress", "completed"} {
		input["operation_id"], input["expected_revision"], input["runs"] = uuid.New(), job["revision"], job["runs"]
		run := job["runs"].([]any)[0].(map[string]any)
		run["status"] = status
		for _, raw := range run["items"].([]any) {
			item := raw.(map[string]any)
			if item["model_id"] == product.ID.String() {
				itemID = uuid.MustParse(item["id"].(string))
				item["attempted"], item["accepted"], item["rejected"] = 12, 10, 2
			} else {
				item["attempted"], item["accepted"] = 1, 1
			}
		}
		empty := performRequest(t, r, "GET", "/v2/inventory/finished-goods", nil)
		if empty.Code != 200 || empty.Body.String() != "[]" {
			t.Fatalf("premature stock: %s", empty.Body.String())
		}
		if status == "completed" {
			if err := db.Exec("CREATE TRIGGER fail_output_receipt BEFORE INSERT ON production_operations BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
				t.Fatal(err)
			}
			if got := performRequest(t, r, "PUT", path, input); got.Code != 500 {
				t.Fatal(got.Body.String())
			}
			if got := performRequest(t, r, "GET", "/v2/inventory/finished-goods", nil); got.Body.String() != "[]" {
				t.Fatal("partial stock", got.Body.String())
			}
			if err := db.Exec("DROP TRIGGER fail_output_receipt").Error; err != nil {
				t.Fatal(err)
			}
		}
		res = performRequest(t, r, "PUT", path, input)
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		decodeBody(t, res, &job)
	}
	if got := performRequest(t, r, "PUT", path, input); got.Body.String() != res.Body.String() {
		t.Fatal("completion replay", got.Body.String())
	}
	var products []inventory.FinishedProduct
	decodeBody(t, performRequest(t, r, "GET", "/v2/inventory/finished-goods", nil), &products)
	if len(products) != 2 || products[0].ID != product.ID || products[0].OnHand != 10 || products[1].OnHand != 1 {
		t.Fatal(products)
	}
	apply := func(cmd inventory.FulfilmentCommand, want error) inventory.Receipt {
		t.Helper()
		var receipt inventory.Receipt
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			receipt, err = inventory.ApplyFulfilment(tx, cmd)
			return err
		})
		if !errors.Is(err, want) {
			t.Fatalf("%s: got %v want %v", cmd.Kind, err, want)
		}
		return receipt
	}
	command := func(kind string, ref uuid.UUID, units int64) inventory.FulfilmentCommand {
		t.Helper()
		b, err := inventory.OutputAvailability(db, product.ID, itemID)
		if err != nil || !b.Verified {
			t.Fatal(b, err)
		}
		return inventory.FulfilmentCommand{OperationID: uuid.New(), SourceID: uuid.New(), Reference: ref, RunItemID: itemID, ProductID: product.ID, Units: units, ExpectedRevision: b.Revision, Kind: kind, Actor: "test", Reason: "test"}
	}
	owner := uuid.New()
	apply(command("reserve", owner, 11), inventory.ErrConflict)
	hold := command("reserve", owner, 3)
	held := apply(hold, nil)
	if held.Balance.OnHand != 10 || held.Balance.Reserved != 3 || held.Balance.Available != 7 {
		t.Fatal(held)
	}
	correction := posting("model", product.ID, -8)
	if got := performRequest(t, r, "POST", adjustments, correction); got.Code != 409 {
		t.Fatal(got.Body.String())
	}
	stale := hold
	stale.OperationID, stale.SourceID = uuid.New(), uuid.New()
	apply(stale, inventory.ErrConflict)
	apply(command("dispatch", hold.OperationID, 4), inventory.ErrConflict)
	ship := command("dispatch", hold.OperationID, 3)
	shipped := apply(ship, nil)
	if shipped.Balance.OnHand != 7 || shipped.Balance.Reserved != 0 || shipped.Balance.Available != 7 {
		t.Fatal(shipped)
	}
	apply(command("release", hold.OperationID, 1), inventory.ErrConflict)
	apply(command("restock", ship.OperationID, 2), nil)
	apply(command("reject_return", ship.OperationID, 1), nil)
	apply(command("restock", ship.OperationID, 1), inventory.ErrConflict)
	extra := command("reserve", owner, 2)
	apply(extra, nil)
	apply(command("release", extra.OperationID, 1), nil)
	duplicate := ship
	duplicate.OperationID = uuid.New()
	apply(duplicate, inventory.ErrConflict)

	input["operation_id"], input["expected_revision"], input["runs"] = uuid.New(), job["revision"], job["runs"]
	item := job["runs"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	item["rejected"], item["accepted"] = item["attempted"], 0
	if got := performRequest(t, r, "PUT", path, input); got.Code != 409 {
		t.Fatal(got.Body.String())
	}
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	target, restored := setupTestRouter(t)
	if got := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "stock.zip", archive.Body.Bytes()); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	// Reject an archive that inflates accepted output while preserving the target.
	if err := db.Model(&inventory.Movement{}).Where("source_id = ?", itemID).Update("quantity", 99).Error; err != nil {
		t.Fatal(err)
	}
	invalid := performRequest(t, r, "GET", "/desktop-data/export", nil)
	if got := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", invalid.Body.Bytes()); got.Code != 400 {
		t.Fatal(got.Body.String())
	}
	db = restored
	if replay := apply(ship, nil); replay.Sequence != shipped.Sequence || replay.Balance != shipped.Balance {
		t.Fatal("restored replay", replay)
	}
	b, err := inventory.OutputAvailability(db, product.ID, itemID)
	if err != nil || b.OnHand != 9 || b.Reserved != 1 || b.Available != 8 {
		t.Fatal(b, err)
	}
}
