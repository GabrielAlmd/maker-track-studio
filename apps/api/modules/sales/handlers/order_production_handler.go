package saleshandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxProductionUnits = 1_000_000_000

func orderProductionError(c *gin.Context, err error) {
	var conflict productionConflict
	if errors.As(err, &conflict) {
		c.JSON(http.StatusConflict, gin.H{"error": conflict.Error()})
		return
	}
	if isSQLiteAllocationLockError(err) {
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationConcurrentConflict.Error()})
		return
	}
	salesOrderError(c, err)
}

func GetOrderProduction(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		var result salesdto.OrderProduction
		if err := db.Transaction(func(tx *gorm.DB) error {
			state, err := loadOrderProduction(tx, id)
			result = state.document()
			return err
		}); err != nil {
			orderProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func ReplaceOrderProduction(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
		var input salesdto.OrderProductionRequest
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&input)
		if err == nil {
			err = decoder.Decode(new(json.RawMessage))
			if err == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Provide exactly one production request"})
				return
			}
		}
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Production request exceeds 256 KiB"})
			return
		}
		if !errors.Is(err, io.EOF) || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 ||
			input.Links == nil || len(input.Links) > maxSalesOrderAllocations {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A production revision and a valid links array are required"})
			return
		}
		links := make(map[productionLinkKey]salesdto.OrderProductionLinkInput, len(input.Links))
		for _, link := range input.Links {
			key := productionLinkKey{link.SalesOrderItemID, link.PrintRunItemID}
			_, duplicate := links[key]
			if duplicate || key.line == uuid.Nil || key.source == uuid.Nil ||
				link.UnitsPlanned < 0 || link.UnitsPlanned > maxProductionUnits ||
				link.UnitsAllocated < 0 || link.UnitsAllocated > maxProductionUnits ||
				link.UnitsPlanned == 0 && link.UnitsAllocated == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Each line and run item requires unique IDs and valid production quantities"})
				return
			}
			links[key] = link
		}
		result, err := replaceOrderProduction(db, id, *input.ExpectedRevision, links)
		if err != nil {
			orderProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func GetOrderProductionSources(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		line, err := uuid.Parse(c.Query("line_id"))
		cursor := uuid.Nil
		if err == nil && c.Query("after") != "" {
			cursor, err = uuid.Parse(c.Query("after"))
		}
		if err != nil || line == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid production line or cursor"})
			return
		}
		result, err := listOrderProductionSources(db, id, line, cursor)
		if err != nil {
			orderProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
