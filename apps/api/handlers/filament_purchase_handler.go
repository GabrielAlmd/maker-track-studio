package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type FilamentPurchaseInput struct {
	FilamentID   uuid.UUID `json:"filament_id" binding:"required"`
	Price        float64   `json:"price" binding:"required"`
	Amount       float64   `json:"amount" binding:"required"`
	PurchaseDate time.Time `json:"purchase_date" binding:"required"`
}

// GetFilamentPurchases godoc
// @Summary      List all filament purchases
// @Description  Retrieve all filament purchases
// @Tags         filament_purchases
// @Produce      json
// @Success      200  {array}  dto.FilamentPurchaseDTO
// @Router       /filament-purchases [get]
func GetFilamentPurchases(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, valid := parseListParams(c, map[string]string{"date": "filament_purchases.purchase_date", "price": "filament_purchases.price", "brand": "filaments.brand", "type": "filaments.type"}, "filament_purchases.purchase_date")
		if !valid {
			c.JSON(400, gin.H{"error": "Invalid list parameters"})
			return
		}
		query := db.Model(&models.FilamentPurchase{})
		if params.Enabled {
			query = query.Joins("JOIN filaments ON filaments.id = filament_purchases.filament_id")
			if search := strings.TrimSpace(c.Query("search")); search != "" {
				value := "%" + strings.ToLower(search) + "%"
				query = query.Where("LOWER(filaments.brand) LIKE ? OR LOWER(filaments.type) LIKE ? OR LOWER(filaments.color) LIKE ?", value, value, value)
			}
			for key, column := range map[string]string{"brand": "filaments.brand", "type": "filaments.type"} {
				if value := strings.TrimSpace(c.Query(key)); value != "" {
					query = query.Where("LOWER("+column+") LIKE ?", "%"+strings.ToLower(value)+"%")
				}
			}
			for key, op := range map[string]string{"min_price": ">=", "max_price": "<="} {
				if raw := c.Query(key); raw != "" {
					value, err := strconv.ParseFloat(raw, 64)
					if err != nil || value < 0 {
						c.JSON(400, gin.H{"error": "Invalid price filter"})
						return
					}
					query = query.Where("filament_purchases.price "+op+" ?", value)
				}
			}
			for key, op := range map[string]string{"date_from": ">=", "date_to": "<="} {
				if raw := c.Query(key); raw != "" {
					value, err := time.Parse("2006-01-02", raw)
					if err != nil {
						c.JSON(400, gin.H{"error": "Invalid date filter"})
						return
					}
					if key == "date_to" {
						value = value.Add(24 * time.Hour)
						op = "<"
					}
					query = query.Where("filament_purchases.purchase_date "+op+" ?", value)
				}
			}
		}
		var purchases []models.FilamentPurchase
		var total int64
		if params.Enabled {
			if err := query.Count(&total).Error; err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve filament purchases"})
				return
			}
			query = query.Order(params.Sort + " " + params.Direction).Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
		}
		if err := query.Preload("Filament").Find(&purchases).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve filament purchases"})
			return
		}
		var dtoList []dto.FilamentPurchaseDTO
		for _, p := range purchases {
			dtoList = append(dtoList, dto.FilamentPurchaseDTO{
				ID:           p.ID,
				FilamentID:   p.FilamentID,
				Price:        p.Price,
				Amount:       p.Amount,
				PurchaseDate: p.PurchaseDate,
				CreatedAt:    p.CreatedAt,
				UpdatedAt:    p.UpdatedAt,
				Filament:     &dto.FilamentDTO{ID: p.Filament.ID, Brand: p.Filament.Brand, Type: p.Filament.Type, Color: p.Filament.Color},
			})
		}
		if params.Enabled {
			c.JSON(200, PageResponse[dto.FilamentPurchaseDTO]{Items: dtoList, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, dtoList)
	}
}

