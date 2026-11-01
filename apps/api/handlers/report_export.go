package handlers

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Dataset declarations are an explicit export allowlist. Adding a database
// column never makes it part of the public, versioned CSV format by accident.
type exportDataset struct {
	name, category, table, columns, dateColumn, joins string
}

var exportDatasets = []exportDataset{
	{"printers", "printers", "printers", "id,name,manufacturer,model,notes,is_default,is_archived,cost_config,created_at,updated_at", "", ""},
	{"filaments", "materials", "filaments", "id,brand,type,color,avg_price,personal_score,times_used,times_purchased,created_at,updated_at", "", ""},
	{"filament_purchases", "purchases", "filament_purchases", "id,filament_id,price,amount,purchase_date,created_at,updated_at", "purchase_date", ""},
	{"models", "print_jobs", "models", "id,name,image_url,material_weight,print_time,waste_material,is_multicolor,created_at,updated_at", "", ""},
	{"print_jobs", "print_jobs", "print_jobs", "id,printer_id,printer_cost,model_id,print_date,units_printed,labor_hours,print_time_minutes,created_at,updated_at", "print_date", ""},
	{"print_job_filaments", "materials", "print_job_filaments", "id,print_job_id,filament_id,weight_used,created_at,updated_at", "print_jobs.print_date", "JOIN print_jobs ON print_jobs.id = print_job_filaments.print_job_id"},
	{"sales_channels", "sales", "sales_channels", "id,name,notes,is_archived,created_at,updated_at", "", ""},
	{"sales_orders", "sales", "sales_orders", "id,reference,order_date,sales_channel_id,fulfilment_status,payment_status,packaging_cost_cents,shipping_cost_cents,platform_fee_cents,other_cost_cents,notes,production_cost_cents,total_cost_cents,net_profit_cents,margin_basis_points,created_at,updated_at", "order_date", ""},
	{"sales_order_items", "sales", "sales_order_items", "id,sales_order_id,model_id,model_name_snapshot,quantity,unit_price_cents,created_at,updated_at", "sales_orders.order_date", "JOIN sales_orders ON sales_orders.id = sales_order_items.sales_order_id"},
	{"sales_order_allocations", "sales", "sales_order_allocations", "id,sales_order_item_id,print_job_id,units_allocated,production_cost_cents,created_at,updated_at", "sales_orders.order_date", "JOIN sales_order_items ON sales_order_items.id = sales_order_allocations.sales_order_item_id JOIN sales_orders ON sales_orders.id = sales_order_items.sales_order_id"},
	{"app_config", "config", "app_config", "key,value,type,created_at,updated_at", "", ""},
}

var reportCategories = map[string]bool{"print_jobs": true, "materials": true, "purchases": true, "sales": true, "printers": true, "events": true}

// Set by the desktop entry point before serving requests.
var ReportApplicationVersion = "dev"

type reportRequest struct {
	mode, format, language string
	start, end             time.Time // UTC, end exclusive
	categories             map[string]bool
}

