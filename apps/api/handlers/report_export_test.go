package handlers_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
)

func reportEntries(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = data
	}
	return entries
}

func reportRows(t *testing.T, data []byte) [][]string {
	t.Helper()
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestFullCSVExportPreservesRelationshipsAndExcludesUnmanagedConfig(t *testing.T) {
	r, db := setupDesktopTestRouter(t)
	filament := models.Filament{Brand: "=HYPERLINK(\"x\")", Type: "PLA", Color: "Blue", AvgPrice: 24}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	job := models.PrintJob{PrintDate: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC), UnitsPrinted: 2}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	usage := models.PrintJobFilament{PrintJobID: job.ID, FilamentID: filament.ID, WeightUsed: 12.5}
	if err := db.Create(&usage).Error; err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []models.AppConfig{{Key: "labor_cost_hour", Value: "15", Type: "number"}, {Key: "auth_token", Value: "SECRET", Type: "text"}} {
		if err := db.Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
	}
	res := performRequest(t, r, http.MethodGet, "/reports/export?mode=full", nil)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	if res.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("content type: %q", res.Header().Get("Content-Type"))
	}
	entries := reportEntries(t, res.Body.Bytes())
	rows := reportRows(t, entries["print_job_filaments.csv"])
	if len(rows) != 2 || rows[1][0] != usage.ID.String() || rows[1][1] != job.ID.String() || rows[1][2] != filament.ID.String() {
		t.Fatalf("relationships: %v", rows)
	}
	if _, err := time.Parse(time.RFC3339Nano, reportRows(t, entries["print_jobs.csv"])[1][4]); err != nil {
		t.Fatalf("print date is not ISO 8601: %v", err)
	}
	filamentRows := reportRows(t, entries["filaments.csv"])
	if filamentRows[1][1] != "'"+filament.Brand {
		t.Fatalf("unsafe CSV text: %q", filamentRows[1][1])
	}
	if strings.Contains(string(entries["app_config.csv"]), "SECRET") || !strings.Contains(string(entries["app_config.csv"]), "labor_cost_hour") {
		t.Fatal("config allowlist failed")
	}
	var manifest struct {
		FormatVersion   int      `json:"format_version"`
		ApplicationName string   `json:"application_name"`
		Datasets        []string `json:"datasets"`
	}
	if err := json.Unmarshal(entries["manifest.json"], &manifest); err != nil || manifest.FormatVersion != 1 || manifest.ApplicationName != "Maker Track Studio" || len(manifest.Datasets) != 12 {
		t.Fatalf("manifest: %+v %v", manifest, err)
	}
}

func TestFinancialReportUsesFrozenOrderCostsAndDatedEquipment(t *testing.T) {
	r, db := setupDesktopTestRouter(t)
	production, total, profit := int64(700), int64(1000), int64(1500)
	order := salesmodels.SalesOrder{Reference: "SALE-1", OrderDate: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), FulfilmentStatus: salesmodels.SalesOrderFulfilmentCompleted, PaymentStatus: salesmodels.SalesOrderPaymentPaid, ProductionCostCents: &production, TotalCostCents: &total, NetProfitCents: &profit}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []salesmodels.SalesOrderItem{{SalesOrderID: order.ID, ModelNameSnapshot: "A", Quantity: 2, UnitPriceCents: 1000}, {SalesOrderID: order.ID, ModelNameSnapshot: "B", Quantity: 1, UnitPriceCents: 500}} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AppConfig{Key: "printer_equipment_costs", Value: `[{"description":"=HYPERLINK(1)","date":"2026-09-12","amount":29.9,"category":"spare_parts"},{"description":"Outside","date":"2026-08-12","amount":40,"category":"other"}]`, Type: "json"}).Error; err != nil {
		t.Fatal(err)
	}
	res := performRequest(t, r, http.MethodGet, "/reports/export?mode=report&period=month&date=2026-09-01&categories=sales,purchases", nil)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	entries := reportEntries(t, res.Body.Bytes())
	summary := string(entries["summary.csv"])
	for _, expected := range []string{"completed_orders,1,orders", "completed_order_revenue,2500,EUR_cents", "completed_order_cost,1000,EUR_cents", "completed_order_profit,1500,EUR_cents", "equipment_expenditure,29.90,EUR"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("missing %s in %s", expected, summary)
		}
	}
	trends := string(entries["trends.csv"])
	for _, expected := range []string{"2026-09,sales,completed_order_revenue,2500,EUR_cents", "2026-09,purchases,equipment_expenditure,29.90,EUR"} {
		if !strings.Contains(trends, expected) {
			t.Fatalf("missing %s in %s", expected, trends)
		}
	}
	expenses := reportRows(t, entries["equipment_expenses.csv"])
	if len(expenses) != 2 || expenses[1][1] != "'=HYPERLINK(1)" {
		t.Fatalf("equipment rows: %v", expenses)
	}
}

