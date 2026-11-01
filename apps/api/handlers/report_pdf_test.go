package handlers

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"time"
)

func TestPDFReportPaginatesLongTables(t *testing.T) {
	request := reportRequest{mode: "report", format: "pdf", language: "en", start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), end: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
	summary := [][]string{{"category", "metric", "value", "unit", "kind"}}
	trends := [][]string{{"month", "category", "metric", "value", "unit", "kind"}}
	for i := 0; i < 120; i++ {
		summary = append(summary, []string{"materials", "consumed:Example material " + string(rune('A'+i%26)), "125.5", "g", "recorded"})
		trends = append(trends, []string{"2026-09", "materials", "consumed:Example material", "125.5", "g", "recorded"})
	}
	var output bytes.Buffer
	if err := renderReportPDF(context.Background(), &output, request, summary, trends, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	pages := regexp.MustCompile(`/Type\s*/Page\b`).FindAll(output.Bytes(), -1)
	if len(pages) < 3 {
		t.Fatalf("expected multiple pages, found %d", len(pages))
	}
	if !bytes.Contains(output.Bytes(), []byte("/Subtype /Image")) {
		t.Fatal("missing logo image in PDF")
	}
	if !bytes.Contains(output.Bytes(), []byte("/Author <FEFF")) || !bytes.Contains(output.Bytes(), []byte("/Creator <FEFF")) {
		t.Fatal("missing app identity in PDF metadata")
	}
}

func TestPDFPresentationLocalizesMetricsValuesAndDates(t *testing.T) {
	printers := map[string]string{"printer-uuid": "A1 - Office"}
	if got := pdfMetric("pt-PT", "job_count:printer-uuid", printers); got != "Trabalhos de impressão — A1 - Office" {
		t.Fatalf("printer metric: %q", got)
	}
	if got := pdfMetric("pt-PT", "print_time_minutes:printer-uuid", printers); got != "Tempo de impressão — A1 - Office" {
		t.Fatalf("printer time metric: %q", got)
	}
	if got := pdfMetric("pt-PT", "consumed:PLA", printers); got != "Material consumido — PLA" {
		t.Fatalf("material metric: %q", got)
	}
	if got := pdfMetric("pt-PT", "estimated_consumed_material_cost_current_avg_price", printers); got != "Custo estimado do material consumido (preço médio atual)" {
		t.Fatalf("cost metric: %q", got)
	}
	for _, item := range []struct{ lang, val, unit, want string }{
		{"pt-PT", "1", "jobs", "1 trabalho de impressão"}, {"pt-PT", "13", "jobs", "13 trabalhos de impressão"},
		{"en", "1", "jobs", "1 print job"}, {"en", "13", "jobs", "13 print jobs"},
		{"pt-PT", "99.00999999999999", "EUR", "99,01 €"}, {"en", "99.00999999999999", "EUR", "€99.01"},
	} {
		if got := pdfValue(item.val, item.unit, item.lang); got != item.want {
			t.Errorf("pdfValue(%q,%q,%q) = %q, want %q", item.val, item.unit, item.lang, got, item.want)
		}
	}
	if got := formatPDFDate("pt-PT", time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)); got != "27/10/2026" {
		t.Fatal(got)
	}
	if got := formatPDFMonth("pt-PT", "2026-10"); got != "outubro 2026" {
		t.Fatal(got)
	}
}

func TestPDFRendersRepresentativeRowsInBothLocales(t *testing.T) {
	printerID := "bd132746-d809-4cb6-9823-2dc1526e34c4"
	for _, language := range []string{"en", "pt-PT"} {
		request := reportRequest{mode: "report", format: "pdf", language: language, start: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), end: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
		summary := [][]string{{"category", "metric", "value", "unit", "kind"}, {"printers", "job_count:" + printerID, "1", "jobs", "recorded"}, {"materials", "estimated_consumed_material_cost_current_avg_price", "99.00999999999999", "EUR", "estimated"}}
		trends := [][]string{{"month", "category", "metric", "value", "unit", "kind"}, {"2026-10", "materials", "consumed:PLA Matte", "125.5", "g", "recorded"}}
		var output bytes.Buffer
		if err := renderReportPDF(context.Background(), &output, request, summary, trends, map[string]string{printerID: "A1 - Office"}); err != nil {
			t.Fatalf("%s render: %v", language, err)
		}
		if output.Len() == 0 {
			t.Fatalf("%s PDF is empty", language)
		}
		if bytes.Contains(output.Bytes(), []byte(printerID)) {
			t.Errorf("%s PDF exposes printer UUID", language)
		}
	}
}