func parseReportRequest(c *gin.Context) (reportRequest, error) {
	request := reportRequest{mode: c.DefaultQuery("mode", "full"), format: c.DefaultQuery("format", "zip"), language: c.DefaultQuery("language", "en"), categories: map[string]bool{}}
	if request.format != "zip" && request.format != "pdf" {
		return request, errors.New("format must be zip or pdf")
	}
	if request.format == "pdf" && request.mode != "report" {
		return request, errors.New("PDF is only available for period reports")
	}
	if request.language != "en" && request.language != "pt-PT" {
		return request, errors.New("unsupported report language")
	}
	if request.mode == "full" {
		return request, nil
	}
	if request.mode != "report" {
		return request, errors.New("mode must be full or report")
	}
	period := c.Query("period")
	date := c.Query("date")
	var err error
	request.start, err = time.Parse("2006-01-02", date)
	if err != nil {
		return request, errors.New("date must be YYYY-MM-DD")
	}
	switch period {
	case "month":
		if request.start.Day() != 1 {
			return request, errors.New("monthly date must be the first day of a month")
		}
		request.end = request.start.AddDate(0, 1, 0)
	case "quarter":
		if request.start.Day() != 1 || (int(request.start.Month())-1)%3 != 0 {
			return request, errors.New("quarterly date must start a calendar quarter")
		}
		request.end = request.start.AddDate(0, 3, 0)
	case "year":
		if request.start.Month() != time.January || request.start.Day() != 1 {
			return request, errors.New("yearly date must be January 1")
		}
		request.end = request.start.AddDate(1, 0, 0)
	case "custom":
		request.end, err = time.Parse("2006-01-02", c.Query("end"))
		if err != nil {
			return request, errors.New("end must be YYYY-MM-DD")
		}
		request.end = request.end.AddDate(0, 0, 1)
	default:
		return request, errors.New("period must be month, quarter, year or custom")
	}
	if !request.end.After(request.start) || request.end.Sub(request.start) > 20*366*24*time.Hour {
		return request, errors.New("date range must be positive and at most 20 years")
	}
	for _, category := range strings.Split(c.Query("categories"), ",") {
		if !reportCategories[category] {
			return request, errors.New("invalid report category")
		}
		request.categories[category] = true
	}
	return request, nil
}

func GetReportExport(db *gorm.DB, events *diagnostics.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, err := parseReportRequest(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !desktopDataAvailable(db) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Reports require desktop SQLite mode"})
			return
		}
		var archive, filename, contentType string
		if request.format == "pdf" {
			archive, filename, err = buildReportPDF(c.Request.Context(), db, request, events)
			contentType = "application/pdf"
		} else {
			archive, filename, err = buildReportArchive(c.Request.Context(), db, request, events)
			contentType = "application/zip"
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate export"})
			return
		}
		defer os.Remove(archive)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", contentType)
		c.File(archive)
	}
}

type exportManifest struct {
	FormatVersion      int      `json:"format_version"`
	SchemaVersion      int      `json:"schema_version"`
	ApplicationName    string   `json:"application_name"`
	ApplicationVersion string   `json:"application_version"`
	CreatedAt          string   `json:"created_at"`
	Mode               string   `json:"mode"`
	Currency           string   `json:"currency"`
	StartDate          string   `json:"start_date,omitempty"`
	EndDate            string   `json:"end_date,omitempty"`
	DateTimezone       string   `json:"date_timezone,omitempty"`
	Datasets           []string `json:"datasets"`
	Categories         []string `json:"categories,omitempty"`
	Notes              []string `json:"notes"`
}

