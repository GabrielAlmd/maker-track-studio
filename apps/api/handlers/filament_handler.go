package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strconv"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

type FilamentInput struct {
	Brand          string  `json:"brand" binding:"required,max=200"`
	Type           string  `json:"type" binding:"required,max=200"`
	Color          string  `json:"color" binding:"required,max=200"`
	AvgPrice       float64 `json:"avg_price" binding:"required,gt=0"`
	PersonalScore  int     `json:"personal_score"`
	TimesUsed      int     `json:"times_used"`
	TimesPurchased int     `json:"times_purchased"`
}

// GetFilaments godoc
// @Summary      List all filaments
// @Description  Retrieve all filaments with their purchases and print jobs
// @Tags         filaments
// @Produce      json
// @Success      200  {array}  dto.FilamentDTO
// @Router       /filaments [get]
func GetFilaments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, valid := parseListParams(c, map[string]string{"brand": "brand", "type": "type", "color": "color", "rating": "personal_score", "price": "avg_price", "created_at": "created_at"}, "brand")
		if !valid {
			c.JSON(400, gin.H{"error": "Invalid list parameters"})
			return
		}
		query := db.Model(&models.Filament{})
		if params.Enabled {
			if search := strings.TrimSpace(c.Query("search")); search != "" {
				value := "%" + strings.ToLower(search) + "%"
				query = query.Where("LOWER(brand) LIKE ? OR LOWER(type) LIKE ? OR LOWER(color) LIKE ?", value, value, value)
			}
			for key, column := range map[string]string{"brand": "brand", "type": "type"} {
				if value := strings.TrimSpace(c.Query(key)); value != "" {
					query = query.Where("LOWER("+column+") LIKE ?", "%"+strings.ToLower(value)+"%")
				}
			}
			if rating := c.Query("rating"); rating != "" {
				value, err := strconv.Atoi(rating)
				if err != nil || value < 1 || value > 5 {
					c.JSON(400, gin.H{"error": "Invalid rating filter"})
					return
				}
				query = query.Where("personal_score = ?", value)
			}
			if low := c.Query("low_stock"); low != "" {
				if low != "true" && low != "false" {
					c.JSON(400, gin.H{"error": "Invalid low_stock filter"})
					return
				}
				if low == "true" {
					query = query.Where("times_purchased > 0 AND times_used >= times_purchased")
				}
			}
		}
		var filaments []models.Filament
		var total int64
		if params.Enabled {
			if err := query.Count(&total).Error; err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve filaments"})
				return
			}
			query = query.Order(params.Sort + " " + params.Direction).Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
		}
		if err := query.Preload("Purchases").Preload("PrintJobs").Find(&filaments).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve filaments"})
			return
		}

		dtoList := make([]dto.FilamentDTO, 0)
		for _, f := range filaments {
			// Map purchases
			var purchases []dto.FilamentPurchaseDTO
			for _, p := range f.Purchases {
				purchases = append(purchases, dto.FilamentPurchaseDTO{
					ID:           p.ID,
					FilamentID:   p.FilamentID,
					Price:        p.Price,
					Amount:       p.Amount,
					PurchaseDate: p.PurchaseDate,
					CreatedAt:    p.CreatedAt,
					UpdatedAt:    p.UpdatedAt,
				})
			}
			// Map print jobs
			var printJobs []dto.PrintJobFilamentDTO
			for _, pj := range f.PrintJobs {
				printJobs = append(printJobs, dto.PrintJobFilamentDTO{
					ID:         pj.ID,
					PrintJobID: pj.PrintJobID,
					FilamentID: pj.FilamentID,
					WeightUsed: pj.WeightUsed,
				})
			}
			dtoList = append(dtoList, dto.FilamentDTO{
				ID:             f.ID,
				Brand:          f.Brand,
				Type:           f.Type,
				Color:          f.Color,
				AvgPrice:       f.AvgPrice,
				PersonalScore:  f.PersonalScore,
				TimesUsed:      f.TimesUsed,
				TimesPurchased: f.TimesPurchased,
				CreatedAt:      f.CreatedAt,
				UpdatedAt:      f.UpdatedAt,
				Purchases:      purchases,
				PrintJobs:      printJobs,
			})
		}
		if params.Enabled {
			c.JSON(200, PageResponse[dto.FilamentDTO]{Items: dtoList, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, dtoList)
	}
}

// GetFilamentBrands returns the normalized, case-insensitively unique brand
// names used by existing filaments.
func GetFilamentBrands(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		brands, err := listFilamentBrands(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve filament brands"})
			return
		}
		c.JSON(200, brands)
	}
}

