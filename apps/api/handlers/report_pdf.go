package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/signintech/gopdf"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed assets/inter-400.ttf assets/report-logo.png
var reportAssets embed.FS

const pdfSummaryLimit = 8 << 20

type boundedReportBuffer struct{ bytes.Buffer }

func (b *boundedReportBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > pdfSummaryLimit {
		return 0, errors.New("report is too large for PDF output")
	}
	return b.Buffer.Write(p)
}

func buildReportPDF(ctx context.Context, db *gorm.DB, request reportRequest, events *diagnostics.Logger) (string, string, error) {
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
	var eventCount int
	if request.categories["events"] {
		for _, raw := range recentReportEvents(events) {
			var item struct {
				Timestamp string `json:"timestamp"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return "", "", err
			}
			stamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
			if err != nil {
				return "", "", err
			}
			if !stamp.Before(request.start) && stamp.Before(request.end) {
				eventCount++
			}
		}
	}
	var summary, trends boundedReportBuffer
	if err := writeReportSummary(ctx, readerDB.WithContext(ctx), sqlDB, &summary, request, eventCount); err != nil {
		return "", "", err
	}
	if err := writeReportTrends(ctx, sqlDB, &trends, request); err != nil {
		return "", "", err
	}
	summaryRows, err := csv.NewReader(bytes.NewReader(summary.Bytes())).ReadAll()
	if err != nil {
		return "", "", err
	}
	trendRows, err := csv.NewReader(bytes.NewReader(trends.Bytes())).ReadAll()
	if err != nil {
		return "", "", err
	}
	printerNames, err := readPDFPrinterNames(ctx, sqlDB)
	if err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp("", "report-*.pdf")
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
	if err := renderReportPDF(ctx, file, request, summaryRows, trendRows, printerNames); err != nil {
		return "", "", err
	}
	if err := file.Sync(); err != nil {
		return "", "", err
	}
	keep = true
	return file.Name(), "maker-track-report-" + time.Now().UTC().Format("20060102-150405") + ".pdf", nil
}

func readPDFPrinterNames(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM printers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}

func recentReportEvents(logger *diagnostics.Logger) []json.RawMessage {
	if logger == nil {
		return nil
	}
	return logger.Recent()
}

type pdfWords struct{ title, continuation, subtitle, summary, trends, noMetrics, noTrends, category, metric, value, basis, month, generated, page, note, recorded, estimated, snapshot string }

func reportWords(lang string) pdfWords {
	if lang == "pt-PT" {
		return pdfWords{"Relatório de produção e finanças", "Relatório (continuação)", "Maker Track Studio", "Resumo", "Tendências mensais", "Sem métricas disponíveis para este período.", "Sem atividade mensal registada neste período.", "Categoria", "Métrica", "Valor", "Base", "Mês", "Gerado", "Página", "Os custos estimados podem usar preços médios atuais. A receita representa encomendas concluídas, não pagamentos confirmados. Os eventos abrangem apenas esta execução.", "Registado", "Estimado", "Histórico"}
	}
	return pdfWords{"Production and financial report", "Report (continued)", "Maker Track Studio", "Summary", "Monthly trends", "No metrics available for this period.", "No monthly activity recorded for this period.", "Category", "Metric", "Value", "Basis", "Month", "Generated", "Page", "Estimated costs may use current average prices. Revenue is from completed orders, not proof of payment. Events cover this run only.", "Recorded", "Estimated", "Snapshot"}
}

type reportPDFLayout struct {
	pdf   gopdf.GoPdf
	y     float64
	page  int
	words pdfWords
	logo  image.Image
}

func (layout *reportPDFLayout) font(size float64, r, g, b uint8) error {
	if err := layout.pdf.SetFont("Inter", "", size); err != nil {
		return err
	}
	layout.pdf.SetTextColor(r, g, b)
	return nil
}

func (layout *reportPDFLayout) cell(x, y, width, height float64, value string) error {
	if value == "" {
		return nil
	}
	layout.pdf.SetXY(x, y)
	return layout.pdf.Cell(&gopdf.Rect{W: width, H: height}, value)
}

func (layout *reportPDFLayout) newPage() error {
	layout.pdf.AddPage()
	layout.page++
	layout.pdf.SetFillColor(15, 27, 50)
	layout.pdf.RectFromUpperLeftWithStyle(0, 0, 595, 88, "F")
	if err := layout.pdf.ImageFrom(layout.logo, 42, 9, &gopdf.Rect{W: 165, H: 68}); err != nil {
		return err
	}
	if err := layout.font(10, 132, 185, 255); err != nil {
		return err
	}
	if err := layout.cell(222, 19, 330, 15, layout.words.subtitle); err != nil {
		return err
	}
	if err := layout.font(17, 255, 255, 255); err != nil {
		return err
	}
	title := layout.words.title
	if layout.page > 1 {
		title = layout.words.continuation
	}
	width, err := layout.pdf.MeasureTextWidth(title)
	if err != nil {
		return err
	}
	if width > 330 {
		if err := layout.font(15, 255, 255, 255); err != nil {
			return err
		}
	}
	if err := layout.cell(222, 42, 330, 27, title); err != nil {
		return err
	}
	if err := layout.font(8, 95, 109, 129); err != nil {
		return err
	}
	if err := layout.cell(42, 806, 510, 16, fmt.Sprintf("%s  %d", layout.words.page, layout.page)); err != nil {
		return err
	}
	layout.y = 106
	return nil
}

func (layout *reportPDFLayout) ensure(height float64) error {
	if layout.y+height <= 790 {
		return nil
	}
	return layout.newPage()
}

func (layout *reportPDFLayout) paragraph(value string, size float64) error {
	if err := layout.font(size, 52, 65, 85); err != nil {
		return err
	}
	lines, err := layout.pdf.SplitText(value, 505)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if err := layout.ensure(16); err != nil {
			return err
		}
		if err := layout.cell(42, layout.y, 510, 15, line); err != nil {
			return err
		}
		layout.y += 15
	}
	layout.y += 8
	return nil
}

func (layout *reportPDFLayout) section(title string) error {
	if err := layout.ensure(40); err != nil {
		return err
	}
	layout.y += 12
	if err := layout.font(13, 15, 50, 99); err != nil {
		return err
	}
	if err := layout.cell(42, layout.y, 510, 22, title); err != nil {
		return err
	}
	layout.y += 28
	return nil
}

func (layout *reportPDFLayout) table(title string, headings []string, widths []float64, rows [][]string) error {
	if len(headings) != len(widths) {
		return errors.New("invalid PDF table layout")
	}
	if len(rows) == 0 {
		if err := layout.section(title); err != nil {
			return err
		}
		if title == layout.words.summary {
			return layout.paragraph(layout.words.noMetrics, 9)
		}
		return layout.paragraph(layout.words.noTrends, 9)
	}
	if err := layout.ensure(84); err != nil {
		return err
	}
	if err := layout.section(title); err != nil {
		return err
	}
	header := func() error {
		if err := layout.ensure(25); err != nil {
			return err
		}
		layout.pdf.SetFillColor(229, 237, 248)
		layout.pdf.RectFromUpperLeftWithStyle(42, layout.y, 510, 23, "F")
		if err := layout.font(8, 26, 48, 79); err != nil {
			return err
		}
		x := 42.0
		for i, heading := range headings {
			if err := layout.cell(x+5, layout.y+3, widths[i]-10, 16, heading); err != nil {
				return err
			}
			x += widths[i]
		}
		layout.y += 23
		return nil
	}
	if err := header(); err != nil {
		return err
	}
	for index, row := range rows {
		if len(row) != len(widths) {
			return errors.New("invalid PDF table row")
		}
		if err := layout.font(8, 29, 41, 61); err != nil {
			return err
		}
		linesByColumn := make([][]string, len(row))
		maxLines := 1
		for i, value := range row {
			lines, err := layout.pdf.SplitText(value, widths[i]-10)
			if err != nil {
				return err
			}
			if len(lines) > 5 {
				lines = append(append([]string{}, lines[:4]...), "…")
			}
			linesByColumn[i] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		height := float64(maxLines)*12 + 9
		if layout.y+height > 790 {
			if err := layout.newPage(); err != nil {
				return err
			}
			if err := layout.section(title); err != nil {
				return err
			}
			if err := header(); err != nil {
				return err
			}
		}
		if index%2 == 1 {
			layout.pdf.SetFillColor(245, 248, 252)
			layout.pdf.RectFromUpperLeftWithStyle(42, layout.y, 510, height, "F")
		}
		if err := layout.font(8, 29, 41, 61); err != nil {
			return err
		}
		x := 42.0
		for i, lines := range linesByColumn {
			for j, line := range lines {
				if err := layout.cell(x+5, layout.y+4+float64(j)*12, widths[i]-10, 11, line); err != nil {
					return err
				}
			}
			x += widths[i]
		}
		layout.y += height
	}
	return nil
}

func renderReportPDF(ctx context.Context, output io.Writer, request reportRequest, summaryRows, trendRows [][]string, printerNames map[string]string) error {
	fontData, err := reportAssets.ReadFile("assets/inter-400.ttf")
	if err != nil {
		return err
	}
	logoData, err := reportAssets.ReadFile("assets/report-logo.png")
	if err != nil {
		return err
	}
	logoSource, err := png.Decode(bytes.NewReader(logoData))
	if err != nil {
		return err
	}
	// The shared logo has transparent padding for the desktop sidebar.
	logo := image.NewRGBA(image.Rect(0, 0, 940, 385))
	draw.Draw(logo, logo.Bounds(), logoSource, image.Pt(280, 300), draw.Src)
	layout := reportPDFLayout{words: reportWords(request.language), logo: logo}
	layout.pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	layout.pdf.SetInfo(gopdf.PdfInfo{Title: layout.words.title, Author: "Maker Track Studio", Creator: "Maker Track Studio"})
	if err := layout.pdf.AddTTFFontData("Inter", fontData); err != nil {
		return err
	}
	if err := layout.newPage(); err != nil {
		return err
	}
	period := formatPDFDate(request.language, request.start) + " – " + formatPDFDate(request.language, request.end.AddDate(0, 0, -1)) + " (UTC)"
	if err := layout.paragraph(period, 11); err != nil {
		return err
	}
	if err := layout.paragraph(layout.words.generated+" "+formatPDFDateTime(request.language, time.Now().UTC())+" (UTC)", 9); err != nil {
		return err
	}
	if err := layout.paragraph(layout.words.note, 8); err != nil {
		return err
	}
	var summary [][]string
	for _, row := range summaryRows[1:] {
		if len(row) != 5 {
			return errors.New("invalid summary row")
		}
		summary = append(summary, []string{pdfCategory(request.language, row[0]), pdfMetric(request.language, row[1], printerNames), pdfValue(row[2], row[3], request.language), pdfBasis(layout.words, row[4])})
	}
	if err := layout.table(layout.words.summary, []string{layout.words.category, layout.words.metric, layout.words.value, layout.words.basis}, []float64{90, 225, 105, 90}, summary); err != nil {
		return err
	}
	var trends [][]string
	for _, row := range trendRows[1:] {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(row) != 6 {
			return errors.New("invalid trend row")
		}
		trends = append(trends, []string{formatPDFMonth(request.language, row[0]), pdfCategory(request.language, row[1]), pdfMetric(request.language, row[2], printerNames), pdfValue(row[3], row[4], request.language)})
	}
	if err := layout.table(layout.words.trends, []string{layout.words.month, layout.words.category, layout.words.metric, layout.words.value}, []float64{65, 100, 230, 115}, trends); err != nil {
		return err
	}
	return layout.pdf.Write(output)
}

func pdfCategory(lang, category string) string {
	labels := map[string][2]string{"print_jobs": {"Print jobs", "Impressões"}, "materials": {"Materials", "Materiais"}, "purchases": {"Purchases", "Compras"}, "sales": {"Sales & orders", "Vendas e encomendas"}, "printers": {"Printers", "Impressoras"}, "events": {"App events", "Eventos da aplicação"}}
	value, ok := labels[category]
	if !ok {
		return category
	}
	if lang == "pt-PT" {
		return value[1]
	}
	return value[0]
}

func pdfMetric(lang, metric string, printerNames map[string]string) string {
	translations := map[string][2]string{
		"job_count": {"Print jobs", "Trabalhos de impressão"}, "units_printed": {"Units printed", "Unidades impressas"},
		"print_time_minutes": {"Print time", "Tempo de impressão"}, "calculated_jobs": {"Jobs with calculated cost", "Trabalhos com custo calculado"},
		"uncalculable_jobs": {"Jobs without calculable cost", "Trabalhos sem custo calculável"}, "estimated_production_cost": {"Estimated production cost", "Custo de produção estimado"},
		"purchase_count": {"Filament purchases", "Compras de filamento"}, "filament_expenditure": {"Filament expenditure", "Despesa com filamento"},
		"filament_purchased": {"Filament purchased", "Filamento comprado"}, "equipment_expenditure": {"Equipment expenditure", "Despesa com equipamento"},
		"completed_orders": {"Completed orders", "Encomendas concluídas"}, "completed_order_revenue": {"Revenue from completed orders", "Receita de encomendas concluídas"},
		"completed_order_cost": {"Cost of completed orders", "Custo das encomendas concluídas"}, "completed_order_profit": {"Profit from completed orders", "Lucro das encomendas concluídas"},
		"completed_order_margin": {"Margin on completed orders", "Margem das encomendas concluídas"}, "recent_event_count": {"Recent application events", "Eventos recentes da aplicação"},
		"estimated_consumed_material_cost_current_avg_price": {"Estimated consumed material cost (current average price)", "Custo estimado do material consumido (preço médio atual)"},
	}
	if strings.HasPrefix(metric, "consumed:") {
		material := strings.TrimPrefix(metric, "consumed:")
		if lang == "pt-PT" {
			return "Material consumido — " + material
		}
		return "Consumed material — " + material
	}
	for _, prefix := range []string{"job_count:", "print_time_minutes:"} {
		if strings.HasPrefix(metric, prefix) {
			id := strings.TrimPrefix(metric, prefix)
			name := printerNames[id]
			if name == "" {
				name = "Unknown printer"
				if lang == "pt-PT" {
					name = "Impressora desconhecida"
				}
			}
			label := translations[strings.TrimSuffix(prefix, ":")]
			if lang == "pt-PT" {
				return label[1] + " — " + name
			}
			return label[0] + " — " + name
		}
	}
	if labels, ok := translations[metric]; ok {
		if lang == "pt-PT" {
			return labels[1]
		}
		return labels[0]
	}
	return "Other metric"
}

func pdfValue(value, unit, lang string) string {
	count, countErr := strconv.ParseInt(value, 10, 64)
	units := map[string][2]string{"jobs": {"print job", "trabalho de impressão"}, "units": {"unit", "unidade"}, "minutes": {"minute", "minuto"}, "purchases": {"purchase", "compra"}, "orders": {"order", "encomenda"}, "events": {"event", "evento"}}
	if words, ok := units[unit]; ok && countErr == nil {
		word := words[0]
		if lang == "pt-PT" {
			word = words[1]
		}
		if absInt64(count) != 1 {
			if lang == "pt-PT" && unit == "jobs" {
				word = "trabalhos de impressão"
			} else {
				word += "s"
			}
		}
		return strconv.FormatInt(count, 10) + " " + word
	}
	switch unit {
	case "EUR_cents":
		if cents, err := strconv.ParseInt(value, 10, 64); err == nil {
			return formatPDFMoney(float64(cents)/100, lang)
		}
	case "EUR":
		if amount, err := strconv.ParseFloat(value, 64); err == nil {
			return formatPDFMoney(amount, lang)
		}
	case "percent":
		if lang == "pt-PT" {
			value = strings.ReplaceAll(value, ".", ",")
		}
		return value + "%"
	case "g":
		return localizedDecimal(value, lang) + " g"
	case "kg":
		return localizedDecimal(value, lang) + " kg"
	}
	if unit == "" {
		return value
	}
	return value + " " + unit
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func formatPDFMoney(value float64, lang string) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	value = math.Round(value*100) / 100
	formatted := fmt.Sprintf("%.2f", math.Abs(value))
	if lang == "pt-PT" {
		formatted = strings.ReplaceAll(formatted, ".", ",")
		if value < 0 {
			return "-" + formatted + " €"
		}
		return formatted + " €"
	}
	if value < 0 {
		return "-€" + formatted
	}
	return "€" + formatted
}

func localizedDecimal(value, lang string) string {
	if lang == "pt-PT" {
		return strings.ReplaceAll(value, ".", ",")
	}
	return value
}

func formatPDFDate(lang string, date time.Time) string {
	if lang == "pt-PT" {
		return date.Format("02/01/2006")
	}
	return date.Format("Jan 2, 2006")
}

func formatPDFDateTime(lang string, date time.Time) string {
	if lang == "pt-PT" {
		return date.Format("02/01/2006 15:04")
	}
	return date.Format("Jan 2, 2006 15:04")
}

func formatPDFMonth(lang, month string) string {
	date, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	if lang != "pt-PT" {
		return date.Format("Jan 2006")
	}
	months := [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
	return months[int(date.Month())-1] + " " + date.Format("2006")
}

func pdfBasis(words pdfWords, value string) string {
	switch value {
	case "recorded":
		return words.recorded
	case "estimated":
		return words.estimated
	case "snapshot":
		return words.snapshot
	}
	return value
}
