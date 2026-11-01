package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
)

type AppConfigInput struct {
	Value string `json:"value" binding:"required"`
	Type  string `json:"type" binding:"required"`
}

var allowedAppConfigKeys = map[string]struct{}{
	"electricity_cost_kwh":   {},
	"printer_wear_cost_hour": {},
	"labor_cost_hour":        {},
	"filament_waste_factor":  {},
}

func isManagedAppConfigKey(key string) bool {
	if key == models.AppFeaturesConfigKey {
		return true
	}
	if key == equipmentCostsKey || key == filamentTypesKey {
		return true
	}
	// Old databases may still contain the removed Print Job pricing tiers.
	// Keep them managed for backup compatibility, but never expose or accept
	// edits for them through the current configuration API.
	if models.IsLegacyMarginTierKey(key) {
		return true
	}
	if _, ok := allowedAppConfigKeys[key]; ok {
		return true
	}
	_, ok := models.AdvancedConfigDefaults[key]
	return ok
}

func loadFeatureConfig(db *gorm.DB) (map[string]bool, error) {
	var config models.AppConfig
	err := db.Where("key = ?", models.AppFeaturesConfigKey).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var features map[string]bool
	if err := json.Unmarshal([]byte(config.Value), &features); err != nil {
		return nil, err
	}
	if features == nil {
		return map[string]bool{}, nil
	}
	return features, nil
}

func GetFeatures(db *gorm.DB, installed *modules.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		stored, err := loadFeatureConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve feature settings"})
			return
		}
		visible := installed.DefaultFeatures()
		for key := range visible {
			if enabled, ok := stored[key]; ok {
				visible[key] = enabled
			}
		}
		c.JSON(200, visible)
	}
}

// UpdateFeatures replaces installed module preferences and retains preferences
// for modules absent from this build. It never changes module-owned data.
func UpdateFeatures(db *gorm.DB, installed *modules.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input map[string]json.RawMessage
		defaults := installed.DefaultFeatures()
		if err := c.ShouldBindJSON(&input); err != nil || len(input) != len(defaults) {
			c.JSON(400, gin.H{"error": "Invalid feature settings"})
			return
		}
		for key := range defaults {
			raw, ok := input[key]
			if !ok || (string(raw) != "true" && string(raw) != "false") {
				c.JSON(400, gin.H{"error": "Invalid feature settings"})
				return
			}
		}
		stored, err := loadFeatureConfig(db)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to save feature settings"})
			return
		}
		response := make(map[string]bool, len(defaults))
		for key := range defaults {
			enabled := string(input[key]) == "true"
			stored[key], response[key] = enabled, enabled
		}
		value, err := json.Marshal(stored)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to save feature settings"})
			return
		}
		now := time.Now()
		config := models.AppConfig{Key: models.AppFeaturesConfigKey, Value: string(value), Type: "json", CreatedAt: now, UpdatedAt: now}
		if err := db.Where("key = ?", models.AppFeaturesConfigKey).
			Assign(models.AppConfig{Value: config.Value, Type: config.Type, UpdatedAt: now}).
			FirstOrCreate(&config).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to save feature settings"})
			return
		}
		c.JSON(200, response)
	}
}

func isClientAppConfigKey(key string) bool {
	if models.IsLegacyMarginTierKey(key) {
		return false
	}
	return key != filamentTypesKey && key != models.AppFeaturesConfigKey && isManagedAppConfigKey(key)
}

func validateAppConfigInput(key string, input AppConfigInput) error {
	_, advanced := models.AdvancedConfigDefaults[key]
	if _, ok := allowedAppConfigKeys[key]; !ok && !advanced {
		return fmt.Errorf("unsupported config key")
	}

	if strings.TrimSpace(input.Type) != "number" {
		return fmt.Errorf("unsupported config type")
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(input.Value), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("value must be a valid number")
	}

	if value < 0 {
		return fmt.Errorf("value must be non-negative")
	}

	if value > 1e9 {
		return fmt.Errorf("value must not exceed 1000000000")
	}
	if key == "machine_cost_mode" && value != 0 && value != 1 {
		return fmt.Errorf("mode must be 0 or 1")
	}
	if (key == "printer_lifetime_years" || key == "printer_uptime_percent") && value < 0.001 {
		return fmt.Errorf("value must be at least 0.001")
	}
	if key == "printer_uptime_percent" && value > 100 {
		return fmt.Errorf("uptime must not exceed 100")
	}
	if key == "printer_buffer_factor" && value < 1 {
		return fmt.Errorf("buffer must be at least 1")
	}

	return nil
}

// GetAppConfig godoc
// @Summary      Get application configuration
// @Description  Retrieve all application configurations
// @Tags         app_config
// @Produce      json
// @Success      200  {array}  dto.AppConfigDTO
// @Router       /app-config [get]
func GetAppConfig(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var config []models.AppConfig
		if err := db.Find(&config).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to retrieve app config"})
			return
		}
		seen := make(map[string]bool)
		for _, row := range config {
			seen[row.Key] = true
		}
		for key, value := range models.AdvancedConfigDefaults {
			if !seen[key] {
				config = append(config, models.AppConfig{Key: key, Value: value, Type: "number"})
			}
		}
		if !seen[equipmentCostsKey] {
			config = append(config, legacyEquipmentConfig(config))
		}
		var dtoList []dto.AppConfigDTO
		for _, ac := range config {
			if !isClientAppConfigKey(ac.Key) {
				continue
			}
			dtoList = append(dtoList, dto.AppConfigDTO{
				Key:       ac.Key,
				Value:     ac.Value,
				Type:      ac.Type,
				CreatedAt: ac.CreatedAt,
				UpdatedAt: ac.UpdatedAt,
			})
		}
		c.JSON(200, dtoList)
	}
}

