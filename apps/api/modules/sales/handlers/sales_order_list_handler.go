package saleshandlers

import (
	corehandlers "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func GetSalesOrders(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, valid := corehandlers.ParseListParams(c, map[string]string{
			"date":              "sales_orders.order_date",
			"reference":         "sales_orders.reference",
			"created_at":        "sales_orders.created_at",
			"updated_at":        "sales_orders.updated_at",
			"fulfilment_status": "sales_orders.fulfilment_status",
			"payment_status":    "sales_orders.payment_status",
		}, "sales_orders.order_date")
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid list parameters"})
			return
		}
		// Sales has no legacy unpaged contract. A request without parameters
		// still receives a bounded server-side page.
		if !params.Enabled {
			params = corehandlers.ListParams{
				Enabled: true, Page: 1, PageSize: 25,
				Sort: "sales_orders.order_date", Direction: "asc",
			}
		}

		query := db.Model(&salesmodels.SalesOrder{})
		if search := strings.TrimSpace(c.Query("search")); search != "" {
			value := "%" + strings.ToLower(search) + "%"
			matchingChannel := db.Model(&salesmodels.SalesChannel{}).
				Select("1").
				Where("sales_channels.id = sales_orders.sales_channel_id AND LOWER(sales_channels.name) LIKE ?", value)
			matchingItem := db.Model(&salesmodels.SalesOrderItem{}).
				Select("1").
				Where("sales_order_items.sales_order_id = sales_orders.id AND LOWER(sales_order_items.model_name_snapshot) LIKE ?", value)
			query = query.Where(
				"LOWER(sales_orders.reference) LIKE ? OR LOWER(sales_orders.notes) LIKE ? OR EXISTS (?) OR EXISTS (?)",
				value, value, matchingChannel, matchingItem,
			)
		}
		var fromDate, toDate time.Time
		if raw := c.Query("date_from"); raw != "" {
			value, err := time.Parse("2006-01-02", raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date filter"})
				return
			}
			fromDate = value
		}
		if raw := c.Query("date_to"); raw != "" {
			value, err := time.Parse("2006-01-02", raw)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date filter"})
				return
			}
			toDate = value
		}
		if !fromDate.IsZero() && !toDate.IsZero() && fromDate.After(toDate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date range"})
			return
		}
		if !fromDate.IsZero() {
			query = query.Where("sales_orders.order_date >= ?", fromDate)
		}
		if !toDate.IsZero() {
			query = query.Where("sales_orders.order_date < ?", toDate.Add(24*time.Hour))
		}
		if raw := c.Query("fulfilment_status"); raw != "" {
			status := salesmodels.SalesOrderFulfilmentStatus(raw)
			if !status.Valid() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fulfilment status filter"})
				return
			}
			query = query.Where("sales_orders.fulfilment_status = ?", status)
		}
		if raw := c.Query("payment_status"); raw != "" {
			status := salesmodels.SalesOrderPaymentStatus(raw)
			if !status.Valid() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment status filter"})
				return
			}
			query = query.Where("sales_orders.payment_status = ?", status)
		}
		for key := range map[string]struct{}{"sales_channel_id": {}, "model_id": {}} {
			if raw := c.Query(key); raw != "" {
				id, err := uuid.Parse(raw)
				if err != nil || id == uuid.Nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reference filter"})
					return
				}
				if key == "sales_channel_id" {
					query = query.Where("sales_orders.sales_channel_id = ?", id)
				} else {
					matchingItem := db.Model(&salesmodels.SalesOrderItem{}).
						Select("1").
						Where("sales_order_items.sales_order_id = sales_orders.id AND sales_order_items.model_id = ?", id)
					query = query.Where("EXISTS (?)", matchingItem)
				}
			}
		}

		var total int64
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve Sales Orders"})
			return
		}
		var orders []salesmodels.SalesOrder
		if err := query.
			Order(params.Sort+" "+params.Direction).
			Order("sales_orders.id ASC").
			Offset((params.Page-1)*params.PageSize).
			Limit(params.PageSize).
			Preload("SalesChannel").
			Preload("Items", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("created_at ASC, id ASC")
			}).
			Preload("Items.Allocations", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("created_at ASC, id ASC")
			}).
			Preload("Items.Allocations.PrintJob.Model").
			Preload("Items.Allocations.PrintJob.Filaments.Filament").
			Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve Sales Orders"})
			return
		}

		var cfg map[string]float64
		for _, order := range orders {
			if order.FulfilmentStatus != salesmodels.SalesOrderFulfilmentCompleted {
				loadedConfig, configErr := corehandlers.LoadProductionCostConfig(db)
				if configErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate Sales Order profitability"})
					return
				}
				cfg = loadedConfig
				break
			}
		}
		result := make([]salesdto.SalesOrderDTO, 0, len(orders))
		for _, order := range orders {
			profitability, err := calculateSalesOrderProfitability(order, cfg)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate Sales Order profitability"})
				return
			}
			result = append(result, salesOrderDTO(order, profitability))
		}
		c.JSON(http.StatusOK, corehandlers.PageResponse[salesdto.SalesOrderDTO]{
			Items: result, Total: total, Page: params.Page, PageSize: params.PageSize,
		})
	}
}
