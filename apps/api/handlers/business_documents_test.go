package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	core "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type activityTestPage struct {
	Items []struct {
		ID          string     `json:"id"`
		Kind        string     `json:"kind"`
		CreatedAt   time.Time  `json:"created_at"`
		ReferenceID *uuid.UUID `json:"reference_id"`
		Detail      string     `json:"detail"`
		Units       int64      `json:"units"`
	}
	HasMore bool `json:"has_more"`
}

func TestBusinessIssuedPDFUsesFrozenRevision(t *testing.T) {
	router, _ := setupDesktopTestRouter(t)
	request, _ := seedProposal(t, router)
	draft := businessRequest[budget.Proposal](t, router, "POST", businessProposals, request, 201)
	root := businessProposals + "/" + draft.ID.String()
	pdf := root + "/document/pdf"
	businessRequest[apiResponse](t, router, "GET", pdf, nil, 400)
	businessRequest[apiResponse](t, router, "GET", pdf+"?revision=1", nil, 404)
	businessRequest[budget.Proposal](t, router, "POST", root+"/issue",
		budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, 200)
	businessRequest[apiResponse](t, router, "GET", pdf+"?revision=2&language=other", nil, 400)
	download := func() []byte {
		t.Helper()
		got := performRequest(t, router, "GET", pdf+"?revision=2&language=pt-PT", nil)
		if got.Code != 200 || got.Header().Get("Content-Type") != "application/pdf" ||
			got.Header().Get("Cache-Control") != "private, no-store" || got.Header().Get("X-Content-Type-Options") != "nosniff" ||
			!strings.HasSuffix(got.Header().Get("Content-Disposition"), "-r2.pdf") ||
			!bytes.HasPrefix(got.Body.Bytes(), []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(got.Body.Bytes()), []byte("%%EOF")) {
			t.Fatalf("invalid proposal download: %d %v", got.Code, got.Header())
		}
		return got.Body.Bytes()
	}
	before := download()
	request.OperationID, request.ExpectedRevision = uuid.New(), 2
	request.Terms.Title, request.Terms.PaymentTerms = "Different title", "Different payment terms"
	businessRequest[budget.Proposal](t, router, "PUT", root, request, 200)
	if !bytes.Equal(before, download()) {
		t.Fatal("editing a new draft changed the PDF of the issued revision")
	}
}

func uploadOrderFile(t *testing.T, router *gin.Engine, base, name string, op uuid.UUID, data []byte, status int) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{"name": {name}, "operation_id": {op.String()}}
	req := httptest.NewRequest(http.MethodPost, base+"/attachments?"+query.Encode(), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	got := httptest.NewRecorder()
	router.ServeHTTP(got, req)
	if got.Code != status {
		t.Fatalf("upload %q: got %d, want %d: %s", name, got.Code, status, got.Body.String())
	}
	return got
}

