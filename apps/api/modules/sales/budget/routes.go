package budget

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type summary struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision"`
	Header
	EstimatedCostCents int64 `json:"estimated_cost_cents"`
}

func RegisterRoutes(router *gin.Engine, db *gorm.DB) {
	internal := router.Group("/v2/business", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
	})
	bind := func(factory func(*gorm.DB) gin.HandlerFunc) gin.HandlerFunc {
		return modules.WithRequestDB(db, factory)
	}

	registerProposalRoutes(internal, db)
	internal.POST("/orders", bind(command(false, confirmOrder)))
	internal.GET("/orders/:id", bind(detail[OrderRevision]))
	internal.POST("/orders/:id/amendments", bind(command(true, amendOrder)))

	internal.GET("/customers", bind(list[Customer, Customer]))
	internal.GET("/customers/:id", bind(detail[Customer]))
	internal.POST("/customers", bind(command(false, writeCustomer)))
	internal.PUT("/customers/:id", bind(command(true, writeCustomer)))
	internal.GET("/budgets", bind(list[Record, summary]))
	internal.GET("/budgets/:id", bind(detail[Record]))
	internal.POST("/budgets", bind(command(false, writeBudget)))
	internal.PUT("/budgets/:id", bind(command(true, writeBudget)))
}

func pathID(c *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, invalid("id", "Provide a valid record UUID")
	}
	return id, nil
}

func positiveQuery(c *gin.Context, key string, fallback, maximum int64) (int64, error) {
	if value, exists := c.GetQuery(key); exists {
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil || number < 1 || number > maximum {
			return 0, invalid(key, "Provide an integer within the supported range")
		}
		return number, nil
	}
	return fallback, nil
}

func list[Model, Result any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var model Model
		latest := db.Model(&model).Select("id, MAX(revision) AS revision").Group("id")
		paginate[Result](c, db.Model(&model).Where("(id, revision) IN (?)", latest).Order("created_at DESC, id ASC"))
	}
}

func paginate[Result any](c *gin.Context, query *gorm.DB) {
	page, err := positiveQuery(c, "page", 1, 100000)
	if err != nil {
		reply(c, 0, nil, err)
		return
	}
	size, err := positiveQuery(c, "page_size", 25, 100)
	if err != nil {
		reply(c, 0, nil, err)
		return
	}
	results := make([]Result, 0)
	err = query.Offset(int((page - 1) * size)).Limit(int(size + 1)).Find(&results).Error
	if err != nil {
		reply(c, 0, nil, err)
		return
	}
	hasMore := len(results) > int(size)
	if hasMore {
		results = results[:size]
	}
	reply(c, http.StatusOK, gin.H{"items": results, "page": page, "page_size": size, "has_more": hasMore}, nil)
}

func detail[T any](db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		revision, err := positiveQuery(c, "revision", 0, maxSafeInteger)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		result, err := readRevision[T](db, id, revision)
		reply(c, http.StatusOK, result, err)
	}
}

func command[Input, Result any](update bool, write func(*gorm.DB, uuid.UUID, Input) (Result, error)) func(*gorm.DB) gin.HandlerFunc {
	return func(db *gorm.DB) gin.HandlerFunc {
		return func(c *gin.Context) {
			id, status := uuid.Nil, http.StatusCreated
			if update {
				var err error
				id, err = pathID(c)
				if err != nil {
					reply(c, 0, nil, err)
					return
				}
				status = http.StatusOK
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<10)
			var input Input
			if err := c.ShouldBindBodyWithJSON(&input); err != nil {
				var limit *http.MaxBytesError
				if errors.As(err, &limit) {
					reply(c, 0, nil, &problem{status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "Request exceeds 512 KiB"})
					return
				}
				reply(c, 0, nil, invalid("body", "Provide a valid JSON request"))
				return
			}
			body, _ := c.Get(gin.BodyBytesKey)
			encoded, _ := body.([]byte)
			if !json.Valid(encoded) {
				reply(c, 0, nil, invalid("body", "Provide exactly one JSON request object"))
				return
			}
			switch any(input).(type) {
			case AmendmentCommand, ConfirmationCommand:
				decoder := json.NewDecoder(bytes.NewReader(encoded))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&input); err != nil {
					reply(c, 0, nil, invalid("body", "Only documented order fields can be changed. Prices, quantities, customer and proposal references are frozen after acceptance."))
					return
				}
			}
			result, err := write(db, id, input)
			reply(c, status, result, err)
		}
	}
}

func reply(c *gin.Context, status int, value any, err error) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	var issue *problem
	if !errors.As(err, &issue) {
		issue = &problem{status: http.StatusInternalServerError, Code: "persistence_error", Message: "Unable to complete the request"}
	}
	c.JSON(issue.status, gin.H{"error": issue})
}
