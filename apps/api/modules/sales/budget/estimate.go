package budget

import (
	"fmt"
	"math"
	"strings"
	"time"

	core "github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxSafeInteger = 1<<53 - 1
	maxItems       = 100
	maxMaterials   = 20
	maxInput       = 1_000_000
)

func within(value, maximum float64) bool { return value >= 0 && value <= maximum }

func validItem(input ItemInput) bool {
	a := input.Assumptions
	if !validID(input.ModelID) || !validID(a.PrinterID) ||
		input.Quantity < 1 || input.Quantity > maxInput ||
		a.PrintTimeMinutes < 0 || a.PrintTimeMinutes > maxInput ||
		!within(float64(a.LaborHours), maxInput) ||
		!validMoney(a.PostProcessCents) || !validMoney(a.OtherCents) ||
		len(a.Materials) > maxMaterials {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(a.Materials))
	for _, material := range a.Materials {
		if material.FilamentID == uuid.Nil || seen[material.FilamentID] ||
			!within(material.WeightG, maxInput) || material.WeightG == 0 ||
			(material.WasteG != nil && !within(*material.WasteG, material.WeightG)) {
			return false
		}
		seen[material.FilamentID] = true
	}
	return true
}

func validID(id *uuid.UUID) bool { return id == nil || *id != uuid.Nil }

func validMoney(cents int64) bool { return cents >= 0 && cents <= maxSafeInteger }

func captureSources(tx *gorm.DB, a Assumptions) (Sources, error) {
	source := Sources{
		CalculatorVersion: 1,
		CapturedAt:        time.Now().UTC(),
		Materials:         make([]MaterialSource, 0, len(a.Materials)),
	}
	if a.PrinterID == nil {
		config, err := core.LoadProductionCostConfig(tx)
		if err != nil {
			return source, err
		}
		source.Config = config
	} else {
		var printer models.Printer
		if err := tx.First(&printer, "id = ?", *a.PrinterID).Error; err != nil {
			return source, missing(err, "printer_id", "Printer was not found")
		}
		if printer.IsArchived {
			return source, invalid("printer_id", "Select an active printer")
		}
		source.PrinterName, source.Config = printer.Name, printer.CostConfig
	}

	for _, material := range a.Materials {
		var filament models.Filament
		if err := tx.First(&filament, "id = ?", material.FilamentID).Error; err != nil {
			return source, missing(err, "filament_id", "Filament was not found")
		}
		source.Materials = append(source.Materials, MaterialSource{
			Name:       strings.TrimSpace(fmt.Sprintf("%s %s %s", filament.Brand, filament.Type, filament.Color)),
			PricePerKg: filament.AvgPrice,
		})
	}
	return source, nil
}

func calculate(input ItemInput, source Sources) (Estimate, error) {
	a := input.Assumptions
	if source.CalculatorVersion != 1 || source.CapturedAt.IsZero() || len(source.Materials) != len(a.Materials) {
		return Estimate{}, invalid("items", "Invalid calculator provenance")
	}
	job := models.PrintJob{PrintTimeMinutes: &a.PrintTimeMinutes, LaborHours: a.LaborHours}
	if a.PrinterID != nil {
		job.PrinterCost = &models.PrinterCostSnapshot{
			Version: source.CalculatorVersion, PrinterName: source.PrinterName,
			CapturedAt: source.CapturedAt, Config: source.Config,
		}
		if err := models.ValidatePrinterCostSnapshot(job.PrinterCost); err != nil {
			return Estimate{}, invalid("printer_id", "Printer cost settings are invalid")
		}
	} else {
		for _, key := range []string{"electricity_cost_kwh", "printer_wear_cost_hour", "labor_cost_hour", "filament_waste_factor"} {
			if rate, exists := source.Config[key]; !exists || !within(rate, math.MaxFloat64) {
				return Estimate{}, invalid("items", "App cost settings are invalid")
			}
		}
	}
	for i, material := range a.Materials {
		price := source.Materials[i].PricePerKg
		if !within(price, math.MaxFloat64) {
			return Estimate{}, invalid("materials", "Filament price is invalid")
		}
		job.Filaments = append(job.Filaments, models.PrintJobFilament{
			FilamentID: material.FilamentID, WeightUsed: material.WeightG, WasteWeightG: material.WasteG,
			Filament: models.Filament{ID: material.FilamentID, AvgPrice: price},
		})
	}

	material, energy, wear, labor := core.ProductionCostParts(job, source.Config)
	parts := [4]int64{}
	for i, euros := range []float64{material, energy, wear, labor} {
		cents := math.Round(euros * 100)
		if !within(cents, maxSafeInteger) {
			return Estimate{}, invalid("items", "Calculated cost exceeds the supported range")
		}
		parts[i] = int64(cents)
	}
	unit := parts[0] + parts[1] + parts[2] + parts[3] + a.PostProcessCents + a.OtherCents
	if unit > maxSafeInteger/input.Quantity {
		return Estimate{}, invalid("items", "Line cost exceeds the supported range")
	}
	return Estimate{
		MaterialCents: parts[0], EnergyCents: parts[1], WearCents: parts[2], LaborCents: parts[3],
		PostProcessCents: a.PostProcessCents, OtherCents: a.OtherCents,
		UnitCents: unit, TotalCents: unit * input.Quantity,
	}, nil
}
