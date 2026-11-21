package handlers_test

import (
	"reflect"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/google/uuid"
)

func TestBusinessFulfilmentRoundTrip(t *testing.T) {
	r, db, printer := setupTestRouterWithDefaultPrinter(t)
	_, product, _ := createSalesOrderReferences(t, db)
	order := confirmedProductionOrder(t, r, product.ID, product.ID)
	base := businessOrders + "/" + order.ID.String()
	path := base + "/fulfilment"
	input, job := planProduction(t, r, printer.ID, []models.PrintRunItem{{ModelID: &product.ID, Planned: 10}})
	completeProduction(t, r, input, job.ID, func(i *models.PrintRunItem) { i.Accepted, i.Attempted = 10, 10 })
	source := input.Runs[0].Items[0].ID
	links := []salesdto.OrderProductionLinkInput{}
	for _, item := range order.Items {
		links = append(links, salesdto.OrderProductionLinkInput{SalesOrderItemID: item.ID, PrintRunItemID: source, UnitsAllocated: int64(item.Quantity)})
	}
	saved := businessRequest[salesdto.OrderProduction](t, r, "PUT", base+"/production", productionChange(0, links...), 200)
	stock := func(hand, reserved, available int64) {
		t.Helper()
		b, err := inventory.OutputAvailability(db, product.ID, source)
		if err != nil || b.OnHand != hand || b.Reserved != reserved || b.Available != available {
			t.Fatalf("unexpected stock %+v, %v", b, err)
		}
	}
	command := func(kind string, allocation, ref uuid.UUID, units int64) map[string]any {
		t.Helper()
		b, err := inventory.OutputAvailability(db, product.ID, source)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{"operation_id": uuid.New(), "source_id": uuid.New(), "kind": kind, "allocation_id": allocation, "reference": ref, "units": units, "expected_revision": b.Revision, "reason": "Sample shipment"}
	}
	var holds, allocations []uuid.UUID
	for _, line := range saved.Items {
		a := *line.Links[0].AllocationID
		req := command("reserve", a, a, line.UnitsAllocated)
		if len(holds) == 0 {
			businessRequest[apiResponse](t, r, "POST", path, req, 409)
			if res := performRequest(t, r, "PUT", "/features", map[string]bool{"sales_orders": true}); res.Code != 200 {
				t.Fatal(res.Body.String())
			}
		}
		got := businessRequest[inventory.Receipt](t, r, "POST", path, req, 200)
		if replay := businessRequest[inventory.Receipt](t, r, "POST", path, req, 200); !reflect.DeepEqual(got, replay) {
			t.Fatal("retry changed receipt")
		}
		req["operation_id"] = uuid.New()
		businessRequest[apiResponse](t, r, "POST", path, req, 409)
		holds = append(holds, got.OperationID)
		allocations = append(allocations, a)
	}
	stock(10, 3, 7)
	if res := performRequest(t, r, "PUT", "/features", map[string]bool{"sales_orders": false}); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	businessRequest[apiResponse](t, r, "PUT", base+"/cancel-remainder", nil, 409)
	stock(10, 3, 7)
	if res := performRequest(t, r, "PUT", "/features", map[string]bool{"sales_orders": true}); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	other := confirmedProductionOrder(t, r, product.ID)
	businessRequest[apiResponse](t, r, "POST", businessOrders+"/"+other.ID.String()+"/fulfilment", command("dispatch", allocations[0], holds[0], 1), 404)
	businessRequest[apiResponse](t, r, "POST", path, command("dispatch", allocations[1], holds[0], 1), 409)
	businessRequest[apiResponse](t, r, "PUT", base+"/production", productionChange(saved.Revision), 409)
	// Roll back the releases if the final commercial write fails.
	if err := db.Exec("CREATE TRIGGER fail_cancel BEFORE UPDATE OF fulfilment_status ON sales_orders BEGIN SELECT RAISE(ABORT,'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[apiResponse](t, r, "PUT", base+"/cancel-remainder", nil, 500)
	stock(10, 3, 7)
	if err := db.Exec("DROP TRIGGER fail_cancel").Error; err != nil {
		t.Fatal(err)
	}
	before := performRequest(t, r, "GET", "/desktop-data/export", nil)
	var shipments []inventory.Receipt
	for i, line := range saved.Items {
		req := command("dispatch", allocations[i], holds[i], line.UnitsAllocated)
		got := businessRequest[inventory.Receipt](t, r, "POST", path, req, 200)
		if replay := businessRequest[inventory.Receipt](t, r, "POST", path, req, 200); !reflect.DeepEqual(got, replay) {
			t.Fatal("duplicate dispatch")
		}
		shipments = append(shipments, got)
	}
	stock(7, 0, 7)
	doc := businessRequest[map[string]any](t, r, "GET", path, nil, 200)
	if doc["status"] != "shipped" {
		t.Fatal(doc)
	}
	req := command("restock", allocations[0], shipments[0].OperationID, 1)
	businessRequest[inventory.Receipt](t, r, "POST", path, req, 200)
	stock(8, 0, 8)
	target, _ := setupDesktopTestRouter(t)
	archive := performRequest(t, r, "GET", "/desktop-data/export", nil)
	if got := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes()); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	businessRequest[inventory.Receipt](t, target, "POST", path, req, 200)
	bad := command("reject_return", allocations[0], shipments[0].OperationID, shipments[0].Units)
	businessRequest[apiResponse](t, r, "POST", path, bad, 409)
	if err := db.Model(&inventory.Movement{}).Where("operation_id = ?", holds[0]).Update("reference", uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	corrupt := performRequest(t, r, "GET", "/desktop-data/export", nil)
	if got := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", corrupt.Body.Bytes()); got.Code != 400 {
		t.Fatal(got.Body.String())
	}
	businessRequest[inventory.Receipt](t, target, "POST", path, req, 200)
	// Restore the outstanding reservations and cancel the unshipped remainder.
	if got := performMultipartFileRequest(t, r, "POST", "/desktop-data/import", "file", "sample.zip", before.Body.Bytes()); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	businessRequest[inventory.Receipt](t, r, "POST", path, command("dispatch", allocations[0], holds[0], 1), 200)
	for range 2 {
		businessRequest[map[string]bool](t, r, "PUT", base+"/cancel-remainder", nil, 200)
	}
	stock(9, 0, 9)
	cancelled := businessRequest[map[string]any](t, r, "GET", path, nil, 200)
	if cancelled["status"] != "partially_shipped" || cancelled["cancelled"] != true {
		t.Fatal(cancelled)
	}
	candidates := businessRequest[salesdto.ProductionSources](t, r, "GET", base+"/production/sources?line_id="+order.Items[0].ID.String(), nil, 200)
	if len(candidates.Items) != 1 || candidates.Items[0].UnitsAvailable != 9 {
		t.Fatal(candidates)
	}
}
