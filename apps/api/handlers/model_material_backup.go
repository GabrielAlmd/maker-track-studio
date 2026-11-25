package handlers

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func readModelMaterialBackup(db *gorm.DB, modelsList []models.Model, filaments []models.Filament, rows *[]models.ModelMaterialRequirement) error {
	if err := db.Select("id", "model_id", "position", "material_type", "color", "part_weight_g", "waste_weight_g", "preferred_filament_id", "source_tool").
		Order("model_id, position").Find(rows).Error; err != nil {
		return models.ErrInvalidModelMaterials
	}
	modelByID := make(map[uuid.UUID]models.Model, len(modelsList))
	for _, model := range modelsList {
		modelByID[model.ID] = model
	}
	filamentIDs := make(map[uuid.UUID]bool, len(filaments))
	for _, filament := range filaments {
		filamentIDs[filament.ID] = true
	}
	groups := make(map[uuid.UUID][]models.ModelMaterialRequirement)
	seen := make(map[uuid.UUID]bool, len(*rows))
	for _, row := range *rows {
		_, exists := modelByID[row.ModelID]
		if !exists || row.ID == uuid.Nil || seen[row.ID] || row.PreferredFilamentID != nil && !filamentIDs[*row.PreferredFilamentID] {
			return models.ErrInvalidModelMaterials
		}
		seen[row.ID] = true
		groups[row.ModelID] = append(groups[row.ModelID], row)
	}
	for id, requirements := range groups {
		summary, err := models.SummarizeModelMaterials(requirements)
		model := modelByID[id]
		if err != nil || model.MaterialWeight != summary.PartWeightG || model.WasteMaterial != summary.KnownWasteG ||
			model.IsMulticolor != (summary.ColorStatus == "multiple") {
			return models.ErrInvalidModelMaterials
		}
	}
	return nil
}
