package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const filamentTypesKey = "filament_types"

// The catalog is stored in app_config so existing SQLite backups include it.
// Filament.Type remains a string: editing choices never rewrites historical data.
func GetFilamentTypes(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		types := []string{"PLA", "ABS", "PETG", "TPU", "ASA", "Nylon", "Resin", "Other"}
		var config models.AppConfig
		err := db.Where("key = ?", filamentTypesKey).First(&config).Error
		if err == nil {
			if err = json.Unmarshal([]byte(config.Value), &types); err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve filament types"})
				return
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Include custom types from databases created before the catalog existed.
			var existing []string
			if err := db.Model(&models.Filament{}).Distinct().Order("type").Pluck("type", &existing).Error; err != nil {
				c.JSON(500, gin.H{"error": "Failed to retrieve filament types"})
				return
			}
			for _, name := range existing {
				name = strings.TrimSpace(name)
				found := name == ""
				for _, current := range types {
					if strings.EqualFold(current, name) {
						found = true
						break
					}
				}
				if !found {
					types = append(types, name)
				}
			}
		} else {
			c.JSON(500, gin.H{"error": "Failed to retrieve filament types"})
			return
		}
		c.JSON(200, types)
	}
}

func UpdateFilamentTypes(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Types []string `json:"types" binding:"required,max=200,dive,max=200"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Provide up to 200 filament types, each up to 200 characters"})
			return
		}
		seen := make(map[string]bool)
		for i, name := range input.Types {
			name = strings.TrimSpace(name)
			key := strings.ToLower(name)
			if name == "" || seen[key] {
				c.JSON(400, gin.H{"error": "Filament types must be non-empty and unique"})
				return
			}
			input.Types[i], seen[key] = name, true
		}
		value, err := json.Marshal(input.Types)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to save filament types"})
			return
		}
		config := models.AppConfig{Key: filamentTypesKey, Value: string(value), Type: "json"}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "type", "updated_at"})}).Create(&config).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to save filament types"})
			return
		}
		c.JSON(200, input.Types)
	}
}
