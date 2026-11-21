package handlers_test

import (
	"reflect"
	"sort"
	"testing"
	"time"

	core "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const productionJobs = "/v2/production/jobs"

func confirmedProductionOrder(t *testing.T, router *gin.Engine, modelIDs ...uuid.UUID) salesdto.SalesOrderDTO {
	t.Helper()
	request, _ := seedProposal(t, router, modelIDs...)
	draft := businessRequest[budget.Proposal](t, router, "POST", businessProposals, request, 201)
	issued := businessRequest[budget.Proposal](t, router, "POST", businessProposals+"/"+draft.ID.String()+"/issue",
		budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, 200)
	order := businessRequest[budget.OrderRevision](t, router, "POST", businessOrders, budget.ConfirmationCommand{
		Command: budget.Command{OperationID: uuid.New()}, ProposalID: issued.ID, ProposalRevision: issued.Revision,
	}, 201)
	return getSalesOrderDTO(t, router, order.ID)
}

func planProduction(t *testing.T, router *gin.Engine, printerID uuid.UUID, runs ...[]models.PrintRunItem) (core.ProductionInput, core.ProductionResponse) {
	t.Helper()
	input := core.ProductionInput{
		PrinterID: &printerID, PrintDate: time.Now().UTC(), OperationID: uuid.New(),
	}
	for _, items := range runs {
		input.Runs = append(input.Runs, core.ProductionRunInput{Status: "planned", Items: items})
	}
	job := legacyOrderRequest[core.ProductionResponse](t, router, "POST", productionJobs, input, 201)
	input.Runs = nil
	for _, run := range job.Runs {
		input.Runs = append(input.Runs, core.ProductionRunInput{ID: &run.ID, Status: run.Status, Items: run.Items})
	}
	input.ExpectedRevision = job.Revision
	return input, job
}

func completeProduction(t *testing.T, router *gin.Engine, input core.ProductionInput, jobID uuid.UUID, outcome func(*models.PrintRunItem)) {
	t.Helper()
	path := productionJobs + "/" + jobID.String()
	input.OperationID = uuid.New()
	for i := range input.Runs {
		input.Runs[i].Status = "in_progress"
	}
	started := legacyOrderRequest[core.ProductionResponse](t, router, "PUT", path, input, 200)
	input.OperationID = uuid.New()
	input.ExpectedRevision = started.Revision
	for i := range input.Runs {
		input.Runs[i].Status = "completed"
		for j := range input.Runs[i].Items {
			outcome(&input.Runs[i].Items[j])
		}
	}
	legacyOrderRequest[core.ProductionResponse](t, router, "PUT", path, input, 200)
}

func productionChange(revision int64, links ...salesdto.OrderProductionLinkInput) salesdto.OrderProductionRequest {
	if links == nil {
		links = []salesdto.OrderProductionLinkInput{}
	}
	return salesdto.OrderProductionRequest{ExpectedRevision: &revision, Links: links}
}

func productionLine(t *testing.T, result salesdto.OrderProduction, id uuid.UUID) salesdto.OrderProductionLine {
	t.Helper()
	for _, line := range result.Items {
		if line.SalesOrderItemID == id {
			return line
		}
	}
	t.Fatalf("missing order line %s", id)
	return salesdto.OrderProductionLine{}
}

func TestBusinessProductionAllocatesVerifiedOutputAndRestoresLinks(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	channel, product, finish := createSalesOrderReferences(t, db)
	order := confirmedProductionOrder(t, router, product.ID, finish.ID)
	line, otherLine := order.Items[0].ID, order.Items[1].ID
	if *order.Items[0].ModelID != product.ID {
		line, otherLine = otherLine, line
	}
	path := businessOrders + "/" + order.ID.String() + "/production"
	empty := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200)
	if empty.Status != "not_started" || productionLine(t, empty, line).Status != "not_started" {
		t.Fatalf("unlinked order should have no production progress: %+v", empty)
	}
	legacyOrder := createAllocationOrder(t, router, channel.ID, product.ID, "Quick Sale", 1)
	legacyJobID := createAllocationPrintJob(t, db, product.ID, 2).ID
	legacyOrderRequest[salesdto.SalesOrderAllocationsDTO](t, router, "PUT", "/sales-orders/"+legacyOrder.ID.String()+"/allocations",
		allocationDocument(salesdto.SalesOrderAllocationRequest{
			SalesOrderItemID: legacyOrder.Items[0].ID, PrintJobID: legacyJobID, UnitsAllocated: 1,
		}), 200)
	legacyResponse := legacyOrderRequest[salesdto.PrintJobAllocationsDTO](t, router, "GET", "/print-jobs/"+legacyJobID.String()+"/allocations", nil, 200)
	if legacyResponse.PrintJob.UnitsAvailable != 1 {
		t.Fatal("legacy job capacity changed")
	}
	first, job := planProduction(t, router, printer.ID,
		[]models.PrintRunItem{{ModelID: &product.ID, Planned: 2}, {ModelID: &finish.ID, Planned: 1}},
		[]models.PrintRunItem{{ModelID: &product.ID, Planned: 7}},
	)
	second, reprint := planProduction(t, router, printer.ID, []models.PrintRunItem{{ModelID: &product.ID, Planned: 4}})
	var primary, rejected, different uuid.UUID
	for _, run := range first.Runs {
		for _, item := range run.Items {
			switch item.Planned {
			case 1:
				different = item.ID
			case 2:
				primary = item.ID
			case 7:
				rejected = item.ID
			}
		}
	}
	otherJobItem := second.Runs[0].Items[0].ID
	sourcePath := path + "/sources?line_id=" + line.String()
	candidates := businessRequest[salesdto.ProductionSources](t, router, "GET", sourcePath, nil, 200)
	if len(candidates.Items) != 3 {
		t.Fatalf("expected only the three compatible verified plans, got %+v", candidates)
	}
	plan := salesdto.OrderProductionLinkInput{SalesOrderItemID: line, PrintRunItemID: primary, UnitsPlanned: 2}
	planWithAllocation := plan
	planWithAllocation.UnitsAllocated = 1
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(0, planWithAllocation), 409)
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(0, plan, plan), 400)
	businessRequest[apiResponse](t, router, "PUT", path, map[string]any{"expected_revision": 0}, 400)
	businessRequest[apiResponse](t, router, "PUT", path, map[string]any{
		"expected_revision": 0, "links": []any{}, "fulfilment_status": "completed",
	}, 400)
	initial := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, productionChange(0, plan), 200)
	initialLine := productionLine(t, initial, line)
	if initial.Status != "planned" || initialLine.Status != "planned" ||
		initial.Revision != 1 || initialLine.UnitsPlanned != 2 || initialLine.UnitsOutstanding != 2 ||
		initialLine.UnitsAccepted != 0 || initialLine.UnitsAllocated != 0 || initialLine.Links[0].AllocationID != nil {
		t.Fatalf("plan was treated as accepted supply: %+v", initialLine)
	}
	var movements int64
	if err := db.Model(&inventory.Movement{}).Where("item_type = 'model'").Count(&movements).Error; err != nil || movements != 0 {
		t.Fatalf("planning posted inventory movements: %d %v", movements, err)
	}

	conflictingRun := first
	conflictingRun.OperationID = uuid.New()
	conflictingRun.Runs = append([]core.ProductionRunInput{}, first.Runs...)
	for i := range conflictingRun.Runs {
		conflictingRun.Runs[i].Items = append([]models.PrintRunItem{}, first.Runs[i].Items...)
		for j, item := range conflictingRun.Runs[i].Items {
			if item.ID == primary {
				conflictingRun.Runs[i].Items[j].Planned = 1
			}
		}
	}
	legacyOrderRequest[apiResponse](t, router, "PUT", productionJobs+"/"+job.ID.String(), conflictingRun, 409)
	for i := range conflictingRun.Runs {
		for j, item := range conflictingRun.Runs[i].Items {
			if item.ID == primary {
				conflictingRun.Runs[i].Items[j].Planned = 2
				conflictingRun.Runs[i].Items[j].ModelID = &finish.ID
			}
		}
	}
	legacyOrderRequest[apiResponse](t, router, "PUT", productionJobs+"/"+job.ID.String(), conflictingRun, 409)

	completeProduction(t, router, first, job.ID, func(item *models.PrintRunItem) {
		item.Attempted = item.Planned
		if item.ID != rejected {
			item.Accepted = 1
		}
		item.Rejected = item.Attempted - item.Accepted
	})
	completeProduction(t, router, second, reprint.ID, func(item *models.PrintRunItem) {
		item.Attempted, item.Accepted = 4, 4
	})
	ready := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200)
	if after := productionLine(t, ready, line); after.UnitsAccepted != 0 || after.Links[0].Source.UnitsAccepted != 1 {
		t.Fatalf("unassigned completion changed order progress: %+v", after)
	}
	if ready.Status != "in_progress" || ready.Revision != initial.Revision {
		t.Fatalf("core execution should update derived progress without changing production links: %+v", ready)
	}
	reprintLink := salesdto.OrderProductionLinkInput{SalesOrderItemID: line, PrintRunItemID: otherJobItem, UnitsAllocated: 1}
	otherLink := salesdto.OrderProductionLinkInput{SalesOrderItemID: otherLine, PrintRunItemID: different, UnitsAllocated: 1}
	rejectedLink := salesdto.OrderProductionLinkInput{SalesOrderItemID: line, PrintRunItemID: rejected, UnitsAllocated: 1}
	wrongModel := otherLink
	wrongModel.PrintRunItemID = primary
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, wrongModel), 409)
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, rejectedLink), 409)
	tooMany := reprintLink
	tooMany.UnitsAllocated = 2
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, planWithAllocation, tooMany), 409)

	if err := db.Exec("CREATE TRIGGER fail_business_allocation BEFORE INSERT ON sales_order_allocations BEGIN SELECT RAISE(ABORT,'test failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, reprintLink), 500)
	if rolledBack := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200); !reflect.DeepEqual(ready, rolledBack) {
		t.Fatal("failed replacement changed plans or revision")
	}
	if err := db.Exec("DROP TRIGGER fail_business_allocation").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&inventory.Movement{}).Where("item_type = 'model'").Count(&movements).Error; err != nil || movements == 0 {
		t.Fatalf("core output did not create finished goods: %d %v", movements, err)
	}
	partial := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, productionChange(1, planWithAllocation, otherLink), 200)
	if progress := productionLine(t, partial, line); partial.Status != "in_progress" || progress.Status != "in_progress" ||
		progress.UnitsAccepted != 1 || progress.UnitsOutstanding != 1 || productionLine(t, partial, otherLine).Status != "complete" {
		t.Fatalf("partial production must leave the order open: %+v", partial)
	}
	reprintLink.UnitsAllocated = 2
	oneLineOnly := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, productionChange(2, plan, reprintLink), 200)
	if oneLineOnly.Status != "in_progress" || productionLine(t, oneLineOnly, line).Status != "complete" ||
		productionLine(t, oneLineOnly, otherLine).Status != "not_started" {
		t.Fatalf("completed output on one line cannot satisfy another line: %+v", oneLineOnly)
	}
	existingAllocation := productionLine(t, oneLineOnly, line).Links
	reprintLink.UnitsAllocated = 1
	change := productionChange(3, planWithAllocation, reprintLink, otherLink)
	saved := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, change, 200)
	lineResult := productionLine(t, saved, line)
	if saved.Status != "complete" || lineResult.Status != "complete" ||
		lineResult.UnitsRequired != 2 || lineResult.UnitsPlanned != 2 || lineResult.UnitsAccepted != 2 ||
		lineResult.UnitsAllocated != 2 || lineResult.UnitsOutstanding != 0 || len(lineResult.Links) != 2 {
		t.Fatalf("incorrect line progress: %+v", lineResult)
	}
	secondLine := productionLine(t, saved, otherLine)
	if secondLine.Status != "complete" || secondLine.UnitsAllocated != 1 || secondLine.UnitsOutstanding != 0 {
		t.Fatalf("mixed-model item was not allocated independently: %+v", secondLine)
	}
	for i, link := range lineResult.Links {
		if link.PrintRunItemID == reprintLink.PrintRunItemID &&
			!reflect.DeepEqual(link.AllocationID, existingAllocation[i].AllocationID) {
			t.Fatal("changing assigned quantities recreated the existing allocation")
		}
	}
	afterOrder := getSalesOrderDTO(t, router, order.ID)
	if afterOrder.FulfilmentStatus != order.FulfilmentStatus || afterOrder.PaymentStatus != order.PaymentStatus ||
		!reflect.DeepEqual(afterOrder.Profitability, order.Profitability) {
		t.Fatalf("production completion changed fulfilment, payment or financials: %+v", afterOrder)
	}
	legacyOrderRequest[apiResponse](t, router, "PUT", "/sales-orders/"+order.ID.String()+"/status",
		map[string]string{"fulfilment_status": "completed"}, 409)
	var finalMovements int64
	if err := db.Model(&inventory.Movement{}).Where("item_type = 'model'").Count(&finalMovements).Error; err != nil || finalMovements != movements {
		t.Fatalf("allocation wrote to Inventory: %d -> %d (%v)", movements, finalMovements, err)
	}
	replayed := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, change, 200)
	if !reflect.DeepEqual(replayed, saved) || saved.Revision != 4 {
		t.Fatal("retry changed allocation identity or revision")
	}
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1), 409)
	legacyOrderRequest[apiResponse](t, router, "DELETE", "/print-jobs/"+job.ID.String(), nil, 409)

	archive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	target, _ := setupDesktopTestRouter(t)
	restored := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "production.zip", archive.Body.Bytes())
	if restored.Code != 200 {
		t.Fatalf("restore: %s", restored.Body.String())
	}
	if result := businessRequest[salesdto.OrderProduction](t, target, "GET", path, nil, 200); !reflect.DeepEqual(result, saved) {
		t.Fatalf("restore lost production data: %+v", result)
	}
	if err := db.Exec("UPDATE sales_order_allocations SET print_job_id = ? WHERE print_run_item_id = ?", reprint.ID, primary).Error; err != nil {
		t.Fatal(err)
	}
	badArchive := performRequest(t, router, "GET", "/desktop-data/export", nil)
	rejectedRestore := performMultipartFileRequest(t, target, "POST", "/desktop-data/import", "file", "invalid.zip", badArchive.Body.Bytes())
	if rejectedRestore.Code != 400 || !reflect.DeepEqual(businessRequest[salesdto.OrderProduction](t, target, "GET", path, nil, 200), saved) {
		t.Fatal("invalid production source was restored or changed the target")
	}
	corrected := businessRequest[salesdto.OrderProduction](t, target, "PUT", path,
		productionChange(saved.Revision, planWithAllocation, otherLink), 200)
	if progress := productionLine(t, corrected, line); corrected.Status != "in_progress" ||
		progress.Status != "in_progress" || progress.UnitsAccepted != 1 || progress.UnitsOutstanding != 1 {
		t.Fatalf("removing assigned units must make the demand outstanding again: %+v", corrected)
	}
}

