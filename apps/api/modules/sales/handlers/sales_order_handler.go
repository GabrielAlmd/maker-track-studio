package saleshandlers

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func salesOrderID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Order UUID"})
		return uuid.Nil, false
	}
	return id, true
}

func salesOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Sales Order not found"})
	case errors.Is(err, errSalesChannelNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": errSalesChannelNotFound.Error()})
	case errors.Is(err, errSalesModelNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": errSalesModelNotFound.Error()})
	case errors.Is(err, errSalesAllocationPrintJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": errSalesAllocationPrintJobNotFound.Error()})
	case errors.Is(err, errSalesAllocationItemConflict):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationItemConflict.Error()})
	case errors.Is(err, errSalesAllocationModelConflict):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationModelConflict.Error()})
	case errors.Is(err, errSalesAllocationItemCapacity):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationItemCapacity.Error()})
	case errors.Is(err, errSalesAllocationPrintJobCapacity):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationPrintJobCapacity.Error()})
	case errors.Is(err, errSalesAllocationsNotEditable):
		c.JSON(http.StatusConflict, gin.H{"error": "Completed or cancelled Sales Order allocations cannot be changed"})
	case errors.Is(err, errSalesAllocationConcurrentConflict):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesAllocationConcurrentConflict.Error()})
	case errors.Is(err, errSalesCompletionAllocationConflict):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesCompletionAllocationConflict.Error()})
	case errors.Is(err, errSalesCompletionCostUnavailable):
		c.JSON(http.StatusConflict, gin.H{"error": errSalesCompletionCostUnavailable.Error()})
	case errors.Is(err, errSalesChannelArchived):
		c.JSON(http.StatusConflict, gin.H{"error": "Archived Sales Channels cannot be assigned to orders"})
	case errors.Is(err, errSalesOrderNotEditable):
		c.JSON(http.StatusConflict, gin.H{"error": "Only draft Sales Orders can be edited"})
	case errors.Is(err, errSalesStatusRequiresRoute):
		c.JSON(http.StatusConflict, gin.H{"error": "Use the Sales Order status endpoint to change statuses"})
	case errors.Is(err, errSalesStatusConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "Sales Order fulfilment transition is not allowed"})
	case errors.Is(err, errSalesDeleteConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "Only draft Sales Orders can be permanently deleted; cancel this order instead"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save or retrieve Sales Order"})
	}
}

func bindSalesOrder(c *gin.Context) (salesdto.SalesOrderRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var input salesdto.SalesOrderRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Order input"})
		return salesdto.SalesOrderRequest{}, false
	}
	if err := validateSalesOrderRequest(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return salesdto.SalesOrderRequest{}, false
	}
	return input, true
}

func GetSalesOrder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		order, err := loadSalesOrderDTO(db, id)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusOK, order)
	}
}

func CreateSalesOrder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindSalesOrder(c)
		if !ok {
			return
		}
		order, err := createSalesOrder(db, input)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusCreated, order)
	}
}

func UpdateSalesOrder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		input, ok := bindSalesOrder(c)
		if !ok {
			return
		}
		order, err := updateSalesOrder(db, id, input)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusOK, order)
	}
}

func UpdateSalesOrderStatus(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var input salesdto.SalesOrderStatusRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Order status input"})
			return
		}
		if err := validateSalesOrderStatusRequest(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		order, err := updateSalesOrderStatus(db, id, input)
		if err != nil {
			salesOrderError(c, err)
			return
		}
		c.JSON(http.StatusOK, order)
	}
}

func DeleteSalesOrder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		if err := deleteSalesOrder(db, id); err != nil {
			salesOrderError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