func TestPeriodReportIncludesBoundariesAndRejectsInvalidRanges(t *testing.T) {
	r, db := setupDesktopTestRouter(t)
	for _, date := range []time.Time{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)} {
		if err := db.Create(&models.PrintJob{PrintDate: date, UnitsPrinted: 2}).Error; err != nil {
			t.Fatal(err)
		}
	}
	res := performRequest(t, r, http.MethodGet, "/reports/export?mode=report&period=month&date=2026-09-01&categories=print_jobs", nil)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	entries := reportEntries(t, res.Body.Bytes())
	if len(reportRows(t, entries["print_jobs.csv"])) != 2 || strings.Contains(string(entries["summary.csv"]), "job_count,2") {
		t.Fatalf("wrong report rows: %s", entries["summary.csv"])
	}
	if !strings.Contains(string(entries["summary.csv"]), "job_count,1,jobs") {
		t.Fatalf("wrong job count: %s", entries["summary.csv"])
	}
	for _, query := range []string{"period=custom&date=2026-10-02&end=2026-10-01", "period=quarter&date=2026-02-01", "period=month&date=2026-09-01&categories=unknown"} {
		bad := performRequest(t, r, http.MethodGet, "/reports/export?mode=report&"+query, nil)
		if bad.Code != 400 {
			t.Fatalf("invalid query accepted: %s", query)
		}
	}
}

func TestEmptyReportStillHasUsableCSVHeaders(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	res := performRequest(t, r, http.MethodGet, "/reports/export?mode=report&period=year&date=2025-01-01&categories=purchases,sales", nil)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	entries := reportEntries(t, res.Body.Bytes())
	if len(reportRows(t, entries["filament_purchases.csv"])) != 1 || len(reportRows(t, entries["equipment_expenses.csv"])) != 1 || len(reportRows(t, entries["trends.csv"])) != 1 {
		t.Fatal("empty datasets lost headers")
	}
	if !strings.Contains(string(entries["summary.csv"]), "completed_orders,0,orders") {
		t.Fatal("empty sales summary missing")
	}
}

func TestRecentEventsExportContainsOnlySafeFields(t *testing.T) {
	r, db := setupDesktopTestRouter(t)
	logger, err := diagnostics.New(diagnostics.DefaultConfig(), diagnostics.Identity{Version: "dev"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close(context.Background()) })
	if !logger.Emit(diagnostics.Input{Event: diagnostics.AppReady}) {
		t.Fatal("event not admitted")
	}
	r.GET("/reports/with-events", handlers.GetReportExport(db, logger))
	month := time.Now().UTC().Format("2006-01") + "-01"
	res := performRequest(t, r, http.MethodGet, "/reports/with-events?mode=report&period=month&date="+month+"&categories=events", nil)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	entries := reportEntries(t, res.Body.Bytes())
	rows := reportRows(t, entries["application_events.csv"])
	if len(rows) != 2 || rows[1][2] != string(diagnostics.AppReady) {
		t.Fatalf("event rows: %v", rows)
	}
	if strings.Contains(string(entries["application_events.csv"]), "message") || strings.Contains(string(entries["application_events.csv"]), "run_id") {
		t.Fatal("sensitive diagnostics leaked")
	}
	if !strings.Contains(string(entries["summary.csv"]), "recent_event_count,1,events") {
		t.Fatal("event count missing")
	}
}

func TestPDFReportUsesPeriodValidationAndReturnsDocument(t *testing.T) {
	r, db := setupDesktopTestRouter(t)
	if err := db.Create(&models.PrintJob{PrintDate: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC), UnitsPrinted: 2}).Error; err != nil {
		t.Fatal(err)
	}
	response := performRequest(t, r, http.MethodGet, "/reports/export?mode=report&format=pdf&period=month&date=2026-09-01&categories=print_jobs&language=pt-PT", nil)
	if response.Code != 200 {
		t.Fatalf("PDF response: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("PDF content type: %q", response.Header().Get("Content-Type"))
	}
	data := response.Body.Bytes()
	if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data, []byte("%%EOF")) {
		t.Fatalf("invalid PDF: %q", data[:min(len(data), 20)])
	}
	for _, query := range []string{"mode=full&format=pdf", "mode=report&format=pdf&period=custom&date=2026-09-02&end=2026-09-01&categories=print_jobs", "mode=report&format=pdf&period=month&date=2026-09-01&categories=print_jobs&language=unknown"} {
		bad := performRequest(t, r, http.MethodGet, "/reports/export?"+query, nil)
		if bad.Code != 400 {
			t.Fatalf("invalid PDF query accepted: %s", query)
		}
	}
}