func TestBusinessAttachmentsRoundTripRejectCorruptionAndRollback(t *testing.T) {
	router, db := setupDesktopTestRouter(t)
	order := confirmedProductionOrder(t, router)
	other := confirmedProductionOrder(t, router)
	base := businessOrders + "/" + order.ID.String()
	otherBase := businessOrders + "/" + other.ID.String()
	data := []byte("%PDF-1.7\nsynthetic purchase order\n%%EOF\n")
	name := "Ordem de produção.pdf"
	op := uuid.New()
	uploadOrderFile(t, router, base, name, op, data, 409)
	legacyBackup := performRequest(t, router, "GET", "/desktop-data/export", nil)
	if legacyBackup.Code != 200 {
		t.Fatal(legacyBackup.Body.String())
	}
	if got := performRequest(t, router, "PUT", "/features", map[string]bool{"sales_orders": true}); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	for _, invalid := range []string{"../unsafe.pdf", "unsafe\\path.pdf", "unsafe\n.pdf"} {
		uploadOrderFile(t, router, base, invalid, uuid.New(), data, 400)
	}
	uploadOrderFile(t, router, base, "fake.png", uuid.New(), data, 415)
	uploadOrderFile(t, router, base, "oversize.pdf", uuid.New(), make([]byte, storage.MaxOrderAttachmentBytes+1), 413)
	created := uploadOrderFile(t, router, base, name, op, data, 201)
	var attachment storage.OrderAttachment
	if err := json.Unmarshal(created.Body.Bytes(), &attachment); err != nil || attachment.ID == uuid.Nil ||
		attachment.Checksum != storage.AttachmentChecksum(data) || attachment.Size != len(data) || attachment.MediaType != "application/pdf" ||
		strings.Contains(created.Body.String(), "content") || strings.Contains(created.Body.String(), "operation_id") {
		t.Fatalf("unexpected attachment metadata: %s (%v)", created.Body.String(), err)
	}
	if replay := uploadOrderFile(t, router, base, name, op, data, 200); replay.Body.String() != created.Body.String() {
		t.Fatal("matched retry changed the attachment metadata")
	}
	uploadOrderFile(t, router, base, "different.pdf", op, data, 409)
	uploadOrderFile(t, router, otherBase, name, op, data, 409)
	path := "/attachments/" + attachment.ID.String()
	businessRequest[apiResponse](t, router, "GET", otherBase+path, nil, 404)
	businessRequest[apiResponse](t, router, "GET", base+"/attachments/bad", nil, 400)
	assertDownload := func(r *gin.Engine) {
		t.Helper()
		got := performRequest(t, r, "GET", base+path, nil)
		disposition, params, err := mime.ParseMediaType(got.Header().Get("Content-Disposition"))
		if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), data) || got.Header().Get("Content-Type") != "application/octet-stream" ||
			disposition != "attachment" || params["filename"] != name || err != nil ||
			got.Header().Get("X-Content-Type-Options") != "nosniff" || got.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("invalid download: %d %v %v", got.Code, got.Header(), err)
		}
	}
	assertDownload(router)
	activity := businessRequest[activityTestPage](t, router, "GET", base+"/activity", nil, 200)
	if len(activity.Items) != 2 || activity.Items[0].Kind != "attachment_added" ||
		activity.Items[0].Detail != name || *activity.Items[0].ReferenceID != attachment.ID ||
		activity.Items[0].CreatedAt.IsZero() || activity.Items[1].Kind != "proposal_accepted" {
		t.Fatalf("unexpected order activity: %+v", activity)
	}

	target, targetDB := setupDesktopTestRouter(t)
	restore := func(archive []byte, status int) {
		t.Helper()
		got := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "sample.zip", archive)
		if got.Code != status {
			t.Fatalf("restore: got %d, want %d: %s", got.Code, status, got.Body.String())
		}
	}
	restore(rewriteBackupSchemaVersion(t, legacyBackup.Body.Bytes(), 15), 200)
	businessRequest[apiResponse](t, target, "GET", base+path, nil, 404)
	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	if archive.Code != 200 {
		t.Fatal(archive.Body.String())
	}
	restore(archive.Body.Bytes(), 200)
	assertDownload(target)
	restored := businessRequest[activityTestPage](t, target, "GET", base+"/activity", nil, 200)
	if !reflect.DeepEqual(activity, restored) {
		t.Fatalf("backup changed order activity: %+v", restored)
	}
	restore(rewriteBackupSchemaVersion(t, archive.Body.Bytes(), 15), 400)
	if err := db.Model(&storage.OrderAttachment{}).Where("id = ?", attachment.ID).Update("checksum", "wrong").Error; err != nil {
		t.Fatal(err)
	}
	tampered := performRequest(t, router, "GET", "/desktop-data/export", nil)
	restore(tampered.Body.Bytes(), 400)
	assertDownload(target)

	if err := targetDB.Exec(`CREATE TRIGGER fail_attachment_restore BEFORE INSERT ON business_order_attachments
		BEGIN SELECT RAISE(ABORT, 'private failure detail'); END`).Error; err != nil {
		t.Fatal(err)
	}
	restore(archive.Body.Bytes(), 500)
	assertDownload(target)
	uploadOrderFile(t, target, base, "another.pdf", uuid.New(), data, 500)
	after := businessRequest[activityTestPage](t, target, "GET", base+"/activity", nil, 200)
	if !reflect.DeepEqual(activity, after) {
		t.Fatal("failed upload changed the timeline")
	}
}

