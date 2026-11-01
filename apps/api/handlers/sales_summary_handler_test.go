package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type summaryOrderInput struct {
	reference       string
	date            time.Time
	fulfilment      salesmodels.SalesOrderFulfilmentStatus
	payment         salesmodels.SalesOrderPaymentStatus
	revenue         int64
	production      int64
	packaging       int64
	shipping        int64
	platform        int64
	other           int64
	frozenTotal     int64
	frozenNetProfit int64
}

func createSummaryOrder(t *testing.T, db *gorm.DB, channel salesmodels.SalesChannel, input summaryOrderInput) salesmodels.SalesOrder {
	t.Helper()
	production, total, net := input.production, input.frozenTotal, input.frozenNetProfit
	order := salesmodels.SalesOrder{
		Reference: input.reference, OrderDate: input.date, SalesChannelID: &channel.ID,
		FulfilmentStatus: input.fulfilment, PaymentStatus: input.payment,
		PackagingCostCents: input.packaging, ShippingCostCents: input.shipping,
		PlatformFeeCents: input.platform, OtherCostCents: input.other,
	}
	if input.fulfilment == salesmodels.SalesOrderFulfilmentCompleted {
		order.ProductionCostCents = &production
		order.TotalCostCents = &total
		order.NetProfitCents = &net
		if input.revenue > 0 {
			basisPoints, ok := testMarginBasisPoints(input.frozenNetProfit, input.revenue)
			if !ok {
				t.Fatal("invalid summary fixture margin")
			}
			order.MarginBasisPoints = &basisPoints
		}
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&salesmodels.SalesOrderItem{
		SalesOrderID: order.ID, ModelNameSnapshot: input.reference,
		Quantity: 1, UnitPriceCents: input.revenue,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

func testMarginBasisPoints(netProfit, revenue int64) (int64, bool) {
	if revenue <= 0 {
		return 0, false
	}
	return netProfit * 10000 / revenue, true
}

func TestSalesSummarySeparatesOperationalAndFrozenFinancialMetrics(t *testing.T) {
	router, db := setupTestRouter(t)
	channel := salesmodels.SalesChannel{Name: "Online shop"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	createSummaryOrder(t, db, channel, summaryOrderInput{
		reference: "ACTIVE", date: base.Add(24 * time.Hour),
		fulfilment: salesmodels.SalesOrderFulfilmentPreparing, payment: salesmodels.SalesOrderPaymentUnpaid,
		revenue: 9000, packaging: 900, shipping: 900, platform: 900, other: 900,
	})
	completed := createSummaryOrder(t, db, channel, summaryOrderInput{
		reference: "COMPLETED", date: base.Add(48 * time.Hour),
		fulfilment: salesmodels.SalesOrderFulfilmentCompleted, payment: salesmodels.SalesOrderPaymentPaid,
		revenue: 2000, production: 500, packaging: 100, shipping: 150, platform: 100, other: 50,
		frozenTotal: 900, frozenNetProfit: 1100,
	})
	zeroRevenue := createSummaryOrder(t, db, channel, summaryOrderInput{
		reference: "COMPLETED-ZERO", date: base.Add(72 * time.Hour),
		fulfilment: salesmodels.SalesOrderFulfilmentCompleted, payment: salesmodels.SalesOrderPaymentUnpaid,
		production: 0, packaging: 25, shipping: 25, platform: 25, other: 25,
		frozenTotal: 100, frozenNetProfit: -100,
	})
	createSummaryOrder(t, db, channel, summaryOrderInput{
		reference: "CANCELLED", date: base.Add(96 * time.Hour),
		fulfilment: salesmodels.SalesOrderFulfilmentCancelled, payment: salesmodels.SalesOrderPaymentUnpaid,
		revenue: 999999, packaging: 999999, shipping: 999999, platform: 999999, other: 999999,
	})

	// If the endpoint used mutable production data instead of snapshots, these
	// completed Orders have no allocations and would produce a different total.
	response := performRequest(t, router, http.MethodGet, "/sales/summary", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("sales summary: %d %s", response.Code, response.Body.String())
	}
	var summary salesdto.SalesSummaryDTO
	decodeBody(t, response, &summary)
	if summary.ActiveOrderCount != 1 || summary.CompletedOrderCount != 2 || summary.UnpaidOrderCount != 2 {
		t.Fatalf("unexpected operational counts: %#v", summary)
	}
	if summary.RevenueCents != 2000 || summary.ProductionCostCents != 500 ||
		summary.PackagingCostCents != 125 || summary.ShippingCostCents != 175 ||
		summary.PlatformFeeCents != 125 || summary.OtherCostCents != 75 ||
		summary.TotalCostCents != 1000 || summary.NetProfitCents != 1000 {
		t.Fatalf("financial totals did not use completed snapshots: %#v", summary)
	}
	if summary.Margin.State != "calculated" || summary.Margin.Percent == nil || *summary.Margin.Percent != 50 {
		t.Fatalf("unexpected aggregate margin: %#v", summary.Margin)
	}
	if len(summary.RecentOrders) != 3 || summary.RecentOrders[0].ID != zeroRevenue.ID ||
		summary.RecentOrders[1].ID != completed.ID || summary.RecentOrders[0].SalesChannel == nil ||
		summary.RecentOrders[0].SalesChannel.Name != channel.Name {
		t.Fatalf("unexpected recent Orders: %#v", summary.RecentOrders)
	}
	for _, order := range summary.RecentOrders {
		if order.Reference == "CANCELLED" {
			t.Fatalf("cancelled Order appeared in recent operational Orders: %#v", summary.RecentOrders)
		}
	}

	// The presentation preference never gates the Sales API.
	disabled := performRequest(t, router, http.MethodPut, "/features", map[string]any{"sales_orders": false})
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable Sales feature: %d %s", disabled.Code, disabled.Body.String())
	}
	if response = performRequest(t, router, http.MethodGet, "/sales/summary", nil); response.Code != http.StatusOK {
		t.Fatalf("feature state gated summary API: %d %s", response.Code, response.Body.String())
	}
}

func TestSalesSummaryZeroRevenueMarginIsNotCalculated(t *testing.T) {
	router, db := setupTestRouter(t)
	channel := salesmodels.SalesChannel{Name: "Direct"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	createSummaryOrder(t, db, channel, summaryOrderInput{
		reference: "FREE", date: time.Now(), fulfilment: salesmodels.SalesOrderFulfilmentCompleted,
		payment: salesmodels.SalesOrderPaymentPaid, packaging: 50, frozenTotal: 50, frozenNetProfit: -50,
	})

	response := performRequest(t, router, http.MethodGet, "/sales/summary", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("zero-revenue summary: %d %s", response.Code, response.Body.String())
	}
	var summary salesdto.SalesSummaryDTO
	decodeBody(t, response, &summary)
	if summary.Margin.State != "not_calculated" || summary.Margin.Percent != nil || summary.Margin.Reason == "" {
		t.Fatalf("zero-revenue margin is invalid: %#v", summary.Margin)
	}
	if strings.Contains(response.Body.String(), "NaN") || strings.Contains(response.Body.String(), "Infinity") {
		t.Fatalf("response contains a non-finite margin: %s", response.Body.String())
	}
}

func TestSalesSummaryRejectsCompletedOrderWithoutFrozenSnapshot(t *testing.T) {
	router, db := setupTestRouter(t)
	channel := salesmodels.SalesChannel{Name: "Legacy"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	order := salesmodels.SalesOrder{
		Reference: "LEGACY", OrderDate: time.Now(), SalesChannelID: &channel.ID,
		FulfilmentStatus: salesmodels.SalesOrderFulfilmentCompleted, PaymentStatus: salesmodels.SalesOrderPaymentPaid,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&salesmodels.SalesOrderItem{SalesOrderID: order.ID, ModelNameSnapshot: "Legacy", Quantity: 1, UnitPriceCents: 100}).Error; err != nil {
		t.Fatal(err)
	}

	response := performRequest(t, router, http.MethodGet, "/sales/summary", nil)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("missing completed snapshot: %d %s", response.Code, response.Body.String())
	}
}
