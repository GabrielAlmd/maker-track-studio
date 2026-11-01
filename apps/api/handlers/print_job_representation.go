package handlers

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func modelReference(pj models.PrintJob) dto.ModelReferenceDTO {
	if pj.ModelID == nil {
		return dto.ModelReferenceDTO{State: "not_assigned", Name: "Model not assigned"}
	}
	if pj.Model == nil || pj.Model.ID == uuid.Nil {
		return dto.ModelReferenceDTO{State: "deleted", Name: "Deleted model"}
	}
	return dto.ModelReferenceDTO{State: "assigned", ID: &pj.Model.ID, Name: pj.Model.Name}
}

func filamentReference(pjf models.PrintJobFilament) dto.FilamentReferenceDTO {
	if pjf.Filament.ID == uuid.Nil {
		return dto.FilamentReferenceDTO{State: "unavailable", Name: "Unavailable filament"}
	}
	return dto.FilamentReferenceDTO{State: "available", ID: &pjf.Filament.ID, Name: filamentDisplayName(pjf.Filament)}
}

func filamentDisplayName(f models.Filament) string {
	name := f.Brand
	if f.Type != "" {
		name += " " + f.Type
	}
	if f.Color != "" {
		name += " " + f.Color
	}
	return name
}

func publicPrinterCostSnapshot(snapshot *models.PrinterCostSnapshot) *models.PrinterCostSnapshot {
	if snapshot == nil {
		return nil
	}
	config := make(map[string]float64, len(snapshot.Config))
	for key, value := range snapshot.Config {
		if !models.IsLegacyMarginTierKey(key) {
			config[key] = value
		}
	}
	return &models.PrinterCostSnapshot{
		Version: snapshot.Version, PrinterName: snapshot.PrinterName,
		CapturedAt: snapshot.CapturedAt, Config: config,
	}
}

func printJobDTO(pj models.PrintJob, cfg map[string]float64) dto.PrintJobDTO {
	filaments := make([]dto.PrintJobFilamentDTO, 0, len(pj.Filaments))
	for _, f := range pj.Filaments {
		filaments = append(filaments, dto.PrintJobFilamentDTO{ID: f.ID, PrintJobID: f.PrintJobID, FilamentID: f.FilamentID, WeightUsed: f.WeightUsed, Filament: filamentReference(f)})
	}
	cost := calculatePrintJobCostState(pj, cfg)
	return dto.PrintJobDTO{ID: pj.ID, PrinterID: pj.PrinterID, PrinterCost: publicPrinterCostSnapshot(pj.PrinterCost), ModelID: modelReferenceID(pj), PrintDate: pj.PrintDate, UnitsPrinted: pj.UnitsPrinted, LaborHours: pj.LaborHours, PrintTimeMinutes: pj.PrintTimeMinutes, EstimatedCost: cost.Total, Model: modelReference(pj), Cost: cost, CreatedAt: pj.CreatedAt, UpdatedAt: pj.UpdatedAt, Filaments: filaments}
}

func loadPrintJobDTO(db *gorm.DB, id uuid.UUID) (dto.PrintJobDTO, error) {
	var printJob models.PrintJob
	if err := db.Preload("Filaments.Filament").Preload("Model").First(&printJob, "id = ?", id).Error; err != nil {
		return dto.PrintJobDTO{}, err
	}
	cfg, err := loadCostConfig(db)
	if err != nil {
		return dto.PrintJobDTO{}, err
	}
	return printJobDTO(printJob, cfg), nil
}

func modelReferenceID(pj models.PrintJob) *uuid.UUID {
	if pj.Model != nil && pj.Model.ID != uuid.Nil {
		return &pj.Model.ID
	}
	return nil
}