func TestBusinessActivityRecordsRunTransitionsAndPreservesLegacy(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	channel, product, _ := createSalesOrderReferences(t, db)
	legacy := createAllocationOrder(t, router, channel.ID, product.ID, "Quick Sale", 1)
	legacyPage := businessRequest[activityTestPage](t, router, "GET", businessOrders+"/"+legacy.ID.String()+"/activity", nil, 200)
	if legacyPage.Items == nil || len(legacyPage.Items) != 0 {
		t.Fatalf("invented legacy events: %+v", legacyPage)
	}
	order := confirmedProductionOrder(t, router, product.ID)
	base := businessOrders + "/" + order.ID.String()
	businessRequest[budget.OrderRevision](t, router, "POST", base+"/amendments", budget.AmendmentCommand{
		Command: budget.Command{OperationID: uuid.New(), ExpectedRevision: 1},
		Reason:  "Customer confirmed collection", DeliveryTerms: new("Collection Friday"),
	}, 200)
	input, job := planProduction(t, router, printer.ID, []models.PrintRunItem{{ModelID: &product.ID, Planned: 2}})
	link := salesdto.OrderProductionLinkInput{PrintRunItemID: input.Runs[0].Items[0].ID, UnitsPlanned: 2}
	for _, item := range order.Items {
		if item.ModelID != nil {
			link.SalesOrderItemID = item.ID
		}
	}
	plan := productionChange(0, link)
	for range 2 {
		businessRequest[salesdto.OrderProduction](t, router, "PUT", base+"/production", plan, 200)
	}
	input.OperationID = uuid.New()
	input.Runs[0].Status = "in_progress"
	jobPath := productionJobs + "/" + job.ID.String()
	if err := db.Exec(`CREATE TRIGGER fail_run_event BEFORE INSERT ON business_order_events
		WHEN new.kind = 'run_in_progress' BEGIN SELECT RAISE(ABORT, 'private failure detail'); END`).Error; err != nil {
		t.Fatal(err)
	}
	legacyOrderRequest[apiResponse](t, router, "PUT", jobPath, input, 500)
	var run models.PrintRun
	if err := db.First(&run, "id = ?", job.Runs[0].ID).Error; err != nil || run.Status != "planned" {
		t.Fatalf("failed event write left run status %q: %v", run.Status, err)
	}
	if err := db.Exec("DROP TRIGGER fail_run_event").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		legacyOrderRequest[core.ProductionResponse](t, router, "PUT", jobPath, input, 200)
	}
	businessRequest[salesdto.OrderProduction](t, router, "PUT", base+"/production", productionChange(1), 200)
	items := businessRequest[activityTestPage](t, router, "GET", base+"/activity", nil, 200).Items
	want := []string{"production_revised", "run_in_progress", "production_revised", "order_amended", "proposal_accepted"}
	if len(items) != len(want) {
		t.Fatalf("unexpected timeline: %+v", items)
	}
	for i, item := range items {
		if item.Kind != want[i] || item.ID == "" || item.CreatedAt.IsZero() {
			t.Fatalf("unexpected timeline event %d: %+v", i, item)
		}
	}
	if items[1].ReferenceID == nil || *items[1].ReferenceID != job.Runs[0].ID || items[3].Detail != "Customer confirmed collection" {
		t.Fatal("timeline lost the run reference or amendment reason")
	}
}