// GetAppConfigByKey godoc
// @Summary      Get application configuration by key
// @Description  Retrieve a specific application configuration by key
// @Tags         app_config
// @Param        key  path      string  true  "Configuration Key"
// @Produce      json
// @Success      200  {object}  dto.AppConfigDTO
// @Router       /app-config/{key} [get]
func GetAppConfigByKey(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")
		if !isClientAppConfigKey(key) {
			c.JSON(404, gin.H{"error": "App config not found"})
			return
		}
		var config models.AppConfig
		if err := db.Where("key = ?", key).First(&config).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) && key == equipmentCostsKey {
				var rows []models.AppConfig
				if err := db.Where("key = ?", "printer_upfront_cost").Find(&rows).Error; err != nil {
					c.JSON(500, gin.H{"error": "Failed to retrieve equipment costs"})
					return
				}
				c.JSON(200, legacyEquipmentConfig(rows))
				return
			}
			value, advanced := models.AdvancedConfigDefaults[key]
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(500, gin.H{"error": "Failed to retrieve app config"})
				return
			}
			if !advanced {
				c.JSON(404, gin.H{"error": "App config not found"})
				return
			}
			config = models.AppConfig{Key: key, Value: value, Type: "number"}
		}
		c.JSON(200, dto.AppConfigDTO{
			Key:       config.Key,
			Value:     config.Value,
			Type:      config.Type,
			CreatedAt: config.CreatedAt,
			UpdatedAt: config.UpdatedAt,
		})
	}
}

// CreateAppConfig godoc
// @Summary      Create application configuration
// @Description  Create a new application configuration
// @Tags         app_config
// @Param        key  path      string  true  "Configuration Key"
// @Param        input  body      AppConfigInput  true  "Configuration Input"
// @Produce      json
// @Success      201  {object}  dto.AppConfigDTO
// @Router       /app-config/{key} [post]
func CreateAppConfig(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")
		if key == equipmentCostsKey {
			saveEquipmentConfig(c, db)
			return
		}
		if aggregateIsManaged(c, db, key) {
			return
		}
		var input AppConfigInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if err := validateAppConfigInput(key, input); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		config := models.AppConfig{
			Key:       key,
			Value:     strings.TrimSpace(input.Value),
			Type:      strings.TrimSpace(input.Type),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(&config).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to create app config"})
			return
		}
		c.JSON(201, dto.AppConfigDTO{
			Key:       config.Key,
			Value:     config.Value,
			Type:      config.Type,
			CreatedAt: config.CreatedAt,
			UpdatedAt: config.UpdatedAt,
		})
	}
}

// UpdateAppConfig godoc
// @Summary      Update application configuration
// @Description  Update an existing application configuration by key
// @Tags         app_config
// @Param        key  path      string  true  "Configuration Key"
// @Param        input  body      AppConfigInput  true  "Configuration Input"
// @Produce      json
// @Success      200  {object}  dto.AppConfigDTO
// @Router       /app-config/{key} [put]
func UpdateAppConfig(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")
		if key == equipmentCostsKey {
			saveEquipmentConfig(c, db)
			return
		}
		if aggregateIsManaged(c, db, key) {
			return
		}
		var config models.AppConfig
		if err := db.Where("key = ?", key).First(&config).Error; err != nil {
			value, advanced := models.AdvancedConfigDefaults[key]
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(500, gin.H{"error": "Failed to retrieve app config"})
				return
			}
			if !advanced {
				c.JSON(404, gin.H{"error": "App config not found"})
				return
			}
			config = models.AppConfig{Key: key, Value: value, Type: "number"}
		}
		var input AppConfigInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}
		if err := validateAppConfigInput(key, input); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		config.Value = strings.TrimSpace(input.Value)
		config.Type = strings.TrimSpace(input.Type)
		config.UpdatedAt = time.Now()

		if err := db.Save(&config).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to update app config"})
			return
		}
		c.JSON(200, dto.AppConfigDTO{
			Key:       config.Key,
			Value:     config.Value,
			Type:      config.Type,
			CreatedAt: config.CreatedAt,
			UpdatedAt: config.UpdatedAt,
		})
	}
}

// DeleteAppConfig godoc
// @Summary      Delete application configuration
// @Description  Delete an application configuration by key
// @Tags         app_config
// @Param        key  path      string  true  "Configuration Key"
// @Produce      json
// @Success      204
// @Router       /app-config/{key} [delete]
func DeleteAppConfig(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")
		if !isClientAppConfigKey(key) {
			c.JSON(404, gin.H{"error": "App config not found"})
			return
		}
		if key == equipmentCostsKey {
			c.JSON(400, gin.H{"error": "Save an empty equipment list to remove all expenses"})
			return
		}
		if aggregateIsManaged(c, db, key) {
			return
		}
		if err := db.Where("key = ?", key).Delete(&models.AppConfig{}).Error; err != nil {
			c.JSON(500, gin.H{"error": "Failed to delete app config"})
			return
		}
		c.Status(204)
	}
}