// GetFilamentPurchase godoc
// @Summary      Get filament purchase by ID
// @Description  Retrieve a specific filament purchase by ID
// @Tags         filament_purchases
// @Param        id  path      string  true  "Filament Purchase ID"
// @Produce      json
// @Success      200  {object}  dto.FilamentPurchaseDTO
// @Router       /filament-purchases/{id} [get]
func GetFilamentPurchase(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var p models.FilamentPurchase
		if err := db.Preload("Filament").Where("id = ?", id).First(&p).Error; err != nil {
			c.JSON(404, gin.H{"error": "Filament purchase not found"})
			return
		}
		dtoObj := dto.FilamentPurchaseDTO{
			ID:           p.ID,
			FilamentID:   p.FilamentID,
			Price:        p.Price,
			Amount:       p.Amount,
			PurchaseDate: p.PurchaseDate,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
		}
		c.JSON(200, dtoObj)
	}
}

// CreateFilamentPurchase godoc
// @Summary      Create a new filament purchase
// @Description  Create a new filament purchase entry
// @Tags         filament_purchases
// @Param        input  body      FilamentPurchaseInput  true  "Filament Purchase Input"
// @Produce      json
// @Success      201  {object}  dto.FilamentPurchaseDTO
// @Router       /filament-purchases [post]
func CreateFilamentPurchase(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input FilamentPurchaseInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		purchase := models.FilamentPurchase{
			FilamentID:   input.FilamentID,
			Price:        input.Price,
			Amount:       input.Amount,
			PurchaseDate: input.PurchaseDate,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := db.Create(&purchase).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to create filament purchase"})
			return
		}
		dtoObj := dto.FilamentPurchaseDTO{
			ID:           purchase.ID,
			FilamentID:   purchase.FilamentID,
			Price:        purchase.Price,
			Amount:       purchase.Amount,
			PurchaseDate: purchase.PurchaseDate,
			CreatedAt:    purchase.CreatedAt,
			UpdatedAt:    purchase.UpdatedAt,
		}
		c.JSON(201, dtoObj)
	}
}

// UpdateFilamentPurchase godoc
// @Summary      Update an existing filament purchase
// @Description  Update an existing filament purchase entry
// @Tags         filament_purchases
// @Param        id    path      string                  true  "Filament Purchase ID"
// @Param        input body      FilamentPurchaseInput   true  "Filament Purchase Input"
// @Produce      json
// @Success      200  {object}  dto.FilamentPurchaseDTO
// @Router       /filament-purchases/{id} [put]
func UpdateFilamentPurchase(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var purchase models.FilamentPurchase
		if err := db.Where("id = ?", id).First(&purchase).Error; err != nil {
			c.JSON(404, gin.H{"error": "Filament purchase not found"})
			return
		}
		var input FilamentPurchaseInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		purchase.FilamentID = input.FilamentID
		purchase.Price = input.Price
		purchase.Amount = input.Amount
		purchase.PurchaseDate = input.PurchaseDate
		purchase.UpdatedAt = time.Now()
		if err := db.Save(&purchase).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to update filament purchase"})
			return
		}
		dtoObj := dto.FilamentPurchaseDTO{
			ID:           purchase.ID,
			FilamentID:   purchase.FilamentID,
			Price:        purchase.Price,
			Amount:       purchase.Amount,
			PurchaseDate: purchase.PurchaseDate,
			CreatedAt:    purchase.CreatedAt,
			UpdatedAt:    purchase.UpdatedAt,
		}
		c.JSON(200, dtoObj)
	}
}

// DeleteFilamentPurchase godoc
// @Summary      Delete filament purchase
// @Description  Delete a filament purchase entry by ID
// @Tags         filament_purchases
// @Param        id  path      string  true  "Filament Purchase ID"
// @Produce      json
// @Success      204
// @Router       /filament-purchases/{id} [delete]
func DeleteFilamentPurchase(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		if err := db.Where("id = ?", id).Delete(&models.FilamentPurchase{}).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to delete filament purchase"})
			return
		}
		c.Status(204)
	}
}
