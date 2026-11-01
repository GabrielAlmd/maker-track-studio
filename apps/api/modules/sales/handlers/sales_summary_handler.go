package saleshandlers

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const salesSummaryRecentLimit = 5

// GetSalesSummary returns all-time business metrics. Operational counters use
// active/non-cancelled Orders, while every financial field uses completed
// Orders and their frozen historical cost/profit snapshots only.
func GetSalesSummary(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := buildSalesSummary(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve Sales summary"})
			return
		}
		c.JSON(http.StatusOK, summary)
	}
}

func buildSalesSummary(db *gorm.DB) (salesdto.SalesSummaryDTO, error) {
	summary := salesdto.SalesSummaryDTO{
		Margin:       salesdto.SalesMarginDTO{State: "not_calculated", Reason: "Revenue is zero"},
		RecentOrders: make([]salesdto.SalesSummaryRecentOrderDTO, 0),
	}

	if err := db.Model(&salesmodels.SalesOrder{}).
		Where("fulfilment_status NOT IN ?", []salesmodels.SalesOrderFulfilmentStatus{
			salesmodels.SalesOrderFulfilmentCompleted,
			salesmodels.SalesOrderFulfilmentCancelled,
		}).Count(&summary.ActiveOrderCount).Error; err != nil {
		return salesdto.SalesSummaryDTO{}, err
	}
	if err := db.Model(&salesmodels.SalesOrder{}).
		Where("fulfilment_status = ?", salesmodels.SalesOrderFulfilmentCompleted).
		Count(&summary.CompletedOrderCount).Error; err != nil {
		return salesdto.SalesSummaryDTO{}, err
	}
	if err := db.Model(&salesmodels.SalesOrder{}).
		Where("payment_status = ? AND fulfilment_status <> ?",
			salesmodels.SalesOrderPaymentUnpaid, salesmodels.SalesOrderFulfilmentCancelled).
		Count(&summary.UnpaidOrderCount).Error; err != nil {
		return salesdto.SalesSummaryDTO{}, err
	}

	var completed []salesmodels.SalesOrder
	if err := db.
		Where("fulfilment_status = ?", salesmodels.SalesOrderFulfilmentCompleted).
		Preload("Items", func(tx *gorm.DB) *gorm.DB { return tx.Order("created_at ASC, id ASC") }).
		Find(&completed).Error; err != nil {
		return salesdto.SalesSummaryDTO{}, err
	}
	for _, order := range completed {
		profitability, err := calculateFrozenSalesOrderProfitability(order)
		if err != nil {
			return salesdto.SalesSummaryDTO{}, err
		}
		if profitability.State != "calculated" || profitability.ProductionCostCents == nil ||
			profitability.TotalCostCents == nil || profitability.NetProfitCents == nil {
			return salesdto.SalesSummaryDTO{}, errors.New("completed Sales Order snapshot is unavailable")
		}
		for target, value := range map[*int64]int64{
			&summary.RevenueCents:        profitability.RevenueCents,
			&summary.ProductionCostCents: *profitability.ProductionCostCents,
			&summary.PackagingCostCents:  order.PackagingCostCents,
			&summary.ShippingCostCents:   order.ShippingCostCents,
			&summary.PlatformFeeCents:    order.PlatformFeeCents,
			&summary.OtherCostCents:      order.OtherCostCents,
			&summary.TotalCostCents:      *profitability.TotalCostCents,
			&summary.NetProfitCents:      *profitability.NetProfitCents,
		} {
			next, ok := checkedAddInt64(*target, value)
			if !ok {
				return salesdto.SalesSummaryDTO{}, errSalesMoneyOverflow
			}
			*target = next
		}
	}
	if summary.RevenueCents > 0 {
		basisPoints, ok := salesMarginBasisPoints(summary.NetProfitCents, summary.RevenueCents)
		if !ok {
			return salesdto.SalesSummaryDTO{}, errSalesMoneyOverflow
		}
		percent := float64(basisPoints) / 100
		summary.Margin = salesdto.SalesMarginDTO{State: "calculated", Percent: &percent}
	}

	var recent []salesmodels.SalesOrder
	if err := db.
		Where("fulfilment_status <> ?", salesmodels.SalesOrderFulfilmentCancelled).
		Order("order_date DESC").Order("created_at DESC").Order("id DESC").
		Limit(salesSummaryRecentLimit).
		Preload("SalesChannel").
		Preload("Items", func(tx *gorm.DB) *gorm.DB { return tx.Order("created_at ASC, id ASC") }).
		Find(&recent).Error; err != nil {
		return salesdto.SalesSummaryDTO{}, err
	}
	for _, order := range recent {
		revenue, err := salesOrderRevenueCents(order)
		if err != nil {
			return salesdto.SalesSummaryDTO{}, err
		}
		item := salesdto.SalesSummaryRecentOrderDTO{
			ID: order.ID, Reference: order.Reference, OrderDate: order.OrderDate,
			FulfilmentStatus: order.FulfilmentStatus, PaymentStatus: order.PaymentStatus,
			RevenueCents: revenue,
		}
		if order.SalesChannel != nil {
			channel := salesChannelDTO(*order.SalesChannel)
			item.SalesChannel = &channel
		}
		summary.RecentOrders = append(summary.RecentOrders, item)
	}

	return summary, nil
}
