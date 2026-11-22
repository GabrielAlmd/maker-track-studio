package budget

import (
	"errors"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const quickSaleMaxQuantity = 1_000_000_000

type QuickSaleCommand struct {
	Command
	Reference      string    `json:"reference"`
	ModelID        uuid.UUID `json:"model_id"`
	CustomerID     uuid.UUID `json:"customer_id"`
	StockRunItemID uuid.UUID `json:"stock_run_item_id"`
	Quantity       int64     `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
}

func quickSaleStock(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		model, err := pathID(c)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		quantity, err := positiveQuery(c, "quantity", 1, quickSaleMaxQuantity)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		sources := storage.ProductionSources(db).Where("item.model_id = ? AND run.status = ?", model, "completed")
		query := db.Table("(?) AS sources", sources).
			Select("sources.print_run_item_id, sources.print_date, MIN(sources.units_available, lots.on_hand - lots.reserved) AS units_available").
			Joins("JOIN (?) AS lots ON lots.source_key = 'output:' || sources.print_run_item_id", inventory.VerifiedOutputBalances(db, model)).
			Where("sources.units_available >= ? AND lots.on_hand - lots.reserved >= ?", quantity, quantity).
			Order("sources.print_date DESC, sources.print_run_item_id")
		paginate[quickSaleLot](c, query)
	}
}

type quickSaleLot struct {
	PrintRunItemID uuid.UUID `json:"print_run_item_id"`
	PrintDate      time.Time `json:"print_date"`
	UnitsAvailable int64     `json:"units_available"`
}

func createQuickSale(db *gorm.DB, id uuid.UUID, input QuickSaleCommand) (OrderRevision, error) {
	return execute(db, "order", id, input.Command, input, func(tx *gorm.DB) (OrderRevision, uuid.UUID, int64, error) {
		var terms OrderRevision
		features, err := models.LoadFeatureConfig(tx)
		if err != nil {
			return terms, id, 0, err
		}
		if !features["sales_orders"] {
			return terms, id, 0, conflict("business_disabled", "Enable Business Operations before creating a Quick Sale")
		}
		reference := strings.TrimSpace(input.Reference)
		if !validText(reference, 1, 160) || input.ModelID == uuid.Nil {
			return terms, id, 0, invalid("sale", "Provide a reference of up to 160 characters and select a model")
		}
		if input.Quantity < 1 || input.Quantity > quickSaleMaxQuantity || input.UnitPriceCents < 1 || input.UnitPriceCents > maxSafeInteger/input.Quantity {
			return terms, id, 0, invalid("price", "Provide a positive quantity and price within the supported range")
		}
		var model models.Model
		if err := tx.Take(&model, "id = ?", input.ModelID).Error; err != nil {
			return terms, id, 0, missing(err, "model_id", "The selected model was not found")
		}
		if input.CustomerID != uuid.Nil {
			customer, err := readRevision[Customer](tx, input.CustomerID, 0)
			if err != nil {
				return terms, id, 0, err
			}
			if customer.IsArchived {
				return terms, id, 0, conflict("archived_customer", "Select an active customer")
			}
			terms.CustomerID, terms.CustomerName = customer.ID, customer.Name
		}

		now := time.Now().UTC()
		payable := input.Quantity * input.UnitPriceCents
		order := storage.SalesOrder{Reference: reference, OrderDate: now, QuickSaleCents: &payable}
		terms.ConfirmedAt, terms.CreatedAt = now, now
		line := storage.SalesOrderItem{ModelID: &model.ID, ModelNameSnapshot: model.Name, Quantity: int(input.Quantity), UnitPriceCents: input.UnitPriceCents}
		if input.StockRunItemID != uuid.Nil {
			order.ProductionRevision = 1
		}
		lines := []storage.SalesOrderItem{line}
		if err := saveConfirmedOrder(tx, &order, lines, &terms); err != nil {
			return terms, id, 0, err
		}
		if input.StockRunItemID != uuid.Nil {
			if err := dispatchQuickSale(tx, input, lines[0]); err != nil {
				return terms, id, 0, err
			}
		}
		return terms, terms.ID, terms.Revision, nil
	})
}

func dispatchQuickSale(tx *gorm.DB, input QuickSaleCommand, line storage.SalesOrderItem) error {
	stockConflict := conflict("stock_unavailable", "The selected lot has insufficient available stock. Choose another lot or change the quantity.")
	var sources []salesdto.ProductionSource
	err := storage.ProductionSources(tx).
		Where("item.id = ? AND item.model_id = ? AND run.status = ?", input.StockRunItemID, input.ModelID, "completed").
		Scan(&sources).Error
	if err != nil {
		return err
	}
	if len(sources) != 1 || sources[0].UnitsAvailable < input.Quantity {
		return stockConflict
	}
	balance, err := inventory.OutputAvailability(tx, input.ModelID, input.StockRunItemID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return stockConflict
	}
	if err != nil {
		return err
	}
	if balance.Available < input.Quantity {
		return stockConflict
	}
	source := sources[0]
	allocation := storage.SalesOrderAllocation{
		SalesOrderItemID: line.ID, PrintRunItemID: &source.PrintRunItemID,
		PrintJobID: &source.PrintJobID, UnitsAllocated: line.Quantity,
	}
	if err := tx.Create(&allocation).Error; err != nil {
		return err
	}
	reservation := uuid.NewSHA1(input.OperationID, []byte("quick-sale-reserve"))
	command := inventory.FulfilmentCommand{
		OperationID: reservation, SourceID: reservation, ProductID: input.ModelID, RunItemID: input.StockRunItemID,
		Reference: allocation.ID, Kind: "reserve", Reason: "Quick Sale", Actor: storage.FulfilmentActor,
		Units: input.Quantity, ExpectedRevision: balance.Revision,
	}
	receipt, err := inventory.ApplyFulfilment(tx, command)
	if err != nil {
		return quickSaleStockError(err, stockConflict)
	}
	dispatch := uuid.NewSHA1(input.OperationID, []byte("quick-sale-dispatch"))
	command.OperationID, command.SourceID, command.Reference = dispatch, dispatch, reservation
	command.Kind, command.ExpectedRevision = "dispatch", receipt.Balance.Revision
	_, err = inventory.ApplyFulfilment(tx, command)
	return quickSaleStockError(err, stockConflict)
}

func quickSaleStockError(err, stockConflict error) error {
	if errors.Is(err, inventory.ErrConflict) {
		return stockConflict
	}
	return err
}
