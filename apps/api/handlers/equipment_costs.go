package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const equipmentCostsKey = "printer_equipment_costs"

const (
	equipmentCategorySpareParts = "spare_parts"
	equipmentCategoryAdditional = "additional"
	equipmentCategoryOther      = "other"
)

type equipmentCost struct {
	Description string  `json:"description"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
}

func validEquipmentCategory(category string) bool {
	return category == equipmentCategorySpareParts || category == equipmentCategoryAdditional || category == equipmentCategoryOther
}

func legacyEquipmentConfig(rows []models.AppConfig) models.AppConfig {
	entries := []equipmentCost{}
	value := models.AdvancedConfigDefaults["printer_upfront_cost"]
	for _, row := range rows {
		if row.Key == "printer_upfront_cost" {
			value = row.Value
		}
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err == nil && amount > 0 && !math.IsInf(amount, 0) {
		entries = append(entries, equipmentCost{Description: "Previous additional costs", Amount: amount, Category: equipmentCategoryAdditional})
	}
	encoded, _ := json.Marshal(entries)
	return models.AppConfig{Key: equipmentCostsKey, Value: string(encoded), Type: "json"}
}

func parseEquipment(input AppConfigInput) ([]equipmentCost, float64, error) {
	if input.Type != "json" || len(input.Value) > 200000 {
		return nil, 0, fmt.Errorf("invalid equipment list")
	}
	var entries []equipmentCost
	if err := json.Unmarshal([]byte(input.Value), &entries); err != nil || entries == nil || len(entries) > 500 {
		return nil, 0, fmt.Errorf("provide an array of at most 500 expenses")
	}
	var amounts []struct {
		Amount *float64 `json:"amount"`
	}
	if err := json.Unmarshal([]byte(input.Value), &amounts); err != nil {
		return nil, 0, fmt.Errorf("invalid amounts")
	}
	for _, entry := range amounts {
		if entry.Amount == nil {
			return nil, 0, fmt.Errorf("amount is required")
		}
	}
	var total float64
	for i := range entries {
		entry := &entries[i]
		entry.Description = strings.TrimSpace(entry.Description)
		// Records created before expense categories existed represented generic
		// additional equipment and retain that meaning after the additive change.
		if entry.Category == "" {
			entry.Category = equipmentCategoryAdditional
		}
		if !validEquipmentCategory(entry.Category) {
			return nil, 0, fmt.Errorf("invalid expense category")
		}
		if entry.Description == "" || len([]rune(entry.Description)) > 200 {
			return nil, 0, fmt.Errorf("description must contain 1 to 200 characters")
		}
		if entry.Date != "" {
			if _, err := time.Parse("2006-01-02", entry.Date); err != nil {
				return nil, 0, fmt.Errorf("date must be YYYY-MM-DD")
			}
		}
		if math.IsNaN(entry.Amount) || math.IsInf(entry.Amount, 0) || entry.Amount < 0 || entry.Amount > 1e9 {
			return nil, 0, fmt.Errorf("amount must be between 0 and 1000000000")
		}
		total += entry.Amount
	}
	if total > 1e9 {
		return nil, 0, fmt.Errorf("total must not exceed 1000000000")
	}
	return entries, total, nil
}

func saveEquipmentConfig(c *gin.Context, db *gorm.DB) {
	var input AppConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}
	entries, total, err := parseEquipment(input)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	encoded, _ := json.Marshal(entries)
	config := models.AppConfig{Key: equipmentCostsKey, Value: string(encoded), Type: "json"}
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, row := range []*models.AppConfig{&config, {Key: "printer_upfront_cost", Value: strconv.FormatFloat(total, 'f', -1, 64), Type: "number"}} {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "type", "updated_at"})}).Create(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save equipment costs"})
		return
	}
	c.JSON(200, config)
}

func aggregateIsManaged(c *gin.Context, db *gorm.DB, key string) bool {
	if key != "printer_upfront_cost" {
		return false
	}
	var count int64
	if err := db.Model(&models.AppConfig{}).Where("key = ?", equipmentCostsKey).Count(&count).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to retrieve equipment costs"})
		return true
	}
	if count > 0 {
		c.JSON(400, gin.H{"error": "Edit the equipment list to change its total"})
		return true
	}
	return false
}
