package routes

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/appmodules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	RegisterRoutesWithModules(r, db, appmodules.Default())
}

// RegisterRoutesWithModules registers core routes and the installed modules.
// An empty registry is valid for production-only composition.
func RegisterRoutesWithModules(r *gin.Engine, db *gorm.DB, installed *modules.Registry, events ...*diagnostics.Logger) {
	var recent *diagnostics.Logger
	if len(events) > 0 {
		recent = events[0]
	}
	// Local backup and recovery routes.
	r.GET("/desktop-data/export", withRequestDB(db, handlers.GetDesktopDataExport))
	r.GET("/reports/export", withRequestDB(db, func(requestDB *gorm.DB) gin.HandlerFunc { return handlers.GetReportExport(requestDB, recent) }))
	r.POST("/desktop-data/import", withRequestDB(db, func(requestDB *gorm.DB) gin.HandlerFunc {
		return handlers.ImportDesktopData(requestDB, installed)
	}))
	r.GET("/data-migration/schema", withRequestDB(db, handlers.GetDataMigrationSchema))
	r.GET("/data-migration/templates/:section", withRequestDB(db, handlers.GetDataMigrationTemplate))
	r.GET("/data-migration/workbook-template", withRequestDB(db, handlers.GetDataMigrationWorkbookTemplate))
	r.POST("/data-migration/inspect", withRequestDB(db, handlers.InspectDataMigration))
	r.POST("/data-migration/preview", withRequestDB(db, handlers.PreviewDataMigration))
	r.POST("/data-migration/import", withRequestDB(db, handlers.ImportDataMigration))

	// Physical printers. Archive instead of deleting stable machine identities.
	r.GET("/printers", withRequestDB(db, handlers.GetPrinters))
	r.GET("/printers/:id", withRequestDB(db, handlers.GetPrinter))
	r.POST("/printers", withRequestDB(db, handlers.CreatePrinter))
	r.PUT("/printers/:id", withRequestDB(db, handlers.UpdatePrinter))
	r.PUT("/printers/:id/default", withRequestDB(db, handlers.SetPrinterDefault))
	r.PUT("/printers/:id/archive", withRequestDB(db, handlers.ArchivePrinter))
	r.PUT("/printers/:id/restore", withRequestDB(db, handlers.RestorePrinter))
	r.PUT("/printers/:id/cost-config", withRequestDB(db, handlers.UpdatePrinterCostConfig))

	// Filament type catalog
	r.GET("/filament-types", withRequestDB(db, handlers.GetFilamentTypes))
	r.PUT("/filament-types", withRequestDB(db, handlers.UpdateFilamentTypes))

	// Filament routes
	r.GET("/filament-brands", withRequestDB(db, handlers.GetFilamentBrands))
	r.GET("/filaments", withRequestDB(db, handlers.GetFilaments))
	r.GET("/filaments/:id", withRequestDB(db, handlers.GetFilament))
	r.POST("/filaments", withRequestDB(db, handlers.CreateFilament))
	r.PUT("/filaments/:id", withRequestDB(db, handlers.UpdateFilament))
	r.DELETE("/filaments/:id", withRequestDB(db, handlers.DeleteFilament))

	// FilamentPurchase routes
	r.GET("/filament-purchases", withRequestDB(db, handlers.GetFilamentPurchases))
	r.GET("/filament-purchases/:id", withRequestDB(db, handlers.GetFilamentPurchase))
	r.POST("/filament-purchases", withRequestDB(db, handlers.CreateFilamentPurchase))
	r.POST("/filament-purchases/batch", withRequestDB(db, handlers.CreateFilamentPurchaseBatch))
	r.PUT("/filament-purchases/:id", withRequestDB(db, handlers.UpdateFilamentPurchase))
	r.DELETE("/filament-purchases/:id", withRequestDB(db, handlers.DeleteFilamentPurchase))

	// Model routes
	r.GET("/models", withRequestDB(db, handlers.GetModels))
	r.GET("/models/:id", withRequestDB(db, handlers.GetModel))
	r.POST("/models", withRequestDB(db, handlers.CreateModel))
	r.PUT("/models/:id", withRequestDB(db, handlers.UpdateModel))
	r.DELETE("/models/:id", withRequestDB(db, handlers.DeleteModel))

	// PrintJob routes
	r.GET("/print-jobs", withRequestDB(db, handlers.GetPrintJobs))
	r.GET("/print-jobs/dashboard-summary", withRequestDB(db, handlers.GetDashboardSummary))
	r.GET("/print-jobs/stats", withRequestDB(db, handlers.GetPrintJobStats))
	r.GET("/print-jobs/:id", withRequestDB(db, handlers.GetPrintJob))
	r.GET("/print-jobs/:id/cost", withRequestDB(db, handlers.GetPrintJobCost))
	r.POST("/print-jobs", withRequestDB(db, handlers.CreatePrintJob))
	r.PUT("/print-jobs/:id", withRequestDB(db, handlers.UpdatePrintJob))
	r.DELETE("/print-jobs/:id", withRequestDB(db, func(requestDB *gorm.DB) gin.HandlerFunc {
		return handlers.DeletePrintJob(requestDB, installed)
	}))

	// PrintJobFilament routes
	r.GET("/print-job-filaments", withRequestDB(db, handlers.GetPrintJobsFilament))
	r.GET("/print-job-filaments/:id", withRequestDB(db, handlers.GetPrintJobFilament))
	r.POST("/print-job-filaments", withRequestDB(db, handlers.CreatePrintJobFilament))
	r.PUT("/print-job-filaments/:id", withRequestDB(db, handlers.UpdatePrintJobFilament))
	r.DELETE("/print-job-filaments/:id", withRequestDB(db, handlers.DeletePrintJobFilament))

	// AppConfig routes
	r.GET("/features", withRequestDB(db, func(requestDB *gorm.DB) gin.HandlerFunc {
		return handlers.GetFeatures(requestDB, installed)
	}))
	r.PUT("/features", withRequestDB(db, func(requestDB *gorm.DB) gin.HandlerFunc {
		return handlers.UpdateFeatures(requestDB, installed)
	}))
	r.GET("/app-config", withRequestDB(db, handlers.GetAppConfig))
	r.GET("/app-config/:key", withRequestDB(db, handlers.GetAppConfigByKey))
	r.POST("/app-config/:key", withRequestDB(db, handlers.CreateAppConfig))
	r.PUT("/app-config/:key", withRequestDB(db, handlers.UpdateAppConfig))
	r.DELETE("/app-config/:key", withRequestDB(db, handlers.DeleteAppConfig))
	installed.RegisterRoutes(r, db)
}

// Bind each handler to the request context so database diagnostics share the
// server-generated correlation ID without mutating the shared GORM handle.
func withRequestDB(db *gorm.DB, factory func(*gorm.DB) gin.HandlerFunc) gin.HandlerFunc {
	return modules.WithRequestDB(db, factory)
}
