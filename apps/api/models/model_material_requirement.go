package models

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	MaxModelMaterialRequirements = 100
	MaxModelMaterialGrams        = 1e9
)

var ErrInvalidModelMaterials = errors.New("invalid model material requirements")

type ModelMaterialRequirement struct {
	ID                  uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	ModelID             uuid.UUID  `json:"-" gorm:"type:uuid;not null;index"`
	Position            int        `json:"position" gorm:"not null"`
	MaterialType        string     `json:"material_type" gorm:"not null"`
	Color               string     `json:"color"`
	PartWeightG         float64    `json:"part_weight_g" gorm:"not null"`
	WasteWeightG        *float64   `json:"waste_weight_g"`
	PreferredFilamentID *uuid.UUID `json:"preferred_filament_id" gorm:"type:uuid;index"`
	PreferredFilament   *Filament  `json:"-" gorm:"foreignKey:PreferredFilamentID;constraint:OnDelete:SET NULL"`
	SourceTool          *int       `json:"source_tool"`
}

func (r *ModelMaterialRequirement) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&r.ID)
	return nil
}

// ModelMaterialSummary contains estimates per finished model unit.
type ModelMaterialSummary struct {
	PartWeightG    float64 `json:"part_weight_g"`
	KnownWasteG    float64 `json:"known_waste_g"`
	WasteComplete  bool    `json:"waste_complete"`
	ColorStatus    string  `json:"color_status"`
	MaterialStatus string  `json:"material_status"`
}

func ValidModelMaterialGrams(grams float64) bool {
	return grams >= 0 && grams <= MaxModelMaterialGrams && !math.IsNaN(grams)
}

func SummarizeModelMaterials(rows []ModelMaterialRequirement) (ModelMaterialSummary, error) {
	summary := ModelMaterialSummary{ColorStatus: "unknown", MaterialStatus: "unknown"}
	if len(rows) == 0 {
		return summary, nil
	}
	if len(rows) > MaxModelMaterialRequirements {
		return summary, ErrInvalidModelMaterials
	}
	summary.WasteComplete = true
	types, colors := map[string]bool{}, map[string]bool{}
	unknownColor := false
	for position, row := range rows {
		material := strings.TrimSpace(row.MaterialType)
		color := strings.TrimSpace(row.Color)
		if row.Position != position || material == "" || utf8.RuneCountInString(material) > 100 ||
			utf8.RuneCountInString(color) > 100 || !ValidModelMaterialGrams(row.PartWeightG) ||
			row.SourceTool != nil && (*row.SourceTool < 1 || *row.SourceTool > MaxModelMaterialRequirements) {
			return summary, ErrInvalidModelMaterials
		}
		types[strings.ToLower(material)] = true
		if color == "" {
			unknownColor = true
		} else {
			colors[strings.ToLower(color)] = true
		}
		summary.PartWeightG += row.PartWeightG
		if row.WasteWeightG == nil {
			summary.WasteComplete = false
		} else if !ValidModelMaterialGrams(*row.WasteWeightG) {
			return summary, ErrInvalidModelMaterials
		} else {
			summary.KnownWasteG += *row.WasteWeightG
		}
	}
	if !ValidModelMaterialGrams(summary.PartWeightG + summary.KnownWasteG) {
		return summary, ErrInvalidModelMaterials
	}
	summary.MaterialStatus = "single"
	if len(types) > 1 {
		summary.MaterialStatus = "multiple"
	}
	if len(colors) > 1 {
		summary.ColorStatus = "multiple"
	} else if !unknownColor {
		summary.ColorStatus = "single"
	}
	return summary, nil
}
