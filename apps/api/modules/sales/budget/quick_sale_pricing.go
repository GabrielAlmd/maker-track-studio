package budget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type QuickSaleEstimateInput struct {
	Source          string                  `json:"source"`
	ManualUnitCents int64                   `json:"manual_unit_cents,omitempty"`
	PrinterID       *uuid.UUID              `json:"printer_id,omitempty"`
	MinutesPerUnit  int                     `json:"minutes_per_unit,omitempty"`
	LaborPerUnit    float32                 `json:"labor_hours_per_unit,omitempty"`
	FilamentMapping map[uuid.UUID]uuid.UUID `json:"filament_mapping,omitempty"`
	Target          *PriceSelection         `json:"target,omitempty"`
}

type QuickSaleEstimateRequest struct {
	ModelID        uuid.UUID              `json:"model_id"`
	StockRunItemID uuid.UUID              `json:"stock_run_item_id"`
	Quantity       int64                  `json:"quantity"`
	Cost           QuickSaleEstimateInput `json:"cost"`
}

type QuickSalePricing struct {
	Source             string          `json:"source"`
	Quantity           int64           `json:"quantity"`
	UnitCostCents      int64           `json:"unit_cost_cents"`
	TotalCostCents     int64           `json:"total_cost_cents"`
	Target             *PriceSelection `json:"target,omitempty"`
	SuggestedUnitCents *int64          `json:"suggested_unit_price_cents,omitempty"`
	Production         *Item           `json:"production,omitempty"`
}

type QuickSaleEstimateResult struct {
	QuickSalePricing
	Token string `json:"token"`
}

func previewQuickSale(db *gorm.DB, _ uuid.UUID, input QuickSaleEstimateRequest) (QuickSaleEstimateResult, error) {
	var result QuickSaleEstimateResult
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := requireQuickSaleEnabled(tx); err != nil {
			return err
		}
		var err error
		result, err = calculateQuickSalePricing(tx, input)
		return err
	})
	return result, err
}

func requireQuickSaleEnabled(db *gorm.DB) error {
	features, err := models.LoadFeatureConfig(db)
	if err != nil {
		return err
	}
	if !features["sales_orders"] {
		return conflict("business_disabled", "Enable Business Operations before using Quick Sale")
	}
	return nil
}

func calculateQuickSalePricing(tx *gorm.DB, input QuickSaleEstimateRequest) (QuickSaleEstimateResult, error) {
	var result QuickSaleEstimateResult
	if input.ModelID == uuid.Nil || input.Quantity < 1 || input.Quantity > quickSaleMaxQuantity {
		return result, invalid("quantity", "Select a model and provide a positive quantity within the supported range")
	}
	var model models.Model
	if err := tx.Take(&model, "id = ?", input.ModelID).Error; err != nil {
		return result, missing(err, "model_id", "The selected model was not found")
	}
	if input.StockRunItemID != uuid.Nil {
		if _, _, err := availableQuickSaleStock(tx, input.ModelID, input.StockRunItemID, input.Quantity); err != nil {
			return result, err
		}
	}
	cost := input.Cost
	result.Source, result.Quantity, result.Target = cost.Source, input.Quantity, cost.Target
	switch cost.Source {
	case "manual":
		if cost.PrinterID != nil || cost.MinutesPerUnit != 0 || cost.LaborPerUnit != 0 || len(cost.FilamentMapping) != 0 {
			return result, invalid("cost", "Provide only a manual unit cost for manual estimation")
		}
		result.UnitCostCents = cost.ManualUnitCents
	case "model":
		if input.StockRunItemID != uuid.Nil || cost.ManualUnitCents != 0 {
			return result, invalid("cost", "Use model estimation for made-to-order sales")
		}
		production, err := quickSaleProductionEstimate(tx, model, input.Quantity, cost)
		if err != nil {
			return result, err
		}
		result.Production, result.UnitCostCents = &production, production.Estimate.UnitCents
	default:
		return result, invalid("cost", "Choose model estimation or provide a manual cost")
	}

	if result.UnitCostCents < 1 || result.UnitCostCents > maxSafeInteger/input.Quantity {
		return result, invalid("cost", "Provide a positive cost within the supported range")
	}
	result.TotalCostCents = result.UnitCostCents * input.Quantity
	suggestion, ok := quickSaleSuggestedPrice(result.UnitCostCents, input.Quantity, cost.Target)
	if !ok {
		return result, invalid("target", "Provide a valid margin or markup within the supported range")
	}
	result.SuggestedUnitCents = suggestion

	// Ignore capture time so that confirmation can recheck the same estimate
	// inside its write transaction. The saved provenance retains capture time.
	stable := result.QuickSalePricing
	if stable.Production != nil {
		production := *stable.Production
		production.Sources.CapturedAt = time.Time{}
		stable.Production = &production
	}
	payload, err := json.Marshal(struct {
		Request QuickSaleEstimateRequest
		Pricing QuickSalePricing
	}{input, stable})
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(payload)
	result.Token = hex.EncodeToString(digest[:])
	return result, nil
}