func buildReportArchive(ctx context.Context, db *gorm.DB, request reportRequest, events *diagnostics.Logger) (string, string, error) {
	snapshot, err := createSQLiteSnapshot(db.WithContext(ctx))
	if err != nil {
		return "", "", err
	}
	defer os.Remove(snapshot)
	readerDB, err := gorm.Open(sqlite.Open(snapshot), &gorm.Config{Logger: db.Logger})
	if err != nil {
		return "", "", err
	}
	sqlDB, err := readerDB.DB()
	if err != nil {
		return "", "", err
	}
	defer sqlDB.Close()
	file, err := os.CreateTemp("", "report-export-*.zip")
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	keep := false
	defer func() {
		if !keep {
			os.Remove(file.Name())
		}
	}()
	writer := zip.NewWriter(file)
	manifest := exportManifest{FormatVersion: 1, SchemaVersion: 1, ApplicationName: "Maker Track Studio", ApplicationVersion: ReportApplicationVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), Mode: request.mode, Currency: "EUR", Datasets: []string{}, Notes: []string{"CSV text is escaped for spreadsheet safety; remove a leading apostrophe to recover original formula-like text."}}
	if request.mode == "report" {
		manifest.StartDate = request.start.Format("2006-01-02")
		manifest.EndDate = request.end.AddDate(0, 0, -1).Format("2006-01-02")
		manifest.DateTimezone = "UTC"
		for category := range request.categories {
			manifest.Categories = append(manifest.Categories, category)
		}
		// Categories are intentionally ordered in the manifest for reproducibility.
		sort.Strings(manifest.Categories)
	}
	for _, dataset := range exportDatasets {
		if request.mode == "report" && !request.categories[dataset.category] {
			continue
		}
		if dataset.name == "app_config" && request.mode == "report" {
			continue
		}
		if err := writeDataset(ctx, sqlDB, writer, dataset, request); err != nil {
			writer.Close()
			return "", "", err
		}
		manifest.Datasets = append(manifest.Datasets, dataset.name+".csv")
	}
	if request.mode == "full" {
		if err := writeEquipmentExpenses(ctx, sqlDB, writer, request); err != nil {
			writer.Close()
			return "", "", err
		}
		manifest.Datasets = append(manifest.Datasets, "equipment_expenses.csv")
	}
	if request.mode == "report" {
		eventCount := 0
		if request.categories["events"] {
			eventCount, err = writeRecentEvents(writer, events, request)
			if err != nil {
				writer.Close()
				return "", "", err
			}
			manifest.Datasets = append(manifest.Datasets, "application_events.csv")
		}
		summaryEntry, err := writer.Create("summary.csv")
		if err != nil {
			writer.Close()
			return "", "", err
		}
		if err := writeReportSummary(ctx, readerDB.WithContext(ctx), sqlDB, summaryEntry, request, eventCount); err != nil {
			writer.Close()
			return "", "", err
		}
		manifest.Datasets = append(manifest.Datasets, "summary.csv")
		trendsEntry, err := writer.Create("trends.csv")
		if err != nil {
			writer.Close()
			return "", "", err
		}
		if err := writeReportTrends(ctx, sqlDB, trendsEntry, request); err != nil {
			writer.Close()
			return "", "", err
		}
		manifest.Datasets = append(manifest.Datasets, "trends.csv")
		if request.categories["purchases"] {
			if err := writeEquipmentExpenses(ctx, sqlDB, writer, request); err != nil {
				writer.Close()
				return "", "", err
			}
			manifest.Datasets = append(manifest.Datasets, "equipment_expenses.csv")
		}
		manifest.Notes = append(manifest.Notes, "Print jobs have no completion or outcome status. Sales use order_date and completed-order frozen costs. Application events cover only the bounded recent memory of this run; diagnostic files are excluded.")
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		writer.Close()
		return "", "", err
	}
	if err := writeZipEntry(writer, "manifest.json", content); err != nil {
		writer.Close()
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	if err := file.Sync(); err != nil {
		return "", "", err
	}
	keep = true
	return file.Name(), "maker-track-" + request.mode + "-" + time.Now().UTC().Format("20060102-150405") + ".zip", nil
}

