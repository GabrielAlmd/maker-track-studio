package handlers

import (
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/inventory"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type materialInventoryRow struct {
	ID              uuid.UUID         `json:"id"`
	Brand           string            `json:"brand"`
	Type            string            `json:"type"`
	Color           string            `json:"color"`
	Archived        bool              `json:"archived"`
	WarningsEnabled bool              `json:"warnings_enabled"`
	TrackingEnabled bool              `json:"tracking_enabled" gorm:"-"`
	ProductURL      string            `json:"product_url,omitempty"`
	ThresholdMG     int64             `json:"threshold_mg"`
	Balance         inventory.Balance `json:"balance" gorm:"embedded"`
}

// InventoryMaterials retains the legacy array for callers without list parameters.
func InventoryMaterials(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		columns := map[string]string{"brand": "brand", "type": "type", "color": "color"}
		params, valid := parseListParams(c, columns, "brand")
		if !valid {
			c.JSON(400, gin.H{"error": "Invalid list parameters"})
			return
		}
		statusFilters := map[string]string{
			"tracked":    "NOT COALESCE(stock.tracking_paused, 0)",
			"untracked":  "COALESCE(stock.tracking_paused, 0)",
			"archived":   "archived",
			"unverified": "NOT COALESCE(stock.tracking_paused, 0) AND NOT COALESCE(stock.verified, 0)",
			"low_stock":  "stock.verified AND NOT stock.tracking_paused AND NOT archived AND NOT warnings_disabled AND stock.on_hand - stock.reserved <= stock_threshold_mg",
		}
		status := c.Query("status")
		statusFilter, validStatus := statusFilters[status]
		if status != "" && !validStatus {
			c.JSON(400, gin.H{"error": "Invalid stock status"})
			return
		}

		rows := make([]materialInventoryRow, 0)
		var total int64
		err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			query := tx.Model(&models.Filament{}).
				Joins("LEFT JOIN (?) AS stock ON stock.item_id = filaments.id", inventory.MaterialBalances(tx))
			if search := strings.ToLower(strings.TrimSpace(c.Query("search"))); search != "" {
				query = query.Where("INSTR(LOWER(brand || ' ' || type || ' ' || color), ?) > 0", search)
			}
			for key, column := range columns {
				if value := strings.ToLower(strings.TrimSpace(c.Query(key))); value != "" {
					query = query.Where("INSTR(LOWER("+column+"), ?) > 0", value)
				}
			}
			if statusFilter != "" {
				query = query.Where(statusFilter)
			}
			if params.Enabled {
				if err := query.Count(&total).Error; err != nil {
					return err
				}
				if total == 0 || int64(params.Page-1) > (total-1)/int64(params.PageSize) {
					return nil
				}
				query = query.Order(params.Sort + " " + params.Direction).
					Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
			}
			return query.Select(`id, brand, type, color, archived, NOT warnings_disabled AS warnings_enabled,
				product_url, stock_threshold_mg AS threshold_mg, COALESCE(stock.on_hand, 0) AS on_hand,
				COALESCE(stock.reserved, 0) AS reserved, COALESCE(stock.verified, 0) AS verified,
				COALESCE(stock.tracking_paused, 0) AS tracking_paused,
				COALESCE(stock.tracking_gap, 0) AS tracking_gap, COALESCE(stock.revision, 0) AS revision`).
				Order("brand, type, color, id").Scan(&rows).Error
		})
		if err != nil {
			inventoryError(c, err)
			return
		}
		for i := range rows {
			rows[i].Balance.SetAvailability()
			rows[i].TrackingEnabled = !rows[i].Balance.TrackingPaused
		}
		if params.Enabled {
			c.JSON(200, PageResponse[materialInventoryRow]{Items: rows, Total: total, Page: params.Page, PageSize: params.PageSize})
			return
		}
		c.JSON(200, rows)
	}
}
