package desktopruntime

import (
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"path/filepath"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

func TestSalesMigrationCreatesTablesAndPreservesProductionData(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "pre-sales.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []any{
		&salesmodels.SalesOrderAllocation{},
		&salesmodels.SalesOrderItem{},
		&salesmodels.SalesOrder{},
		&salesmodels.SalesChannel{},
	} {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}

	productionModel := models.Model{
		Name: "Existing model", MaterialWeight: 42, PrintTime: 90,
	}
	if err := db.Create(&productionModel).Error; err != nil {
		t.Fatal(err)
	}
	printJob := models.PrintJob{
		ModelID: &productionModel.ID, PrintDate: time.Now(), UnitsPrinted: 2,
	}
	if err := db.Create(&printJob).Error; err != nil {
		t.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := migrated.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	for _, table := range []any{
		&salesmodels.SalesChannel{},
		&salesmodels.SalesOrder{},
		&salesmodels.SalesOrderItem{},
		&salesmodels.SalesOrderAllocation{},
	} {
		if !migrated.Migrator().HasTable(table) {
			t.Fatalf("sales table %T was not created", table)
		}
	}

	for _, column := range []struct {
		model any
		name  string
	}{
		{&salesmodels.SalesOrder{}, "production_cost_cents"},
		{&salesmodels.SalesOrder{}, "total_cost_cents"},
		{&salesmodels.SalesOrder{}, "net_profit_cents"},
		{&salesmodels.SalesOrder{}, "margin_basis_points"},
		{&salesmodels.SalesOrderAllocation{}, "production_cost_cents"},
	} {
		if !migrated.Migrator().HasColumn(column.model, column.name) {
			t.Fatalf("Sales snapshot column %s was not created", column.name)
		}
	}

	var savedModel models.Model
	if err := migrated.First(&savedModel, "id = ?", productionModel.ID).Error; err != nil {
		t.Fatal(err)
	}
	if savedModel.Name != productionModel.Name {
		t.Fatalf("existing model changed during migration: %#v", savedModel)
	}
	var savedJob models.PrintJob
	if err := migrated.First(&savedJob, "id = ?", printJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if savedJob.UnitsPrinted != printJob.UnitsPrinted || savedJob.ModelID == nil || *savedJob.ModelID != productionModel.ID {
		t.Fatalf("existing print job changed during migration: %#v", savedJob)
	}
}

func TestSalesModelsGenerateStableUUIDs(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "sales-uuids.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}

	productionModel := models.Model{Name: "Sold model", MaterialWeight: 20, PrintTime: 30}
	if err := db.Create(&productionModel).Error; err != nil {
		t.Fatal(err)
	}
	printJob := models.PrintJob{ModelID: &productionModel.ID, PrintDate: time.Now(), UnitsPrinted: 1}
	if err := db.Create(&printJob).Error; err != nil {
		t.Fatal(err)
	}
	channel := salesmodels.SalesChannel{Name: "Direct"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	order := salesmodels.SalesOrder{
		Reference: "ORDER-1", OrderDate: time.Now(), SalesChannelID: &channel.ID,
		FulfilmentStatus: salesmodels.SalesOrderFulfilmentDraft,
		PaymentStatus:    salesmodels.SalesOrderPaymentUnpaid,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	item := salesmodels.SalesOrderItem{
		SalesOrderID: order.ID, ModelID: &productionModel.ID,
		ModelNameSnapshot: productionModel.Name, Quantity: 1, UnitPriceCents: 1299,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	allocation := salesmodels.SalesOrderAllocation{
		SalesOrderItemID: item.ID, PrintJobID: &printJob.ID, UnitsAllocated: 1,
	}
	if err := db.Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]uuid.UUID{
		"channel": channel.ID, "order": order.ID, "item": item.ID, "allocation": allocation.ID,
	} {
		if id == uuid.Nil {
			t.Fatalf("%s UUID was not generated", name)
		}
	}
}

func TestSalesModuleMigrationPreservesCompletedSnapshot(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "existing-sales.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	productionCost, totalCost, netProfit, margin := int64(250), int64(300), int64(700), int64(7000)
	order := salesmodels.SalesOrder{
		Reference: "EXISTING-1", OrderDate: time.Now(),
		FulfilmentStatus:    salesmodels.SalesOrderFulfilmentCompleted,
		PaymentStatus:       salesmodels.SalesOrderPaymentPaid,
		ProductionCostCents: &productionCost,
		TotalCostCents:      &totalCost,
		NetProfitCents:      &netProfit,
		MarginBasisPoints:   &margin,
	}
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

	migrated, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := migrated.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var saved salesmodels.SalesOrder
	if err := migrated.First(&saved, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ID != order.ID || saved.ProductionCostCents == nil || *saved.ProductionCostCents != productionCost ||
		saved.TotalCostCents == nil || *saved.TotalCostCents != totalCost ||
		saved.NetProfitCents == nil || *saved.NetProfitCents != netProfit ||
		saved.MarginBasisPoints == nil || *saved.MarginBasisPoints != margin {
		t.Fatalf("completed snapshot changed during module migration: %#v", saved)
	}
}
