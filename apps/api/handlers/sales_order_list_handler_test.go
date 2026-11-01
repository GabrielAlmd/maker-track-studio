package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type salesOrderPage struct {
	Items    []salesdto.SalesOrderDTO `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type salesOrderListFixture struct {
	channelOne salesmodels.SalesChannel
	channelTwo salesmodels.SalesChannel
	modelOne   models.Model
	modelTwo   models.Model
	orderOne   salesdto.SalesOrderDTO
	orderTwo   salesdto.SalesOrderDTO
	orderThree salesdto.SalesOrderDTO
}

func createSalesOrderListFixture(t *testing.T, router *gin.Engine, db *gorm.DB) salesOrderListFixture {
	t.Helper()
	channelOne, modelOne, modelTwo := createSalesOrderReferences(t, db)
	channelTwo := salesmodels.SalesChannel{Name: "Craft marketplace"}
	if err := db.Create(&channelTwo).Error; err != nil {
		t.Fatal(err)
	}

	create := func(reference, date string, channel salesmodels.SalesChannel, model models.Model, fulfilment, payment string) salesdto.SalesOrderDTO {
		payload := salesOrderRequest(channel.ID, []map[string]any{
			{"model_id": model.ID.String(), "quantity": 1, "unit_price_cents": 1000},
		})
		payload["reference"] = reference
		payload["order_date"] = date
		payload["fulfilment_status"] = fulfilment
		payload["payment_status"] = payment
		return createSalesOrderForTest(t, router, payload)
	}
	fixture := salesOrderListFixture{
		channelOne: channelOne,
		channelTwo: channelTwo,
		modelOne:   modelOne,
		modelTwo:   modelTwo,
		orderOne:   create("ALPHA-ORDER", "2026-09-01T08:00:00Z", channelOne, modelOne, "draft", "unpaid"),
		orderTwo:   create("BETA-ORDER", "2026-09-02T09:00:00Z", channelTwo, modelTwo, "confirmed", "paid"),
		orderThree: create("GAMMA-ORDER", "2026-09-03T10:00:00Z", channelOne, modelTwo, "cancelled", "refunded"),
	}
	archive := performRequest(t, router, http.MethodPut, "/sales-channels/"+channelOne.ID.String()+"/archive", nil)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive list fixture channel: %d %s", archive.Code, archive.Body.String())
	}
	return fixture
}

func getSalesOrderPage(t *testing.T, router *gin.Engine, path string) salesOrderPage {
	t.Helper()
	response := performRequest(t, router, http.MethodGet, path, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
	}
	var page salesOrderPage
	decodeBody(t, response, &page)
	return page
}

func TestSalesOrderListPaginationSortingAndHistoricalChannels(t *testing.T) {
	router, db := setupTestRouter(t)
	fixture := createSalesOrderListFixture(t, router, db)

	page := getSalesOrderPage(t, router, "/sales-orders?page=1&pageSize=2&sort=date&direction=desc")
	if page.Total != 3 || page.Page != 1 || page.PageSize != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected page metadata: %#v", page)
	}
	if page.Items[0].ID != fixture.orderThree.ID || page.Items[1].ID != fixture.orderTwo.ID {
		t.Fatalf("unexpected sort order: %#v", page.Items)
	}
	if page.Items[0].SalesChannel == nil || !page.Items[0].SalesChannel.IsArchived || len(page.Items[0].Items) != 1 {
		t.Fatalf("historical associations missing from list: %#v", page.Items[0])
	}

	secondPage := getSalesOrderPage(t, router, "/sales-orders?page=2&pageSize=2&sort=date&direction=desc")
	if secondPage.Total != 3 || len(secondPage.Items) != 1 || secondPage.Items[0].ID != fixture.orderOne.ID {
		t.Fatalf("unexpected second page: %#v", secondPage)
	}
	defaultPage := getSalesOrderPage(t, router, "/sales-orders")
	if defaultPage.Total != 3 || defaultPage.Page != 1 || defaultPage.PageSize != 25 || len(defaultPage.Items) != 3 {
		t.Fatalf("unexpected default page: %#v", defaultPage)
	}
}

func TestSalesOrderListSearchAndFilters(t *testing.T) {
	router, db := setupTestRouter(t)
	fixture := createSalesOrderListFixture(t, router, db)

	tests := []struct {
		name string
		path string
		ids  []uuid.UUID
	}{
		{"reference search", "/sales-orders?search=beta", []uuid.UUID{fixture.orderTwo.ID}},
		{"channel search", "/sales-orders?search=marketplace", []uuid.UUID{fixture.orderTwo.ID}},
		{"snapshot search", "/sales-orders?search=dragon", []uuid.UUID{fixture.orderOne.ID}},
		{"date range", "/sales-orders?date_from=2026-09-02&date_to=2026-09-02", []uuid.UUID{fixture.orderTwo.ID}},
		{"statuses", "/sales-orders?fulfilment_status=confirmed&payment_status=paid", []uuid.UUID{fixture.orderTwo.ID}},
		{"channel", "/sales-orders?sales_channel_id=" + fixture.channelOne.ID.String(), []uuid.UUID{fixture.orderOne.ID, fixture.orderThree.ID}},
		{"model", "/sales-orders?model_id=" + fixture.modelTwo.ID.String(), []uuid.UUID{fixture.orderTwo.ID, fixture.orderThree.ID}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := getSalesOrderPage(t, router, test.path+"&sort=date&direction=asc")
			if page.Total != int64(len(test.ids)) || len(page.Items) != len(test.ids) {
				t.Fatalf("unexpected filtered page: %#v", page)
			}
			for index, id := range test.ids {
				if page.Items[index].ID != id {
					t.Fatalf("unexpected filtered order at %d: got %s, want %s", index, page.Items[index].ID, id)
				}
			}
		})
	}
}

func TestSalesOrderListRejectsInvalidParameters(t *testing.T) {
	router, _ := setupTestRouter(t)
	paths := []string{
		"/sales-orders?page=0",
		"/sales-orders?pageSize=101",
		"/sales-orders?sort=reference%3BDROP",
		"/sales-orders?direction=sideways",
		"/sales-orders?date_from=not-a-date",
		"/sales-orders?date_from=2026-09-03&date_to=2026-09-02",
		"/sales-orders?fulfilment_status=unknown",
		"/sales-orders?payment_status=pending",
		"/sales-orders?sales_channel_id=not-a-uuid",
		"/sales-orders?model_id=00000000-0000-0000-0000-000000000000",
	}
	for _, path := range paths {
		response := performRequest(t, router, http.MethodGet, path, nil)
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want %d: %s", path, response.Code, http.StatusBadRequest, response.Body.String())
		}
	}
}
