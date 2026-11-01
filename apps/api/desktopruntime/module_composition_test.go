package desktopruntime

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/routes"
	"github.com/gin-gonic/gin"
)

func TestProductionOnlyStartupAndPrimaryRoutes(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "production-only.db"))
	installed, err := modules.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := initSQLiteDBWithModules(nil, installed)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if db.Migrator().HasTable("sales_orders") {
		t.Fatal("Sales table initialized without the module")
	}
	model := models.Model{Name: "Production only", MaterialWeight: 20, PrintTime: 30}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	job := models.PrintJob{ModelID: &model.ID, PrintDate: time.Now(), UnitsPrinted: 1}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	routes.RegisterRoutesWithModules(router, db, installed)
	for _, check := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/models", http.StatusOK},
		{http.MethodGet, "/print-jobs", http.StatusOK},
		{http.MethodGet, "/features", http.StatusOK},
		{http.MethodGet, "/sales-orders", http.StatusNotFound},
		{http.MethodDelete, "/print-jobs/" + job.ID.String(), http.StatusNoContent},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(check.method, check.path, nil))
		if response.Code != check.want {
			t.Fatalf("%s %s: got %d, want %d: %s", check.method, check.path, response.Code, check.want, response.Body.String())
		}
	}
}

func TestMissingModuleCannotStartOnRetainedSalesData(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "retained-sales.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	order := salesmodels.SalesOrder{Reference: "RETAINED", OrderDate: time.Now(), FulfilmentStatus: salesmodels.SalesOrderFulfilmentDraft, PaymentStatus: salesmodels.SalesOrderPaymentUnpaid}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	empty, err := modules.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initSQLiteDBWithModules(nil, empty); err == nil {
		t.Fatal("production-only build accepted a database containing Sales data")
	}
	retained, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := retained.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var saved salesmodels.SalesOrder
	if err := retained.First(&saved, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Reference != order.Reference {
		t.Fatal("Sales data changed after incompatible startup")
	}
}
