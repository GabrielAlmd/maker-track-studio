// Package sales composes the Sales & Orders backend into the desktop app.
package sales

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	saleshandlers "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/handlers"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Module struct{}

var _ modules.Module = Module{}

func (Module) ID() string               { return "sales_orders" }
func (Module) Version() int             { return 1 }
func (Module) RequiresCoreVersion() int { return modules.CoreVersion }
func (Module) EnabledByDefault() bool   { return false }
func (Module) BackupTables() []string {
	return []string{"sales_channels", "sales_orders", "sales_order_items", "sales_order_allocations"}
}

func (Module) Migrate(tx *gorm.DB) error {
	return tx.AutoMigrate(
		&salesmodels.SalesChannel{},
		&salesmodels.SalesOrder{},
		&salesmodels.SalesOrderItem{},
		&salesmodels.SalesOrderAllocation{},
	)
}

func (Module) RegisterRoutes(router *gin.Engine, db *gorm.DB) {
	bind := func(factory func(*gorm.DB) gin.HandlerFunc) gin.HandlerFunc {
		return modules.WithRequestDB(db, factory)
	}
	router.GET("/sales-channels", bind(saleshandlers.GetSalesChannels))
	router.POST("/sales-channels", bind(saleshandlers.CreateSalesChannel))
	router.PUT("/sales-channels/:id", bind(saleshandlers.UpdateSalesChannel))
	router.PUT("/sales-channels/:id/archive", bind(saleshandlers.ArchiveSalesChannel))
	router.GET("/sales/summary", bind(saleshandlers.GetSalesSummary))
	router.GET("/sales-orders", bind(saleshandlers.GetSalesOrders))
	router.GET("/sales-orders/:id", bind(saleshandlers.GetSalesOrder))
	router.GET("/sales-orders/:id/allocations", bind(saleshandlers.GetSalesOrderAllocations))
	router.POST("/sales-orders", bind(saleshandlers.CreateSalesOrder))
	router.PUT("/sales-orders/:id", bind(saleshandlers.UpdateSalesOrder))
	router.PUT("/sales-orders/:id/status", bind(saleshandlers.UpdateSalesOrderStatus))
	router.PUT("/sales-orders/:id/allocations", bind(saleshandlers.ReplaceSalesOrderAllocations))
	router.DELETE("/sales-orders/:id", bind(saleshandlers.DeleteSalesOrder))
	router.GET("/print-jobs/:id/allocations", bind(saleshandlers.GetPrintJobAllocations))
}
