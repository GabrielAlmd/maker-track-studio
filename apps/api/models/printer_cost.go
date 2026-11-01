package models

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PrinterCostConfigFromAppConfig builds the template copied when a physical
// printer is created. AppConfig is deliberately not consulted after that:
// each Printer owns its live cost configuration and PrintJob snapshots own
// the historical configuration used for an individual job.
func PrinterCostConfigFromAppConfig(rows []AppConfig) map[string]float64 {
	config := map[string]float64{
		"electricity_cost_kwh": 0, "printer_wear_cost_hour": 0,
		"labor_cost_hour": 0, "filament_waste_factor": 1,
	}
	for key, value := range AdvancedConfigDefaults {
		config[key], _ = strconv.ParseFloat(value, 64)
	}
	for _, row := range rows {
		value, err := strconv.ParseFloat(strings.TrimSpace(row.Value), 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
			if _, known := config[row.Key]; known {
				config[row.Key] = value
			}
		}
	}
	if config["filament_waste_factor"] == 0 {
		config["filament_waste_factor"] = 1
	}
	return config
}

// PrinterCostSnapshot freezes calculation inputs, not material purchase prices.
// Version 1 uses the existing simple/advanced cost formulas.
type PrinterCostSnapshot struct {
	Version     int                `json:"version"`
	PrinterName string             `json:"printer_name"`
	CapturedAt  time.Time          `json:"captured_at"`
	Config      map[string]float64 `json:"config"`
}

func IsLegacyMarginTierKey(key string) bool {
	switch key {
	case "margin_tier_1", "margin_tier_2", "margin_tier_3":
		return true
	default:
		return false
	}
}

func ValidatePrinterCostConfig(config map[string]float64) error {
	required := map[string]bool{
		"electricity_cost_kwh": true, "printer_wear_cost_hour": true,
		"labor_cost_hour": true, "filament_waste_factor": true,
	}
	for key := range AdvancedConfigDefaults {
		required[key] = true
	}
	for key := range required {
		if _, exists := config[key]; !exists {
			return fmt.Errorf("provide all supported printer cost inputs")
		}
	}
	for key, v := range config {
		if !required[key] && !IsLegacyMarginTierKey(key) {
			return fmt.Errorf("invalid printer cost input: %s", key)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e9 {
			return fmt.Errorf("invalid printer cost input: %s", key)
		}
	}
	if config["machine_cost_mode"] != 0 && config["machine_cost_mode"] != 1 {
		return fmt.Errorf("mode must be 0 or 1")
	}
	if config["printer_lifetime_years"] < 0.001 || config["printer_uptime_percent"] < 0.001 || config["printer_uptime_percent"] > 100 {
		return fmt.Errorf("invalid printer lifetime or uptime")
	}
	if config["printer_buffer_factor"] < 1 {
		return fmt.Errorf("buffer must be at least 1")
	}
	return nil
}

func ValidatePrinterCostSnapshot(snapshot *PrinterCostSnapshot) error {
	if snapshot == nil || snapshot.Version != 1 || snapshot.CapturedAt.IsZero() || strings.TrimSpace(snapshot.PrinterName) == "" {
		return fmt.Errorf("invalid printer cost snapshot")
	}
	return ValidatePrinterCostConfig(snapshot.Config)
}

// MigrateLegacyPrintJobCosts initializes the printer-profile side of the
// migration without rewriting historical Print Jobs. Legacy jobs deliberately
// keep their nil assignment/snapshot and continue to use global AppConfig
// values at runtime. It is called during startup and version-1 backup import.
func MigrateLegacyPrintJobCosts(tx *gorm.DB) error {
	var jobs []PrintJob
	if err := tx.Where("printer_id IS NULL OR printer_cost IS NULL").Find(&jobs).Error; err != nil {
		return err
	}

	var configs []AppConfig
	if err := tx.Find(&configs).Error; err != nil {
		return err
	}
	template := PrinterCostConfigFromAppConfig(configs)
	if err := ValidatePrinterCostConfig(template); err != nil {
		return err
	}

	var printers []Printer
	if err := tx.Find(&printers).Error; err != nil {
		return err
	}
	byID := make(map[string]*Printer, len(printers))
	var defaultPrinter *Printer
	for i := range printers {
		printer := &printers[i]
		if printer.CostConfig == nil {
			printer.CostConfig = clonePrinterCostConfig(template)
			if err := tx.Save(printer).Error; err != nil {
				return err
			}
		}
		if err := ValidatePrinterCostConfig(printer.CostConfig); err != nil {
			return err
		}
		byID[printer.ID.String()] = printer
		if printer.IsDefault && !printer.IsArchived {
			defaultPrinter = printer
		}
	}
	if defaultPrinter == nil {
		migrated := Printer{
			Name: "Migrated default printer", Manufacturer: "Migrated", Model: "Existing setup",
			Notes: "Created during the printer cost migration.", IsDefault: true,
			CostConfig: clonePrinterCostConfig(template),
		}
		if err := tx.Create(&migrated).Error; err != nil {
			return err
		}
		defaultPrinter = &migrated
		byID[migrated.ID.String()] = defaultPrinter
	}

	for i := range jobs {
		job := &jobs[i]
		if job.PrinterID != nil {
			_, exists := byID[job.PrinterID.String()]
			if !exists {
				return fmt.Errorf("print job references a missing printer")
			}
		}
		if job.PrinterID == nil && job.PrinterCost != nil {
			return fmt.Errorf("print job has an unassigned printer cost snapshot")
		}
		if job.PrinterCost != nil {
			if err := ValidatePrinterCostSnapshot(job.PrinterCost); err != nil {
				return err
			}
		}
	}
	return nil
}

func clonePrinterCostConfig(config map[string]float64) map[string]float64 {
	clone := make(map[string]float64, len(config))
	for key, value := range config {
		clone[key] = value
	}
	return clone
}