func writeDataset(ctx context.Context, db *sql.DB, archive *zip.Writer, dataset exportDataset, request reportRequest) error {
	columns := strings.Split(dataset.columns, ",")
	selected := make([]string, len(columns))
	for i, column := range columns {
		selected[i] = dataset.table + "." + column
	}
	query := "SELECT " + strings.Join(selected, ",") + " FROM " + dataset.table + " " + dataset.joins
	args := []any{}
	if request.mode == "report" && dataset.dateColumn != "" {
		query += " WHERE " + dataset.dateColumn + " >= ? AND " + dataset.dateColumn + " < ?"
		args = append(args, request.start, request.end)
	}
	if dataset.name == "app_config" {
		query += " WHERE key IN ('features','filament_types','printer_equipment_costs','electricity_cost_kwh','printer_wear_cost_hour','labor_cost_hour','filament_waste_factor','machine_cost_mode','printer_purchase_cost','printer_upfront_cost','printer_annual_maintenance','printer_lifetime_years','printer_uptime_percent','printer_power_watts','printer_buffer_factor','margin_tier_1','margin_tier_2','margin_tier_3')"
	}
	query += " ORDER BY " + dataset.table + "." + columns[0]
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	entry, err := archive.Create(dataset.name + ".csv")
	if err != nil {
		return err
	}
	csvWriter := csv.NewWriter(entry)
	if err := csvWriter.Write(columns); err != nil {
		return err
	}
	values := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		record := make([]string, len(columns))
		for i, value := range values {
			record[i], err = exportField(value, columns[i])
			if err != nil {
				return fmt.Errorf("%s.%s: %w", dataset.name, columns[i], err)
			}
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func csvValue(value any) string {
	if value == nil {
		return ""
	}
	var text string
	switch v := value.(type) {
	case []byte:
		text = string(v)
	case time.Time:
		text = v.UTC().Format(time.RFC3339Nano)
	case int64, float64:
		return fmt.Sprint(v)
	default:
		text = fmt.Sprint(v)
	}
	trimmed := strings.TrimLeft(text, " \t\r\n\ufeff")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + text
	}
	return text
}

func exportField(value any, column string) (string, error) {
	if value == nil {
		return "", nil
	}
	if column != "created_at" && column != "updated_at" && column != "print_date" && column != "purchase_date" && column != "order_date" {
		return csvValue(value), nil
	}
	if stamp, ok := value.(time.Time); ok {
		return stamp.UTC().Format(time.RFC3339Nano), nil
	}
	raw := csvValue(value)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02"} {
		if stamp, err := time.Parse(layout, raw); err == nil {
			return stamp.UTC().Format(time.RFC3339Nano), nil
		}
	}
	return "", errors.New("invalid timestamp")
}