func TestBusinessProductionUsesInspectedUnitsAndRecoversAfterFailure(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	_, product, _ := createSalesOrderReferences(t, db)
	order := confirmedProductionOrder(t, router, product.ID)
	path := businessOrders + "/" + order.ID.String() + "/production"
	var lineID uuid.UUID
	for _, item := range order.Items {
		if item.ModelID != nil && *item.ModelID == product.ID {
			lineID = item.ID
		}
	}
	input, job := planProduction(t, router, printer.ID,
		[]models.PrintRunItem{{ModelID: &product.ID, Planned: 2}})
	failed := salesdto.OrderProductionLinkInput{SalesOrderItemID: lineID, PrintRunItemID: input.Runs[0].Items[0].ID, UnitsPlanned: 2}
	businessRequest[salesdto.OrderProduction](t, router, "PUT", path, productionChange(0, failed), 200)

	jobPath := productionJobs + "/" + job.ID.String()
	input.OperationID = uuid.New()
	input.Runs[0].Status = "in_progress"
	input.Runs[0].Items[0].Attempted = 2
	input.Runs[0].Items[0].Accepted = 2
	started := legacyOrderRequest[core.ProductionResponse](t, router, "PUT", jobPath, input, 200)
	tentative := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200)
	if line := productionLine(t, tentative, lineID); tentative.Status != "in_progress" ||
		line.UnitsAccepted != 0 || line.Links[0].Source.UnitsAccepted != 0 || line.UnitsOutstanding != 2 {
		t.Fatalf("tentative counts became verified progress: %+v", tentative)
	}
	claim := failed
	claim.UnitsAllocated = 1
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, claim), 409)

	input.OperationID = uuid.New()
	input.ExpectedRevision = started.Revision
	input.Runs[0].Status = "failed"
	input.Runs[0].Items[0].Accepted = 0
	input.Runs[0].Items[0].Rejected = 2
	legacyOrderRequest[core.ProductionResponse](t, router, "PUT", jobPath, input, 200)
	rejected := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200)
	if line := productionLine(t, rejected, lineID); rejected.Status != "in_progress" || line.UnitsPlanned != 0 ||
		line.UnitsAccepted != 0 || line.UnitsOutstanding != 2 || line.Links[0].Source.Status != "failed" {
		t.Fatalf("failed output should leave demand outstanding: %+v", rejected)
	}
	businessRequest[apiResponse](t, router, "PUT", path, productionChange(1, claim), 409)

	reprint, replacement := planProduction(t, router, printer.ID,
		[]models.PrintRunItem{{ModelID: &product.ID, Planned: 2}})
	completeProduction(t, router, reprint, replacement.ID, func(item *models.PrintRunItem) {
		item.Attempted, item.Accepted = 2, 2
	})
	output := salesdto.OrderProductionLinkInput{SalesOrderItemID: lineID,
		PrintRunItemID: replacement.Runs[0].Items[0].ID, UnitsAllocated: 2}
	finished := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, productionChange(1, failed, output), 200)
	if line := productionLine(t, finished, lineID); finished.Status != "in_progress" ||
		line.Status != "complete" || line.UnitsAccepted != 2 ||
		line.UnitsOutstanding != 0 || len(line.Links) != 2 {
		t.Fatalf("verified replacement failed to complete production while preserving the failed run link: %+v", finished)
	}
}

