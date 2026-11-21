package handlers_test

import (
	"reflect"
	"sort"
	"testing"

	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func legacyOrderRequest[T any](t *testing.T, router *gin.Engine, method, path string, body any, status int) T {
	t.Helper()
	res := performRequest(t, router, method, path, body)
	if res.Code != status {
		t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
	}
	var result T
	decodeBody(t, res, &result)
	return result
}

const businessOrders = "/v2/business/orders"

func TestBusinessOrderConfirmationAmendmentAndRestore(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	request, _ := seedProposal(t, router)
	draft := businessRequest[budget.Proposal](t, router, "POST", businessProposals, request, 201)
	path := businessProposals + "/" + draft.ID.String()
	input := budget.ConfirmationCommand{Command: budget.Command{OperationID: uuid.New()}, ProposalID: draft.ID, ProposalRevision: 1, PurchaseOrder: "PO-1"}
	businessRequest[apiResponse](t, router, "POST", businessOrders, input, 409)
	issued := businessRequest[budget.Proposal](t, router, "POST", path+"/issue", budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, 200)
	input.ProposalRevision = issued.Revision
	if err := db.Exec("CREATE TRIGGER fail_accept BEFORE INSERT ON business_order_revisions BEGIN SELECT RAISE(ABORT,'private'); END").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[apiResponse](t, router, "POST", businessOrders, input, 500)
	assertSalesOrderCounts(t, db, 0, 0)
	if err := db.Exec("DROP TRIGGER fail_accept").Error; err != nil {
		t.Fatal(err)
	}
	order := businessRequest[budget.OrderRevision](t, router, "POST", businessOrders, input, 201)
	replay := businessRequest[budget.OrderRevision](t, router, "POST", businessOrders, input, 201)
	if !reflect.DeepEqual(order, replay) || order.ConfirmedAt.IsZero() || order.CustomerID != request.CustomerID || order.ProposalRevision != issued.Revision {
		t.Fatal("acceptance lost original identity or time")
	}
	duplicate := input
	duplicate.OperationID = uuid.New()
	businessRequest[apiResponse](t, router, "POST", businessOrders, duplicate, 409)
	detail := getSalesOrderDTO(t, router, order.ID)
	if detail.LegacyWriteSupported || detail.Profitability.RevenueCents != issued.NetCents || len(detail.Items) != 2 || detail.Items[0].DiscountCents == 0 {
		t.Fatalf("incorrect order projection: %+v", detail)
	}
	legacy := "/sales-orders/" + order.ID.String()
	legacyOrderRequest[apiResponse](t, router, "PUT", legacy+"/status", map[string]string{"payment_status": "paid"}, 409)
	legacyOrderRequest[apiResponse](t, router, "PUT", legacy+"/allocations", allocationDocument(), 409)
	resource := businessOrders + "/" + order.ID.String()
	businessRequest[apiResponse](t, router, "POST", resource+"/amendments", map[string]any{"expected_revision": 1, "operation_id": uuid.New(), "reason": "Change price", "unit_price_cents": 10, "purchase_order": "PO-2"}, 400)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	for _, po := range []string{"PO-2", "PO-3"} {
		go func() {
			<-start
			r := performRequest(t, router, "POST", resource+"/amendments", budget.AmendmentCommand{Command: budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, PurchaseOrder: &po, Reason: "Buyer corrected PO"})
			statuses <- r.Code
		}()
	}
	close(start)
	got := []int{<-statuses, <-statuses}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{200, 409}) {
		t.Fatalf("concurrent amendments: %v", got)
	}
	original := businessRequest[budget.OrderRevision](t, router, "GET", resource+"?revision=1", nil, 200)
	current := businessRequest[budget.OrderRevision](t, router, "GET", resource, nil, 200)
	if !reflect.DeepEqual(original, order) || current.Revision != 2 || current.PurchaseOrder == order.PurchaseOrder || current.Reason == "" {
		t.Fatal("invalid amendment history")
	}
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	target, _ := setupDesktopTestRouter(t)
	restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive.Body.Bytes())
	if restored.Code != 200 {
		t.Fatalf("restore: %s", restored.Body.String())
	}
	after := businessRequest[budget.OrderRevision](t, target, "GET", resource, nil, 200)
	if !reflect.DeepEqual(after, current) || !reflect.DeepEqual(getSalesOrderDTO(t, target, order.ID), detail) {
		t.Fatal("restored order differs")
	}
	if err := db.Model(&storage.SalesOrderItem{}).Where("id = ?", detail.Items[0].ID).Update("discount_cents", 0).Error; err != nil {
		t.Fatal(err)
	}
	invalidArchive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	rejected := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", invalidArchive.Body.Bytes())
	if rejected.Code != 400 || !reflect.DeepEqual(getSalesOrderDTO(t, target, order.ID), detail) {
		t.Fatal("restore accepted inconsistent discounted revenue or changed target")
	}
	if err := db.Model(&storage.SalesOrder{}).Where("id = ?", order.ID).Update("fulfilment_status", "shipped").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[apiResponse](t, router, "POST", resource+"/amendments", budget.AmendmentCommand{Command: budget.Command{OperationID: uuid.New(), ExpectedRevision: 2}, PurchaseOrder: new("new"), Reason: "After dispatch"}, 409)
}