func writeReportSummary(ctx context.Context, gormDB *gorm.DB, db *sql.DB, output io.Writer, request reportRequest, eventCount int) error {
	csvWriter := csv.NewWriter(output)
	if err := csvWriter.Write([]string{"category", "metric", "value", "unit", "kind"}); err != nil {
		return err
	}
	add := func(category, metric string, value any, unit, kind string) error {
		return csvWriter.Write([]string{category, metric, fmt.Sprint(value), unit, kind})
	}
	if request.categories["events"] {
		if err := add("events", "recent_event_count", eventCount, "events", "recorded"); err != nil {
			return err
		}
	}
	start, end := request.start, request.end
	if request.categories["print_jobs"] || request.categories["printers"] {
		rows, err := db.QueryContext(ctx, "SELECT COALESCE(printer_id,''), COUNT(*), COALESCE(SUM(units_printed),0), COALESCE(SUM(print_time_minutes),0) FROM print_jobs WHERE print_date >= ? AND print_date < ? GROUP BY printer_id", start, end)
		if err != nil {
			return err
		}
		var jobs, units, minutes int64
		for rows.Next() {
			var id string
			var count, quantity, duration int64
			if err := rows.Scan(&id, &count, &quantity, &duration); err != nil {
				rows.Close()
				return err
			}
			jobs += count
			units += quantity
			minutes += duration
			if request.categories["printers"] {
				if err := add("printers", "job_count:"+id, count, "jobs", "recorded"); err != nil {
					rows.Close()
					return err
				}
				if err := add("printers", "print_time_minutes:"+id, duration, "minutes", "recorded"); err != nil {
					rows.Close()
					return err
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if request.categories["print_jobs"] {
			for _, item := range []struct {
				metric string
				value  int64
				unit   string
			}{{"job_count", jobs, "jobs"}, {"units_printed", units, "units"}, {"print_time_minutes", minutes, "minutes"}} {
				if err := add("print_jobs", item.metric, item.value, item.unit, "recorded"); err != nil {
					return err
				}
			}
			cfg, err := loadCostConfig(gormDB)
			if err != nil {
				return err
			}
			var estimated float64
			var calculated, unavailable int64
			var batch []models.PrintJob
			err = gormDB.Where("print_date >= ? AND print_date < ?", start, end).Preload("Model").Preload("Filaments.Filament").Order("id").FindInBatches(&batch, 200, func(_ *gorm.DB, _ int) error {
				for _, job := range batch {
					state := calculatePrintJobCostState(job, cfg)
					if state.State != "calculated" || state.Total == nil {
						unavailable++
						continue
					}
					calculated++
					estimated += *state.Total
				}
				return nil
			}).Error
			if err != nil {
				return err
			}
			if math.IsNaN(estimated) || math.IsInf(estimated, 0) {
				return errors.New("invalid estimated cost")
			}
			for _, item := range []struct {
				metric     string
				value      any
				unit, kind string
			}{{"calculated_jobs", calculated, "jobs", "estimated"}, {"uncalculable_jobs", unavailable, "jobs", "recorded"}, {"estimated_production_cost", strconv.FormatFloat(estimated, 'f', 2, 64), "EUR", "estimated"}} {
				if err := add("print_jobs", item.metric, item.value, item.unit, item.kind); err != nil {
					return err
				}
			}
		}
	}
	if request.categories["materials"] {
		rows, err := db.QueryContext(ctx, "SELECT f.type, COALESCE(SUM(pjf.weight_used),0) FROM print_job_filaments pjf JOIN print_jobs pj ON pj.id=pjf.print_job_id JOIN filaments f ON f.id=pjf.filament_id WHERE pj.print_date >= ? AND pj.print_date < ? GROUP BY f.type ORDER BY f.type", start, end)
		if err != nil {
			return err
		}
		for rows.Next() {
			var material string
			var grams float64
			if err := rows.Scan(&material, &grams); err != nil {
				rows.Close()
				return err
			}
			if err := add("materials", "consumed:"+material, strconv.FormatFloat(grams, 'f', -1, 64), "g", "recorded"); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		var materialCost float64
		if err := db.QueryRowContext(ctx, "SELECT COALESCE(SUM(pjf.weight_used*f.avg_price/1000),0) FROM print_job_filaments pjf JOIN print_jobs pj ON pj.id=pjf.print_job_id JOIN filaments f ON f.id=pjf.filament_id WHERE pj.print_date >= ? AND pj.print_date < ?", start, end).Scan(&materialCost); err != nil {
			return err
		}
		if err := add("materials", "estimated_consumed_material_cost_current_avg_price", strconv.FormatFloat(materialCost, 'f', 2, 64), "EUR", "estimated"); err != nil {
			return err
		}
	}
	if request.categories["purchases"] {
		var count int64
		var spent, kilograms float64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(price),0),COALESCE(SUM(amount),0) FROM filament_purchases WHERE purchase_date >= ? AND purchase_date < ?", start, end).Scan(&count, &spent, &kilograms); err != nil {
			return err
		}
		if err := add("purchases", "purchase_count", count, "purchases", "recorded"); err != nil {
			return err
		}
		if err := add("purchases", "filament_expenditure", strconv.FormatFloat(spent, 'f', 2, 64), "EUR", "recorded"); err != nil {
			return err
		}
		if err := add("purchases", "filament_purchased", strconv.FormatFloat(kilograms, 'f', -1, 64), "kg", "recorded"); err != nil {
			return err
		}
		expenses, err := readEquipmentExpenses(ctx, db, request)
		if err != nil {
			return err
		}
		var equipmentSpent float64
		for _, expense := range expenses {
			equipmentSpent += expense.Amount
		}
		if err := add("purchases", "equipment_expenditure", strconv.FormatFloat(equipmentSpent, 'f', 2, 64), "EUR", "recorded"); err != nil {
			return err
		}
	}
	if request.categories["sales"] {
		var missingSnapshots int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sales_orders WHERE order_date >= ? AND order_date < ? AND fulfilment_status='completed' AND (production_cost_cents IS NULL OR total_cost_cents IS NULL OR net_profit_cents IS NULL)", start, end).Scan(&missingSnapshots); err != nil {
			return err
		}
		if missingSnapshots > 0 {
			return errors.New("completed order cost snapshot is unavailable")
		}
		var count, revenue, cost, profit int64
		err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM((SELECT COALESCE(SUM(i.quantity*i.unit_price_cents),0) FROM sales_order_items i WHERE i.sales_order_id=o.id)),0), COALESCE(SUM(o.total_cost_cents),0), COALESCE(SUM(o.net_profit_cents),0) FROM sales_orders o WHERE o.order_date >= ? AND o.order_date < ? AND o.fulfilment_status='completed'`, start, end).Scan(&count, &revenue, &cost, &profit)
		if err != nil {
			return err
		}
		for _, item := range []struct {
			metric string
			value  int64
			unit   string
			kind   string
		}{{"completed_orders", count, "orders", "recorded"}, {"completed_order_revenue", revenue, "EUR_cents", "recorded"}, {"completed_order_cost", cost, "EUR_cents", "snapshot"}, {"completed_order_profit", profit, "EUR_cents", "snapshot"}} {
			if err := add("sales", item.metric, item.value, item.unit, item.kind); err != nil {
				return err
			}
		}
		if revenue > 0 {
			if err := add("sales", "completed_order_margin", strconv.FormatFloat(float64(profit)*100/float64(revenue), 'f', 2, 64), "percent", "snapshot"); err != nil {
				return err
			}
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

// Only fixed, already-sanitized fields leave the diagnostics boundary. Recent
// is bounded memory from this run; retained files are intentionally untouched.
func writeRecentEvents(archive *zip.Writer, logger *diagnostics.Logger, request reportRequest) (int, error) {
	entry, err := archive.Create("application_events.csv")
	if err != nil {
		return 0, err
	}
	writer := csv.NewWriter(entry)
	if err := writer.Write([]string{"timestamp", "level", "event", "component"}); err != nil {
		return 0, err
	}
	var recent []json.RawMessage
	if logger != nil {
		recent = logger.Recent()
	}
	count := 0
	for _, raw := range recent {
		var item struct{ Timestamp, Level, Event, Component string }
		if err := json.Unmarshal(raw, &item); err != nil {
			return 0, err
		}
		stamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
		if err != nil {
			return 0, err
		}
		if stamp.Before(request.start) || !stamp.Before(request.end) {
			continue
		}
		if err := writer.Write([]string{stamp.UTC().Format(time.RFC3339Nano), csvValue(item.Level), csvValue(item.Event), csvValue(item.Component)}); err != nil {
			return 0, err
		}
		count++
	}
	writer.Flush()
	return count, writer.Error()
}

func writeReportTrends(ctx context.Context, db *sql.DB, output io.Writer, request reportRequest) error {
	writer := csv.NewWriter(output)
	if err := writer.Write([]string{"month", "category", "metric", "value", "unit", "kind"}); err != nil {
		return err
	}
	queries := []struct{ category, query, unit, kind string }{
		{"print_jobs", "SELECT strftime('%Y-%m',print_date), 'job_count', COUNT(*) FROM print_jobs WHERE print_date >= ? AND print_date < ? GROUP BY 1 ORDER BY 1", "jobs", "recorded"},
		{"materials", "SELECT strftime('%Y-%m',pj.print_date), 'consumed:'||f.type, SUM(pjf.weight_used) FROM print_job_filaments pjf JOIN print_jobs pj ON pj.id=pjf.print_job_id JOIN filaments f ON f.id=pjf.filament_id WHERE pj.print_date >= ? AND pj.print_date < ? GROUP BY 1,2 ORDER BY 1,2", "g", "recorded"},
		{"purchases", "SELECT strftime('%Y-%m',purchase_date), 'filament_expenditure', SUM(price) FROM filament_purchases WHERE purchase_date >= ? AND purchase_date < ? GROUP BY 1 ORDER BY 1", "EUR", "recorded"},
		{"sales", "SELECT strftime('%Y-%m',o.order_date), 'completed_order_revenue', SUM((SELECT COALESCE(SUM(i.quantity*i.unit_price_cents),0) FROM sales_order_items i WHERE i.sales_order_id=o.id)) FROM sales_orders o WHERE o.order_date >= ? AND o.order_date < ? AND o.fulfilment_status='completed' GROUP BY 1 ORDER BY 1", "EUR_cents", "recorded"},
		{"sales", "SELECT strftime('%Y-%m',order_date), 'completed_order_cost', SUM(total_cost_cents) FROM sales_orders WHERE order_date >= ? AND order_date < ? AND fulfilment_status='completed' GROUP BY 1 ORDER BY 1", "EUR_cents", "snapshot"},
		{"sales", "SELECT strftime('%Y-%m',order_date), 'completed_order_profit', SUM(net_profit_cents) FROM sales_orders WHERE order_date >= ? AND order_date < ? AND fulfilment_status='completed' GROUP BY 1 ORDER BY 1", "EUR_cents", "snapshot"},
		{"printers", "SELECT strftime('%Y-%m',print_date), 'job_count:'||COALESCE(printer_id,''), COUNT(*) FROM print_jobs WHERE print_date >= ? AND print_date < ? GROUP BY 1,2 ORDER BY 1,2", "jobs", "recorded"},
	}
	for _, item := range queries {
		if !request.categories[item.category] {
			continue
		}
		rows, err := db.QueryContext(ctx, item.query, request.start, request.end)
		if err != nil {
			return err
		}
		for rows.Next() {
			var month, metric string
			var value any
			if err := rows.Scan(&month, &metric, &value); err != nil {
				rows.Close()
				return err
			}
			if err := writer.Write([]string{month, item.category, csvValue(metric), csvValue(value), item.unit, item.kind}); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if request.categories["purchases"] {
		expenses, err := readEquipmentExpenses(ctx, db, request)
		if err != nil {
			return err
		}
		monthly := map[string]float64{}
		for _, expense := range expenses {
			monthly[expense.Date[:7]] += expense.Amount
		}
		months := make([]string, 0, len(monthly))
		for month := range monthly {
			months = append(months, month)
		}
		sort.Strings(months)
		for _, month := range months {
			if err := writer.Write([]string{month, "purchases", "equipment_expenditure", strconv.FormatFloat(monthly[month], 'f', 2, 64), "EUR", "recorded"}); err != nil {
				return err
			}
		}
	}
	writer.Flush()
	return writer.Error()
}

type reportEquipmentExpense struct {
	Description string  `json:"description"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
}

func readEquipmentExpenses(ctx context.Context, db *sql.DB, request reportRequest) ([]reportEquipmentExpense, error) {
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM app_config WHERE key = ?", equipmentCostsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []reportEquipmentExpense
	if err := json.Unmarshal([]byte(raw), &all); err != nil {
		return nil, err
	}
	selected := make([]reportEquipmentExpense, 0)
	for _, expense := range all {
		if request.mode == "full" {
			selected = append(selected, expense)
			continue
		}
		date, err := time.Parse("2006-01-02", expense.Date)
		if err != nil || date.Before(request.start) || !date.Before(request.end) {
			continue
		}
		selected = append(selected, expense)
	}
	return selected, nil
}

func writeEquipmentExpenses(ctx context.Context, db *sql.DB, archive *zip.Writer, request reportRequest) error {
	expenses, err := readEquipmentExpenses(ctx, db, request)
	if err != nil {
		return err
	}
	entry, err := archive.Create("equipment_expenses.csv")
	if err != nil {
		return err
	}
	writer := csv.NewWriter(entry)
	if err := writer.Write([]string{"report_index", "description", "date", "amount", "category"}); err != nil {
		return err
	}
	for i, expense := range expenses {
		if err := writer.Write([]string{strconv.Itoa(i), csvValue(expense.Description), expense.Date, strconv.FormatFloat(expense.Amount, 'f', 2, 64), csvValue(expense.Category)}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