func TestBusinessProductionSerializesSharedRunCapacity(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	_, product, _ := createSalesOrderReferences(t, db)
	first := confirmedProductionOrder(t, router, product.ID)
	second := confirmedProductionOrder(t, router, product.ID)
	input, job := planProduction(t, router, printer.ID, []models.PrintRunItem{{ModelID: &product.ID, Planned: 3}})
	completeProduction(t, router, input, job.ID, func(item *models.PrintRunItem) {
		item.Attempted, item.Accepted = 3, 3
	})
	source := input.Runs[0].Items[0].ID
	start := make(chan struct{})
	statuses := make(chan int, 2)
	for _, order := range []salesdto.SalesOrderDTO{first, second} {
		lineID := order.Items[0].ID
		if order.Items[0].ModelID == nil {
			lineID = order.Items[1].ID
		}
		go func() {
			<-start
			request := productionChange(0, salesdto.OrderProductionLinkInput{
				SalesOrderItemID: lineID, PrintRunItemID: source, UnitsAllocated: 2,
			})
			result := performRequest(t, router, "PUT", businessOrders+"/"+order.ID.String()+"/production", request)
			statuses <- result.Code
		}()
	}
	close(start)
	got := []int{<-statuses, <-statuses}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{200, 409}) {
		t.Fatalf("concurrent capacity protection: %v", got)
	}
	var units int64
	if err := db.Table("sales_order_allocations").Select("SUM(units_allocated)").
		Where("print_run_item_id = ?", source).Scan(&units).Error; err != nil || units != 2 {
		t.Fatalf("incorrect accepted allocation total: %d %v", units, err)
	}
	for _, order := range []salesdto.SalesOrderDTO{first, second} {
		path := businessOrders + "/" + order.ID.String() + "/production"
		current := businessRequest[salesdto.OrderProduction](t, router, "GET", path, nil, 200)
		if current.Revision == 0 {
			lineID := order.Items[0].ID
			if order.Items[0].ModelID == nil {
				lineID = order.Items[1].ID
			}
			request := productionChange(0, salesdto.OrderProductionLinkInput{
				SalesOrderItemID: lineID, PrintRunItemID: source, UnitsAllocated: 1,
			})
			saved := businessRequest[salesdto.OrderProduction](t, router, "PUT", path, request, 200)
			if got := productionLine(t, saved, lineID).Links[0].Source.UnitsAvailable; got != 0 {
				t.Fatalf("used all shared output, %d still marked available", got)
			}
			if err := db.Exec("UPDATE sales_orders SET fulfilment_status = 'shipped' WHERE id = ?", order.ID).Error; err != nil {
				t.Fatal(err)
			}
			businessRequest[apiResponse](t, router, "PUT", path, productionChange(1), 409)
		}
	}
}