func quickSaleSuggestedPrice(cost, quantity int64, target *PriceSelection) (*int64, bool) {
	if target == nil {
		return nil, true
	}
	if target.Mode != "margin" && target.Mode != "markup" {
		return nil, false
	}
	price, ok := unitPrice(cost, *target)
	return &price, ok && price > 0 && price <= maxSafeInteger/quantity
}

func quickSaleProductionEstimate(tx *gorm.DB, model models.Model, quantity int64, cost QuickSaleEstimateInput) (Item, error) {
	var result Item
	var requirements []models.ModelMaterialRequirement
	if err := tx.Where("model_id = ?", model.ID).Order("position").Find(&requirements).Error; err != nil {
		return result, err
	}
	missingCost := conflict("cost_unavailable", "Matching priced filaments and model material requirements are needed for an estimate")
	if len(requirements) == 0 || len(cost.FilamentMapping) > models.MaxModelMaterialRequirements {
		return result, missingCost
	}
	summary, err := models.SummarizeModelMaterials(requirements)
	if err != nil || summary.PartWeightG <= 0 {
		return result, missingCost
	}

	assumptions := Assumptions{PrinterID: cost.PrinterID, PrintTimeMinutes: cost.MinutesPerUnit, LaborHours: cost.LaborPerUnit}
	positions := map[uuid.UUID]int{}
	filaments := map[uuid.UUID]models.Filament{}
	mapped := 0
	for _, requirement := range requirements {
		weight := requirement.PartWeightG
		if requirement.WasteWeightG != nil {
			weight += *requirement.WasteWeightG
		}
		if weight == 0 {
			continue
		}
		mapped++
		id := cost.FilamentMapping[requirement.ID]
		if id == uuid.Nil {
			return result, missingCost
		}
		filament, found := filaments[id]
		if !found {
			err := tx.Take(&filament, "id = ? AND archived = ?", id, false).Error
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return result, missingCost
				}
				return result, err
			}
			filaments[id] = filament
		}
		if !strings.EqualFold(strings.TrimSpace(filament.Type), strings.TrimSpace(requirement.MaterialType)) ||
			(requirement.Color != "" && !strings.EqualFold(strings.TrimSpace(filament.Color), strings.TrimSpace(requirement.Color))) {
			return result, missingCost
		}
		position, exists := positions[id]
		if !exists {
			positions[id] = len(assumptions.Materials)
			assumptions.Materials = append(assumptions.Materials, MaterialInput{FilamentID: id, WeightG: weight, WasteG: requirement.WasteWeightG})
			continue
		}
		material := &assumptions.Materials[position]
		material.WeightG += weight
		if material.WasteG != nil || requirement.WasteWeightG != nil {
			material.WasteG = addQuickSaleWaste(material.WasteG, requirement.WasteWeightG)
		}
	}

	if mapped != len(cost.FilamentMapping) {
		return result, missingCost
	}
	item := ItemInput{ModelID: &model.ID, Name: model.Name, Quantity: quantity, Assumptions: assumptions}
	if !validItem(item) || cost.MinutesPerUnit < 1 {
		return result, invalid("cost", "Enter a positive print time and valid production assumptions")
	}
	sources, err := captureSources(tx, assumptions)
	if err != nil {
		return result, err
	}
	for _, filament := range sources.Materials {
		if filament.PricePerKg <= 0 {
			return result, missingCost
		}
	}
	estimate, err := calculate(item, sources)
	return Item{ItemInput: item, Estimate: estimate, Sources: sources}, err
}

func addQuickSaleWaste(a, b *float64) *float64 {
	var total float64
	for _, weight := range []*float64{a, b} {
		if weight != nil {
			total += *weight
		}
	}
	return &total
}

// The snapshot is validated against its frozen inputs and the confirmed line
// during backup import. Catalogue records referenced by an estimate can change.
func validQuickSalePricing(pricing *QuickSalePricing, quantity int64) bool {
	if pricing == nil {
		return true
	}
	if quantity < 1 || quantity > quickSaleMaxQuantity || pricing.Quantity != quantity ||
		pricing.UnitCostCents < 1 || pricing.UnitCostCents > maxSafeInteger/quantity ||
		pricing.TotalCostCents != pricing.UnitCostCents*quantity {
		return false
	}
	suggestion, ok := quickSaleSuggestedPrice(pricing.UnitCostCents, quantity, pricing.Target)
	if !ok || (suggestion == nil) != (pricing.SuggestedUnitCents == nil) ||
		(suggestion != nil && *suggestion != *pricing.SuggestedUnitCents) {
		return false
	}
	switch pricing.Source {
	case "manual":
		return pricing.Production == nil
	case "model":
		production := pricing.Production
		if production == nil || !validItem(production.ItemInput) || production.Quantity != quantity ||
			production.ModelID == nil || production.Assumptions.PrintTimeMinutes < 1 || len(production.Assumptions.Materials) == 0 ||
			production.Assumptions.PostProcessCents != 0 || production.Assumptions.OtherCents != 0 {
			return false
		}
		for _, filament := range production.Sources.Materials {
			if filament.PricePerKg <= 0 {
				return false
			}
		}
		expected, err := calculate(production.ItemInput, production.Sources)
		return err == nil && expected == production.Estimate && expected.UnitCents == pricing.UnitCostCents
	}
	return false
}
