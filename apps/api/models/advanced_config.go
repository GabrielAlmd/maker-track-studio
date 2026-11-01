package models

// AdvancedConfigDefaults are virtual defaults for older databases and backups.
// They become persisted app_config rows only when the user edits them.
var AdvancedConfigDefaults = map[string]string{
	"machine_cost_mode":          "0",
	"printer_purchase_cost":      "299",
	"printer_upfront_cost":       "23",
	"printer_annual_maintenance": "30",
	"printer_lifetime_years":     "3",
	"printer_uptime_percent":     "50",
	"printer_power_watts":        "150",
	"printer_buffer_factor":      "1.3",
}
