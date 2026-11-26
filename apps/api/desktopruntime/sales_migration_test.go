package desktopruntime

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
)

func TestSalesMigrationCreatesTablesAndPreservesProductionData(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "pre-sales.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []any{
		&salesmodels.SalesProductionPlan{},
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
		&salesmodels.SalesProductionPlan{},
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

func TestProductionPlanMigrationRetainsLegacyAllocation(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "sales-module-v4.db"))
	db, err := initSQLiteDB()
	if err != nil {
		t.Fatal(err)
	}
	order := salesmodels.SalesOrder{
		Reference: "Legacy completed order", OrderDate: time.Now(),
		FulfilmentStatus: salesmodels.SalesOrderFulfilmentCompleted, PaymentStatus: salesmodels.SalesOrderPaymentPaid,
		ProductionCostCents: new(int64(145)), TotalCostCents: new(int64(145)),
		NetProfitCents: new(int64(605)), MarginBasisPoints: new(int64(8067)),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	line := salesmodels.SalesOrderItem{
		SalesOrderID: order.ID, ModelNameSnapshot: "Legacy enclosure", Quantity: 3, UnitPriceCents: 250,
	}
	if err := db.Create(&line).Error; err != nil {
		t.Fatal(err)
	}
	job := models.PrintJob{PrintDate: time.Now(), UnitsPrinted: 3}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	allocation := salesmodels.SalesOrderAllocation{
		SalesOrderItemID: line.ID, PrintJobID: &job.ID, UnitsAllocated: 3, ProductionCostCents: new(int64(145)),
	}
	if err := db.Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}

	// Reconstruct the relevant Sales module v4 schema around the historical rows.
	for _, statement := range []string{
		"DROP TABLE sales_production_plans",
		"DROP INDEX idx_sales_allocation_line_run_item",
		"DROP INDEX idx_sales_order_allocations_print_run_item_id",
		"ALTER TABLE sales_order_allocations DROP COLUMN print_run_item_id",
		"ALTER TABLE sales_orders DROP COLUMN production_revision",
		"ALTER TABLE sales_orders DROP COLUMN quick_sale_cents",
		"ALTER TABLE business_order_revisions DROP COLUMN customer_name",
		"ALTER TABLE business_order_revisions DROP COLUMN quick_sale_pricing",
		"UPDATE module_versions SET version = 4 WHERE id = 'sales_orders'",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		migrated, err := initSQLiteDB()
		if err != nil {
			t.Fatal(err)
		}
		if !migrated.Migrator().HasTable(&salesmodels.SalesProductionPlan{}) ||
			!migrated.Migrator().HasIndex(&salesmodels.SalesOrderAllocation{}, "idx_sales_allocation_line_run_item") {
			t.Fatal("missing production plan schema or allocation uniqueness constraint")
		}
		if !migrated.Migrator().HasColumn("business_order_revisions", "quick_sale_pricing") {
			t.Fatal("missing nullable Quick Sale pricing column")
		}
		var saved salesmodels.SalesOrderAllocation
		if err := migrated.Take(&saved, "id = ?", allocation.ID).Error; err != nil {
			t.Fatal(err)
		}
		if saved.SalesOrderItemID != line.ID || saved.PrintJobID == nil || *saved.PrintJobID != job.ID ||
			saved.PrintRunItemID != nil || saved.ProductionCostCents == nil || *saved.ProductionCostCents != 145 ||
			saved.UnitsAllocated != allocation.UnitsAllocated || !saved.CreatedAt.Equal(allocation.CreatedAt) {
			t.Fatalf("historical allocation changed during migration: %+v", saved)
		}
		var savedOrder salesmodels.SalesOrder
		if err := migrated.Take(&savedOrder, "id = ?", order.ID).Error; err != nil {
			t.Fatal(err)
		}
		if savedOrder.IsV2() || savedOrder.ProductionRevision != 0 ||
			savedOrder.FulfilmentStatus != order.FulfilmentStatus || savedOrder.PaymentStatus != order.PaymentStatus ||
			savedOrder.ProductionCostCents == nil || *savedOrder.ProductionCostCents != 145 ||
			savedOrder.TotalCostCents == nil || *savedOrder.TotalCostCents != 145 ||
			savedOrder.NetProfitCents == nil || *savedOrder.NetProfitCents != 605 ||
			savedOrder.MarginBasisPoints == nil || *savedOrder.MarginBasisPoints != 8067 {
			t.Fatalf("legacy order changed during migration: %+v", savedOrder)
		}
		var savedLine salesmodels.SalesOrderItem
		if err := migrated.Take(&savedLine, "id = ?", line.ID).Error; err != nil {
			t.Fatal(err)
		}
		if savedLine.SalesOrderID != order.ID || savedLine.UnitPriceCents != line.UnitPriceCents || savedLine.Quantity != line.Quantity ||
			savedLine.ModelNameSnapshot != line.ModelNameSnapshot {
			t.Fatalf("legacy price or line changed during migration: %+v", savedLine)
		}
		connection, err := migrated.DB()
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
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
	for _, table := range budget.Tables() {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE module_versions SET version = 1 WHERE id = ?", "sales_orders").Error; err != nil {
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
	for _, table := range budget.Tables() {
		if !migrated.Migrator().HasTable(table) {
			t.Fatalf("Sales module did not create %T", table)
		}
	}
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
