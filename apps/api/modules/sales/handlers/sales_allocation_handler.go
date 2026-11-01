package saleshandlers

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxSalesOrderAllocations = 1000

func bindSalesOrderAllocations(c *gin.Context) (salesdto.SalesOrderAllocationsRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var input salesdto.SalesOrderAllocationsRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Order allocation input"})
		return salesdto.SalesOrderAllocationsRequest{}, false
	}
	if len(input.Allocations) > maxSalesOrderAllocations {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Too many Sales Order allocations"})
		return salesdto.SalesOrderAllocationsRequest{}, false
	}

	pairs := make(map[string]struct{}, len(input.Allocations))
	for _, allocation := range input.Allocations {
		if allocation.SalesOrderItemID == uuid.Nil || allocation.PrintJobID == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Each allocation requires valid Sales Order Item and Print Job IDs"})
			return salesdto.SalesOrderAllocationsRequest{}, false
		}
		if allocation.UnitsAllocated <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Allocation units must be greater than zero"})
			return salesdto.SalesOrderAllocationsRequest{}, false
		}
		key := allocationPairKey(allocation.SalesOrderItemID, allocation.PrintJobID)
		if _, duplicate := pairs[key]; duplicate {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Duplicate Sales Order Item and Print Job allocation"})
			return salesdto.SalesOrderAllocationsRequest{}, false
		}
		pairs[key] = struct{}{}
	}
	return input, true
}

func GetSalesOrderAllocations(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		result, err := loadSalesOrderAllocationsDTO(db, id)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func ReplaceSalesOrderAllocations(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		input, ok := bindSalesOrderAllocations(c)
		if !ok {
			return
		}
		result, err := replaceSalesOrderAllocations(db, id, input)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func GetPrintJobAllocations(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Print Job UUID"})
			return
		}
		result, err := loadPrintJobAllocationsDTO(db, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Print Job not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve Print Job allocations"})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