// GetFilament godoc
// @Summary      Get filament by ID
// @Description  Retrieve a specific filament by ID with its purchases and print jobs
// @Tags         filaments
// @Param        id  path      string  true  "Filament ID"
// @Produce      json
// @Success      200  {object}  dto.FilamentDTO
// @Router       /filaments/{id} [get]
func GetFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var f models.Filament
		if err := db.Preload("Purchases").Preload("PrintJobs").Where("id = ?", id).First(&f).Error; err != nil {
			c.JSON(404, gin.H{"error": "Filament not found"})
			return
		}
		// Map purchases
		var purchases []dto.FilamentPurchaseDTO
		for _, p := range f.Purchases {
			purchases = append(purchases, dto.FilamentPurchaseDTO{
				ID:           p.ID,
				FilamentID:   p.FilamentID,
				Price:        p.Price,
				Amount:       p.Amount,
				PurchaseDate: p.PurchaseDate,
				CreatedAt:    p.CreatedAt,
				UpdatedAt:    p.UpdatedAt,
			})
		}
		// Map print jobs
		var printJobs []dto.PrintJobFilamentDTO
		for _, pj := range f.PrintJobs {
			printJobs = append(printJobs, dto.PrintJobFilamentDTO{
				ID:         pj.ID,
				PrintJobID: pj.PrintJobID,
				FilamentID: pj.FilamentID,
				WeightUsed: pj.WeightUsed,
			})
		}
		dtoObj := dto.FilamentDTO{
			ID:             f.ID,
			Brand:          f.Brand,
			Type:           f.Type,
			Color:          f.Color,
			AvgPrice:       f.AvgPrice,
			PersonalScore:  f.PersonalScore,
			TimesUsed:      f.TimesUsed,
			TimesPurchased: f.TimesPurchased,
			CreatedAt:      f.CreatedAt,
			UpdatedAt:      f.UpdatedAt,
			Purchases:      purchases,
			PrintJobs:      printJobs,
		}
		c.JSON(200, dtoObj)
	}
}

// CreateFilament godoc
// @Summary      Create a new filament
// @Description  Create a new filament entry
// @Tags         filaments
// @Param        input  body      FilamentInput  true  "Filament Input"
// @Produce      json
// @Success      201  {object}  dto.FilamentDTO
// @Router       /filaments [post]
func CreateFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input FilamentInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		brand, err := canonicalFilamentBrand(db, input.Brand)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to create filament"})
			return
		}
		if brand == "" {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		filament := models.Filament{
			Brand:          brand,
			Type:           input.Type,
			Color:          input.Color,
			AvgPrice:       input.AvgPrice,
			PersonalScore:  input.PersonalScore,
			TimesUsed:      input.TimesUsed,
			TimesPurchased: input.TimesPurchased,
		}
		if err := db.Create(&filament).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to create filament"})
			return
		}
		dtoObj := dto.FilamentDTO{
			ID:             filament.ID,
			Brand:          filament.Brand,
			Type:           filament.Type,
			Color:          filament.Color,
			AvgPrice:       filament.AvgPrice,
			PersonalScore:  filament.PersonalScore,
			TimesUsed:      filament.TimesUsed,
			TimesPurchased: filament.TimesPurchased,
			CreatedAt:      filament.CreatedAt,
			UpdatedAt:      filament.UpdatedAt,
			Purchases:      nil, // or preload if you want
			PrintJobs:      nil, // or preload if you want
		}
		c.JSON(201, dtoObj)
	}
}

// UpdateFilament godoc
// @Summary      Update an existing filament
// @Description  Update an existing filament entry
// @Tags         filaments
// @Param        id  path      string  true  "Filament ID"
// @Param        input  body      FilamentInput  true  "Filament Input"
// @Produce      json
// @Success      200  {object}  dto.FilamentDTO
// @Router       /filaments/{id} [put]
func UpdateFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		var filament models.Filament
		if err := db.Where("id = ?", id).First(&filament).Error; err != nil {
			c.JSON(404, gin.H{"error": "Filament not found"})
			return
		}

		var input FilamentInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		brand, err := canonicalFilamentBrand(db, input.Brand)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to update filament"})
			return
		}
		if brand == "" {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}

		filament.Brand = brand
		filament.Type = input.Type
		filament.Color = input.Color
		filament.AvgPrice = input.AvgPrice
		filament.PersonalScore = input.PersonalScore
		filament.TimesUsed = input.TimesUsed
		filament.TimesPurchased = input.TimesPurchased

		if err := db.Save(&filament).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to update filament"})
			return
		}
		dtoObj := dto.FilamentDTO{
			ID:             filament.ID,
			Brand:          filament.Brand,
			Type:           filament.Type,
			Color:          filament.Color,
			AvgPrice:       filament.AvgPrice,
			PersonalScore:  filament.PersonalScore,
			TimesUsed:      filament.TimesUsed,
			TimesPurchased: filament.TimesPurchased,
			CreatedAt:      filament.CreatedAt,
			UpdatedAt:      filament.UpdatedAt,
			Purchases:      nil, // or preload if you want
			PrintJobs:      nil, // or preload if you want
		}
		c.JSON(200, dtoObj)
	}
}

// DeleteFilament godoc
// @Summary      Delete filament
// @Description  Delete a filament entry by ID
// @Tags         filaments
// @Param        id  path      string  true  "Filament ID"
// @Produce      json
// @Success      204
// @Router       /filaments/{id} [delete]
func DeleteFilament(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if _, err := uuid.Parse(id); err != nil {
			c.JSON(400, gin.H{"error": "Invalid UUID"})
			return
		}
		if err := db.Where("id = ?", id).Delete(&models.Filament{}).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to delete filament"})
			return
		}
		c.Status(204) // No content response
	}
}