func TestDraftEditsPreserveAllocatedIDs(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	order := createAllocationOrder(t, router, channel.ID, product.ID, "sample", 4)
	id := order.Items[0].ID
	job := createAllocationPrintJob(t, db, product.ID, 4)
	path := "/sales-orders/" + order.ID.String()
	input := allocationDocument(salesdto.SalesOrderAllocationRequest{SalesOrderItemID: id, PrintJobID: job.ID, UnitsAllocated: 2})
	a := legacyOrderRequest[salesdto.SalesOrderAllocationsDTO](t, router, "PUT", path+"/allocations", input, 200)
	row := map[string]any{"id": id, "model_id": product.ID, "quantity": 1, "unit_price_cents": 20}
	update := salesOrderRequest(channel.ID, []map[string]any{row})
	legacyOrderRequest[apiResponse](t, router, "PUT", path, update, 409)
	row["quantity"] = 3
	updated := legacyOrderRequest[salesdto.SalesOrderDTO](t, router, "PUT", path, update, 200)
	b := legacyOrderRequest[salesdto.SalesOrderAllocationsDTO](t, router, "PUT", path+"/allocations", input, 200)
	if updated.Items[0].ID != id || len(b.Items[0].Allocations) != 1 || a.Items[0].Allocations[0].ID != b.Items[0].Allocations[0].ID {
		t.Fatal("draft edit lost line or allocation identity")
	}
}

func TestOrderRejectsSupersededAndExpiredRevision(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	request, _ := seedProposal(t, router)
	draft := businessRequest[budget.Proposal](t, router, "POST", businessProposals, request, 201)
	path := businessProposals + "/" + draft.ID.String()
	issue := func(revision int64) {
		businessRequest[budget.Proposal](t, router, "POST", path+"/issue", budget.Command{OperationID: uuid.New(), ExpectedRevision: revision}, 200)
	}
	issue(1)
	request.Command = budget.Command{OperationID: uuid.New(), ExpectedRevision: 2}
	businessRequest[budget.Proposal](t, router, "PUT", path, request, 200)
	issue(3)
	input := budget.ConfirmationCommand{Command: budget.Command{OperationID: uuid.New()}, ProposalID: draft.ID, ProposalRevision: 2}
	businessRequest[apiResponse](t, router, "POST", businessOrders, input, 409)
	if err := db.Exec("UPDATE business_proposals SET terms=json_set(terms,'$.valid_until','2000-01-01') WHERE id=? AND revision=4", draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	input.ProposalRevision = 4
	businessRequest[apiResponse](t, router, "POST", businessOrders, input, 409)
	assertSalesOrderCounts(t, db, 0, 0)
}
