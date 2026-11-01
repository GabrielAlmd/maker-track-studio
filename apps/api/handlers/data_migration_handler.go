package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const (
	migrationMaxFileBytes = 10 << 20
	migrationMaxRows      = 5000
	migrationMaxColumns   = 100
)

type migrationField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Aliases  []string `json:"aliases"`
	Example  string   `json:"example"`
}

type migrationSectionDefinition struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Fields []migrationField `json:"fields"`
}

var migrationSections = []migrationSectionDefinition{
	{Key: "printers", Label: "Printers", Fields: []migrationField{
		{Key: "name", Label: "Name", Required: true, Aliases: []string{"printer", "printer_name", "nickname"}, Example: "Workshop A1"},
		{Key: "manufacturer", Label: "Manufacturer", Required: true, Aliases: []string{"brand", "make"}, Example: "Bambu Lab"},
		{Key: "model", Label: "Model", Required: true, Aliases: []string{"printer_model", "machine_model"}, Example: "A1"},
		{Key: "notes", Label: "Notes", Aliases: []string{"description", "comments"}, Example: "Primary printer"},
	}},
	{Key: "filaments", Label: "Filaments", Fields: []migrationField{
		{Key: "brand", Label: "Brand", Required: true, Aliases: []string{"manufacturer"}, Example: "Bambu Lab"},
		{Key: "type", Label: "Type", Required: true, Aliases: []string{"material", "material_type"}, Example: "PLA"},
		{Key: "color", Label: "Color", Required: true, Aliases: []string{"colour"}, Example: "Black"},
		{Key: "avg_price", Label: "Average price per kg", Required: true, Aliases: []string{"average_price", "price_per_kg", "price"}, Example: "24.99"},
		{Key: "personal_score", Label: "Personal score", Aliases: []string{"rating", "score"}, Example: "4"},
		{Key: "times_used", Label: "Times used", Aliases: []string{"uses"}, Example: "0"},
		{Key: "times_purchased", Label: "Times purchased", Aliases: []string{"purchases"}, Example: "1"},
	}},
	{Key: "purchases", Label: "Purchases", Fields: []migrationField{
		{Key: "filament", Label: "Filament", Required: true, Aliases: []string{"filament_name", "material"}, Example: "Bambu Lab | PLA | Black"},
		{Key: "price", Label: "Price", Required: true, Aliases: []string{"total_price", "cost"}, Example: "24.99"},
		{Key: "amount", Label: "Amount (kg)", Required: true, Aliases: []string{"amount_kg", "weight_kg", "quantity"}, Example: "1"},
		{Key: "purchase_date", Label: "Purchase date", Required: true, Aliases: []string{"date", "bought_at"}, Example: "2026-10-02"},
	}},
	{Key: "models", Label: "Models", Fields: []migrationField{
		{Key: "name", Label: "Name", Required: true, Aliases: []string{"model", "model_name"}, Example: "Desk organiser"},
		{Key: "material_weight", Label: "Material weight (g)", Required: true, Aliases: []string{"weight", "weight_g", "material_g"}, Example: "125"},
		{Key: "print_time", Label: "Print time (min)", Required: true, Aliases: []string{"print_time_minutes", "duration_minutes", "duration"}, Example: "180"},
		{Key: "waste_material", Label: "Waste material (g)", Aliases: []string{"waste", "waste_g"}, Example: "5"},
		{Key: "is_multicolor", Label: "Multicolor", Aliases: []string{"multicolor", "multi_color", "multi_colour"}, Example: "false"},
		{Key: "image_url", Label: "Image URL", Aliases: []string{"image", "photo_url"}, Example: ""},
	}},
	{Key: "print_jobs", Label: "Print Jobs", Fields: []migrationField{
		{Key: "printer", Label: "Printer", Required: true, Aliases: []string{"printer_name", "machine"}, Example: "Workshop A1"},
		{Key: "model", Label: "Model", Aliases: []string{"model_name", "item"}, Example: "Desk organiser"},
		{Key: "print_date", Label: "Print date", Required: true, Aliases: []string{"date", "printed_at"}, Example: "2026-10-02"},
		{Key: "units_printed", Label: "Units printed", Required: true, Aliases: []string{"units", "quantity"}, Example: "1"},
		{Key: "labor_hours", Label: "Labor hours", Required: true, Aliases: []string{"labour_hours", "work_hours"}, Example: "0.5"},
		{Key: "print_time_minutes", Label: "Print time (min)", Aliases: []string{"print_time", "duration_minutes"}, Example: "180"},
		{Key: "filament", Label: "Filament", Aliases: []string{"filament_name", "material"}, Example: "Bambu Lab | PLA | Black"},
		{Key: "filament_weight_g", Label: "Filament used (g)", Aliases: []string{"filament_weight", "weight_used", "material_g"}, Example: "125"},
	}},
}

type migrationSource struct {
	Name     string     `json:"name"`
	Columns  []string   `json:"columns"`
	Rows     [][]string `json:"-"`
	Samples  [][]string `json:"sample_rows"`
	RowCount int        `json:"row_count"`
}

type migrationInspectResponse struct {
	Format  string            `json:"format"`
	Sources []migrationSource `json:"sources"`
}

type migrationPlan struct {
	Sections []migrationSectionPlan `json:"sections"`
}

type migrationSectionPlan struct {
	Section           string            `json:"section"`
	Source            string            `json:"source"`
	Mapping           map[string]string `json:"mapping"`
	Defaults          map[string]string `json:"defaults"`
	DuplicateStrategy string            `json:"duplicate_strategy"`
}

type migrationIssue struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type migrationPreviewRow struct {
	Row      int               `json:"row"`
	Status   string            `json:"status"`
	Action   string            `json:"action"`
	Values   map[string]string `json:"values"`
	Issues   []migrationIssue  `json:"issues"`
	prepared any
	existing uuid.UUID
	provided map[string]bool
}

type migrationSummary struct {
	Total      int `json:"total"`
	Ready      int `json:"ready"`
	Warnings   int `json:"warnings"`
	Invalid    int `json:"invalid"`
	Duplicates int `json:"duplicates"`
	Skipped    int `json:"skipped"`
}

type migrationSectionPreview struct {
	Section          string                `json:"section"`
	Source           string                `json:"source"`
	EffectiveMapping map[string]string     `json:"effective_mapping"`
	Rows             []migrationPreviewRow `json:"rows"`
	Summary          migrationSummary      `json:"summary"`
}

type migrationPreviewResponse struct {
	CanImport bool                      `json:"can_import"`
	Sections  []migrationSectionPreview `json:"sections"`
}

type preparedPurchase struct {
	ID         uuid.UUID
	FilamentID uuid.UUID
	Price      float64
	Amount     float64
	Date       time.Time
}

type preparedPrintJob struct {
	Job            models.PrintJob
	FilamentID     *uuid.UUID
	FilamentWeight float64
}

type migrationLookups struct {
	printers       map[string][]uuid.UUID
	activePrinters map[string][]uuid.UUID
	filaments      map[string][]uuid.UUID
	models         map[string][]uuid.UUID
	purchases      map[string][]uuid.UUID
	jobs           map[string][]uuid.UUID
}

func GetDataMigrationSchema(_ *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"sections": migrationSections}) }
}

func GetDataMigrationTemplate(_ *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		definition, ok := migrationDefinition(c.Param("section"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "Unsupported migration section"})
			return
		}
		var output bytes.Buffer
		output.WriteString("\xef\xbb\xbf")
		writer := csv.NewWriter(&output)
		headers, example := make([]string, 0, len(definition.Fields)), make([]string, 0, len(definition.Fields))
		for _, field := range definition.Fields {
			headers, example = append(headers, field.Key), append(example, field.Example)
		}
		_ = writer.Write(headers)
		_ = writer.Write(example)
		writer.Flush()
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="makertrack-%s-template.csv"`, definition.Key))
		c.Data(http.StatusOK, "text/csv; charset=utf-8", output.Bytes())
	}
}

func GetDataMigrationWorkbookTemplate(_ *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		book := excelize.NewFile()
		defer book.Close()
		for index, definition := range migrationSections {
			if index == 0 {
				if err := book.SetSheetName("Sheet1", definition.Key); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create the workbook template"})
					return
				}
			} else if _, err := book.NewSheet(definition.Key); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create the workbook template"})
				return
			}
			for column, field := range definition.Fields {
				cell, err := excelize.CoordinatesToCellName(column+1, 1)
				if err == nil {
					err = book.SetCellStr(definition.Key, cell, field.Key)
				}
				if err == nil {
					cell, err = excelize.CoordinatesToCellName(column+1, 2)
				}
				if err == nil {
					err = book.SetCellStr(definition.Key, cell, field.Example)
				}
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create the workbook template"})
					return
				}
			}
		}
		var output bytes.Buffer
		if err := book.Write(&output); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create the workbook template"})
			return
		}
		c.Header("Content-Disposition", `attachment; filename="makertrack-migration-template.xlsx"`)
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", output.Bytes())
	}
}

func InspectDataMigration(_ *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		parsed, err := readMigrationUpload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, parsed)
	}
}

func PreviewDataMigration(db *gorm.DB) gin.HandlerFunc {
	return migrationPreviewOrImport(db, false)
}

func ImportDataMigration(db *gorm.DB) gin.HandlerFunc {
	return migrationPreviewOrImport(db, true)
}

func migrationPreviewOrImport(db *gorm.DB, commit bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		parsed, err := readMigrationUpload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var plan migrationPlan
		planJSON := c.PostForm("plan")
		if len(planJSON) == 0 || len(planJSON) > 64<<10 || json.Unmarshal([]byte(planJSON), &plan) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid migration plan"})
			return
		}
		preview, err := prepareMigration(db, parsed, plan)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !commit {
			c.JSON(http.StatusOK, preview)
			return
		}
		if !preview.CanImport {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Resolve invalid rows before importing", "preview": preview})
			return
		}
		counts, err := commitMigration(db, preview)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Migration failed; no rows were imported"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"imported": counts, "preview": preview})
	}
}

func readMigrationUpload(c *gin.Context) (migrationInspectResponse, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, migrationMaxFileBytes+(1<<20))
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return migrationInspectResponse{}, errors.New("Choose a CSV or XLSX file")
	}
	defer file.Close()
	if header.Size > migrationMaxFileBytes {
		return migrationInspectResponse{}, errors.New("Migration file must not exceed 10 MB")
	}
	data, err := io.ReadAll(io.LimitReader(file, migrationMaxFileBytes+1))
	if err != nil || len(data) > migrationMaxFileBytes {
		return migrationInspectResponse{}, errors.New("Could not read the migration file")
	}
	extension := strings.ToLower(filepath.Ext(header.Filename))
	switch extension {
	case ".csv":
		source, err := parseMigrationCSV(bytes.NewReader(data), strings.TrimSuffix(filepath.Base(header.Filename), extension))
		if err != nil {
			return migrationInspectResponse{}, err
		}
		return migrationInspectResponse{Format: "csv", Sources: []migrationSource{source}}, nil
	case ".xlsx":
		sources, err := parseMigrationXLSX(bytes.NewReader(data))
		if err != nil {
			return migrationInspectResponse{}, err
		}
		return migrationInspectResponse{Format: "xlsx", Sources: sources}, nil
	default:
		return migrationInspectResponse{}, errors.New("Only CSV and XLSX files are supported")
	}
}

func parseMigrationCSV(reader io.Reader, name string) (migrationSource, error) {
	parser := csv.NewReader(reader)
	parser.FieldsPerRecord = -1
	parser.ReuseRecord = false
	records := make([][]string, 0)
	for len(records) <= migrationMaxRows+1 {
		record, err := parser.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return migrationSource{}, errors.New("CSV file could not be parsed")
		}
		if len(record) > migrationMaxColumns {
			return migrationSource{}, errors.New("Migration files may contain at most 100 columns")
		}
		for i := range record {
			record[i] = strings.TrimSpace(strings.TrimPrefix(record[i], "\ufeff"))
		}
		records = append(records, record)
	}
	if len(records) > migrationMaxRows+1 {
		return migrationSource{}, errors.New("Migration files may contain at most 5,000 rows")
	}
	return migrationSourceFromRows(name, records)
}

func parseMigrationXLSX(reader io.Reader) ([]migrationSource, error) {
	book, err := excelize.OpenReader(reader, excelize.Options{
		RawCellValue:      false,
		UnzipSizeLimit:    50 << 20,
		UnzipXMLSizeLimit: 10 << 20,
	})
	if err != nil {
		return nil, errors.New("XLSX file could not be parsed")
	}
	defer book.Close()
	if len(book.GetSheetList()) == 0 {
		return nil, errors.New("The XLSX workbook has no worksheets")
	}
	sources := make([]migrationSource, 0, len(book.GetSheetList()))
	for _, sheet := range book.GetSheetList() {
		rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: false})
		if err != nil {
			return nil, errors.New("XLSX worksheet could not be read")
		}
		if len(rows) > migrationMaxRows+1 {
			return nil, errors.New("Migration worksheets may contain at most 5,000 data rows")
		}
		for i := range rows {
			if len(rows[i]) > migrationMaxColumns {
				return nil, errors.New("Migration files may contain at most 100 columns")
			}
			for j := range rows[i] {
				rows[i][j] = strings.TrimSpace(rows[i][j])
			}
		}
		source, sourceErr := migrationSourceFromRows(sheet, rows)
		if sourceErr == nil {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("The XLSX workbook contains no tabular worksheets")
	}
	return sources, nil
}

func migrationSourceFromRows(name string, rows [][]string) (migrationSource, error) {
	for len(rows) > 0 && rowBlank(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) == 0 || rowBlank(rows[0]) {
		return migrationSource{}, errors.New("Migration source requires a header row")
	}
	headers := rows[0]
	seen := map[string]bool{}
	for _, header := range headers {
		key := normalizeMigrationName(header)
		if key == "" {
			return migrationSource{}, errors.New("Every source column needs a heading")
		}
		if seen[key] {
			return migrationSource{}, errors.New("Source column headings must be unique")
		}
		seen[key] = true
	}
	data := make([][]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if rowBlank(row) {
			continue
		}
		padded := make([]string, len(headers))
		copy(padded, row)
		data = append(data, padded)
	}
	samples := data
	if len(samples) > 5 {
		samples = samples[:5]
	}
	return migrationSource{Name: name, Columns: headers, Rows: data, Samples: samples, RowCount: len(data)}, nil
}

func rowBlank(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func migrationDefinition(key string) (migrationSectionDefinition, bool) {
	for _, definition := range migrationSections {
		if definition.Key == key {
			return definition, true
		}
	}
	return migrationSectionDefinition{}, false
}

func prepareMigration(db *gorm.DB, inspected migrationInspectResponse, plan migrationPlan) (migrationPreviewResponse, error) {
	if len(plan.Sections) == 0 || len(plan.Sections) > len(migrationSections) {
		return migrationPreviewResponse{}, errors.New("Choose at least one supported section")
	}
	if inspected.Format == "csv" && len(plan.Sections) != 1 {
		return migrationPreviewResponse{}, errors.New("A CSV file can import one section at a time")
	}
	bySource := map[string]migrationSource{}
	for _, source := range inspected.Sources {
		bySource[source.Name] = source
	}
	bySection := map[string]migrationSectionPlan{}
	for _, entry := range plan.Sections {
		if _, exists := bySection[entry.Section]; exists {
			return migrationPreviewResponse{}, errors.New("Each section can only be selected once")
		}
		if _, ok := migrationDefinition(entry.Section); !ok {
			return migrationPreviewResponse{}, errors.New("Unsupported migration section")
		}
		if entry.DuplicateStrategy != "skip" && entry.DuplicateStrategy != "update" && entry.DuplicateStrategy != "create" {
			return migrationPreviewResponse{}, errors.New("Duplicate strategy must be skip, update, or create")
		}
		if entry.Source == "" && len(inspected.Sources) == 1 {
			entry.Source = inspected.Sources[0].Name
		}
		if _, ok := bySource[entry.Source]; !ok {
			return migrationPreviewResponse{}, fmt.Errorf("source %q is not available", entry.Source)
		}
		if len(bySource[entry.Source].Rows) == 0 {
			return migrationPreviewResponse{}, fmt.Errorf("source %q has no data rows", entry.Source)
		}
		bySection[entry.Section] = entry
	}
	lookups, err := loadMigrationLookups(db)
	if err != nil {
		return migrationPreviewResponse{}, errors.New("Could not inspect existing data")
	}
	response := migrationPreviewResponse{CanImport: true, Sections: make([]migrationSectionPreview, 0, len(plan.Sections))}
	for _, definition := range migrationSections {
		entry, selected := bySection[definition.Key]
		if !selected {
			continue
		}
		source := bySource[entry.Source]
		mapping := effectiveMigrationMapping(definition, source.Columns, entry.Mapping)
		section := migrationSectionPreview{Section: definition.Key, Source: source.Name, EffectiveMapping: mapping, Rows: []migrationPreviewRow{}}
		for index, sourceRow := range source.Rows {
			values, provided := mappedMigrationValues(definition, source.Columns, sourceRow, mapping, entry.Defaults)
			row := validateMigrationRow(definition.Key, index+2, values, entry.DuplicateStrategy, lookups)
			row.provided = provided
			section.Rows = append(section.Rows, row)
			section.Summary.Total++
			switch row.Status {
			case "invalid":
				section.Summary.Invalid++
				response.CanImport = false
			case "warning":
				section.Summary.Warnings++
			case "duplicate":
				section.Summary.Duplicates++
				if row.Action == "skip" {
					section.Summary.Skipped++
				}
			default:
				section.Summary.Ready++
			}
		}
		response.Sections = append(response.Sections, section)
	}
	return response, nil
}

func effectiveMigrationMapping(definition migrationSectionDefinition, headers []string, requested map[string]string) map[string]string {
	result := map[string]string{}
	headerByName := map[string]string{}
	for _, header := range headers {
		headerByName[normalizeMigrationName(header)] = header
	}
	for _, field := range definition.Fields {
		if header := strings.TrimSpace(requested[field.Key]); header != "" {
			if actual, ok := headerByName[normalizeMigrationName(header)]; ok {
				result[field.Key] = actual
			}
			continue
		}
		for _, candidate := range append([]string{field.Key, field.Label}, field.Aliases...) {
			if actual, ok := headerByName[normalizeMigrationName(candidate)]; ok {
				result[field.Key] = actual
				break
			}
		}
	}
	return result
}

func mappedMigrationValues(definition migrationSectionDefinition, headers, row []string, mapping, defaults map[string]string) (map[string]string, map[string]bool) {
	columnIndex := map[string]int{}
	for i, header := range headers {
		columnIndex[header] = i
	}
	values := map[string]string{}
	provided := map[string]bool{}
	for _, field := range definition.Fields {
		if header := mapping[field.Key]; header != "" {
			provided[field.Key] = true
			if index, ok := columnIndex[header]; ok && index < len(row) {
				values[field.Key] = strings.TrimSpace(row[index])
			}
		}
		if values[field.Key] == "" {
			if fallback := strings.TrimSpace(defaults[field.Key]); fallback != "" {
				values[field.Key] = fallback
				provided[field.Key] = true
			}
		}
	}
	return values, provided
}

func validateMigrationRow(section string, rowNumber int, values map[string]string, strategy string, lookups *migrationLookups) migrationPreviewRow {
	row := migrationPreviewRow{Row: rowNumber, Status: "ready", Action: "create", Values: values, Issues: []migrationIssue{}}
	issue := func(field, code, message string) {
		row.Issues = append(row.Issues, migrationIssue{Field: field, Code: code, Message: message})
	}
	required := func(field string) string {
		value := strings.TrimSpace(values[field])
		if value == "" {
			issue(field, "required", "Required value is missing")
		}
		return value
	}
	positiveFloat := func(field string, allowZero bool) float64 {
		raw := required(field)
		if raw == "" {
			return 0
		}
		value, err := parseMigrationFloat(raw)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) || value > 1_000_000_000 || (!allowZero && value <= 0) || (allowZero && value < 0) {
			issue(field, "positive_number", "Enter a valid positive number")
			return 0
		}
		return value
	}
	integer := func(field string, minimum int, optional bool) int {
		raw := strings.TrimSpace(values[field])
		if optional && raw == "" {
			return 0
		}
		if !optional {
			raw = required(field)
			if raw == "" {
				return 0
			}
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < minimum || value > 1_000_000_000 {
			issue(field, "whole_number", fmt.Sprintf("Enter a whole number of at least %d", minimum))
			return 0
		}
		return value
	}
	var duplicate []uuid.UUID
	switch section {
	case "printers":
		name, manufacturer, model := required("name"), required("manufacturer"), required("model")
		if len([]rune(name)) > 100 || len([]rune(manufacturer)) > 100 || len([]rune(model)) > 100 {
			issue("name", "too_long", "Printer fields must not exceed 100 characters")
		}
		if len([]rune(values["notes"])) > 2000 {
			issue("notes", "too_long", "Notes must not exceed 2,000 characters")
		}
		printer := models.Printer{ID: uuid.New(), Name: name, Manufacturer: manufacturer, Model: model, Notes: values["notes"]}
		key := lookupKey(name)
		row.prepared, duplicate = printer, lookups.printers[key]
		if len(row.Issues) == 0 {
			selectedID := selectedMigrationID(printer.ID, duplicate, strategy)
			addLookup(lookups.printers, key, selectedID)
			if len(duplicate) == 0 || strategy == "create" || lookupContains(lookups.activePrinters, key, selectedID) {
				addLookup(lookups.activePrinters, key, selectedID)
			}
		}
	case "filaments":
		brand, filamentType, color := required("brand"), required("type"), required("color")
		price := positiveFloat("avg_price", false)
		score := 3
		if strings.TrimSpace(values["personal_score"]) != "" {
			score = integer("personal_score", 1, true)
			if score > 5 {
				issue("personal_score", "rating_range", "Rating must be between 1 and 5")
			}
		}
		used, purchased := integer("times_used", 0, true), integer("times_purchased", 0, true)
		filament := models.Filament{ID: uuid.New(), Brand: brand, Type: filamentType, Color: color, AvgPrice: price, PersonalScore: score, TimesUsed: used, TimesPurchased: purchased}
		key := filamentLookupKey(brand, filamentType, color)
		row.prepared, duplicate = filament, lookups.filaments[key]
		if len(row.Issues) == 0 {
			addLookup(lookups.filaments, key, selectedMigrationID(filament.ID, duplicate, strategy))
		}
	case "models":
		name := required("name")
		weight := positiveFloat("material_weight", false)
		printTime := integer("print_time", 1, false)
		waste := 0.0
		if strings.TrimSpace(values["waste_material"]) != "" {
			waste = positiveFloat("waste_material", true)
		}
		multicolor := false
		if strings.TrimSpace(values["is_multicolor"]) != "" {
			var err error
			multicolor, err = parseMigrationBool(values["is_multicolor"])
			if err != nil {
				issue("is_multicolor", "boolean", "Enter true or false")
			}
		}
		model := models.Model{ID: uuid.New(), Name: name, ImageURL: values["image_url"], MaterialWeight: weight, PrintTime: printTime, WasteMaterial: waste, IsMulticolor: multicolor}
		row.prepared, duplicate = model, lookups.models[lookupKey(name)]
		if len(row.Issues) == 0 {
			addLookup(lookups.models, lookupKey(name), selectedMigrationID(model.ID, duplicate, strategy))
		}
	case "purchases":
		filamentReference := required("filament")
		filamentIDs := lookups.filaments[lookupKey(filamentReference)]
		if len(filamentIDs) != 1 {
			issue("filament", "relationship", "Filament reference could not be resolved")
		}
		price, amount := positiveFloat("price", false), positiveFloat("amount", false)
		date, err := parseMigrationDate(required("purchase_date"))
		if err != nil {
			issue("purchase_date", "date", "Enter a valid date")
		}
		prepared := preparedPurchase{ID: uuid.New(), Price: price, Amount: amount, Date: date}
		if len(filamentIDs) == 1 {
			prepared.FilamentID = filamentIDs[0]
		}
		key := purchaseLookupKey(prepared.FilamentID, date)
		row.prepared, duplicate = prepared, lookups.purchases[key]
		if len(row.Issues) == 0 {
			addLookup(lookups.purchases, key, selectedMigrationID(prepared.ID, duplicate, strategy))
		}
	case "print_jobs":
		printerName := required("printer")
		printerIDs := lookups.activePrinters[lookupKey(printerName)]
		if len(printerIDs) != 1 {
			issue("printer", "relationship", "Printer reference could not be resolved")
		}
		var modelID *uuid.UUID
		if modelName := strings.TrimSpace(values["model"]); modelName != "" {
			ids := lookups.models[lookupKey(modelName)]
			if len(ids) != 1 {
				issue("model", "relationship", "Model reference could not be resolved")
			} else {
				id := ids[0]
				modelID = &id
			}
		}
		date, err := parseMigrationDate(required("print_date"))
		if err != nil {
			issue("print_date", "date", "Enter a valid date")
		}
		units := integer("units_printed", 1, false)
		labor := positiveFloat("labor_hours", true)
		var duration *int
		if strings.TrimSpace(values["print_time_minutes"]) != "" {
			value := integer("print_time_minutes", 1, true)
			duration = &value
		}
		var filamentID *uuid.UUID
		weight := 0.0
		filamentReference, weightRaw := strings.TrimSpace(values["filament"]), strings.TrimSpace(values["filament_weight_g"])
		if (filamentReference == "") != (weightRaw == "") {
			issue("filament", "paired", "Filament and filament weight must be provided together")
		} else if filamentReference != "" {
			ids := lookups.filaments[lookupKey(filamentReference)]
			if len(ids) != 1 {
				issue("filament", "relationship", "Filament reference could not be resolved")
			} else {
				id := ids[0]
				filamentID = &id
			}
			weight = positiveFloat("filament_weight_g", false)
		}
		job := models.PrintJob{ID: uuid.New(), ModelID: modelID, PrintDate: date, UnitsPrinted: units, LaborHours: float32(labor), PrintTimeMinutes: duration}
		if len(printerIDs) == 1 {
			job.PrinterID = &printerIDs[0]
		}
		prepared := preparedPrintJob{Job: job, FilamentID: filamentID, FilamentWeight: weight}
		key := printJobLookupKey(job.PrinterID, modelID, date)
		row.prepared, duplicate = prepared, lookups.jobs[key]
		if len(row.Issues) == 0 {
			addLookup(lookups.jobs, key, selectedMigrationID(job.ID, duplicate, strategy))
		}
	}
	if len(row.Issues) > 0 {
		row.Status, row.Action = "invalid", "none"
		return row
	}
	if len(duplicate) > 0 {
		row.existing = duplicate[0]
		switch strategy {
		case "skip":
			row.Status, row.Action = "duplicate", "skip"
		case "update":
			row.Status, row.Action = "warning", "update"
			issue("", "duplicate_update", "An existing record will be updated")
		case "create":
			row.Status, row.Action = "warning", "create"
			issue("", "duplicate_create", "A new record will be created alongside an existing match")
		}
	}
	return row
}

func selectedMigrationID(created uuid.UUID, duplicates []uuid.UUID, strategy string) uuid.UUID {
	if len(duplicates) > 0 && strategy != "create" {
		return duplicates[0]
	}
	return created
}

func loadMigrationLookups(db *gorm.DB) (*migrationLookups, error) {
	lookups := &migrationLookups{printers: map[string][]uuid.UUID{}, activePrinters: map[string][]uuid.UUID{}, filaments: map[string][]uuid.UUID{}, models: map[string][]uuid.UUID{}, purchases: map[string][]uuid.UUID{}, jobs: map[string][]uuid.UUID{}}
	var printers []models.Printer
	if err := db.Find(&printers).Error; err != nil {
		return nil, err
	}
	for _, item := range printers {
		addLookup(lookups.printers, lookupKey(item.Name), item.ID)
		if !item.IsArchived {
			addLookup(lookups.activePrinters, lookupKey(item.Name), item.ID)
		}
	}
	var filaments []models.Filament
	if err := db.Find(&filaments).Error; err != nil {
		return nil, err
	}
	for _, item := range filaments {
		addLookup(lookups.filaments, filamentLookupKey(item.Brand, item.Type, item.Color), item.ID)
	}
	var modelRows []models.Model
	if err := db.Find(&modelRows).Error; err != nil {
		return nil, err
	}
	for _, item := range modelRows {
		addLookup(lookups.models, lookupKey(item.Name), item.ID)
	}
	var purchases []models.FilamentPurchase
	if err := db.Find(&purchases).Error; err != nil {
		return nil, err
	}
	for _, item := range purchases {
		addLookup(lookups.purchases, purchaseLookupKey(item.FilamentID, item.PurchaseDate), item.ID)
	}
	var jobs []models.PrintJob
	if err := db.Find(&jobs).Error; err != nil {
		return nil, err
	}
	for _, item := range jobs {
		addLookup(lookups.jobs, printJobLookupKey(item.PrinterID, item.ModelID, item.PrintDate), item.ID)
	}
	return lookups, nil
}

func commitMigration(db *gorm.DB, preview migrationPreviewResponse) (map[string]int, error) {
	counts := map[string]int{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE printers SET is_default = is_default WHERE is_default = 1").Error; err != nil {
			return err
		}
		config, err := loadRawCostConfig(tx)
		if err != nil {
			return err
		}
		if err := models.ValidatePrinterCostConfig(config); err != nil {
			return err
		}
		var activePrinters int64
		if err := tx.Model(&models.Printer{}).Where("is_archived = ?", false).Count(&activePrinters).Error; err != nil {
			return err
		}
		for _, section := range preview.Sections {
			for _, row := range section.Rows {
				if row.Action == "skip" {
					counts["skipped"]++
					continue
				}
				switch value := row.prepared.(type) {
				case models.Printer:
					if row.Action == "update" {
						var current models.Printer
						if err := tx.First(&current, "id = ?", row.existing).Error; err != nil {
							return err
						}
						current.Name, current.Manufacturer, current.Model = value.Name, value.Manufacturer, value.Model
						if row.provided["notes"] {
							current.Notes = value.Notes
						}
						if err := tx.Save(&current).Error; err != nil {
							return err
						}
					} else {
						value.CostConfig = config
						value.IsDefault = activePrinters == 0
						if err := models.ValidatePrinter(value); err != nil {
							return err
						}
						if err := tx.Create(&value).Error; err != nil {
							return err
						}
						activePrinters++
					}
				case models.Filament:
					if row.Action == "update" {
						value.ID = row.existing
						updates := map[string]any{"brand": value.Brand, "type": value.Type, "color": value.Color, "avg_price": value.AvgPrice}
						for key, fieldValue := range map[string]any{"personal_score": value.PersonalScore, "times_used": value.TimesUsed, "times_purchased": value.TimesPurchased} {
							if row.provided[key] {
								updates[key] = fieldValue
							}
						}
						if err := tx.Model(&models.Filament{}).Where("id = ?", row.existing).Updates(updates).Error; err != nil {
							return err
						}
					} else if err := tx.Create(&value).Error; err != nil {
						return err
					}
				case models.Model:
					if row.Action == "update" {
						updates := map[string]any{"name": value.Name, "material_weight": value.MaterialWeight, "print_time": value.PrintTime}
						for key, fieldValue := range map[string]any{"image_url": value.ImageURL, "waste_material": value.WasteMaterial, "is_multicolor": value.IsMulticolor} {
							if row.provided[key] {
								updates[key] = fieldValue
							}
						}
						if err := tx.Model(&models.Model{}).Where("id = ?", row.existing).Updates(updates).Error; err != nil {
							return err
						}
					} else if err := tx.Create(&value).Error; err != nil {
						return err
					}
				case preparedPurchase:
					purchase := models.FilamentPurchase{ID: value.ID, FilamentID: value.FilamentID, Price: value.Price, Amount: value.Amount, PurchaseDate: value.Date}
					if row.Action == "update" {
						if err := tx.Model(&models.FilamentPurchase{}).Where("id = ?", row.existing).Updates(map[string]any{"filament_id": value.FilamentID, "price": value.Price, "amount": value.Amount, "purchase_date": value.Date}).Error; err != nil {
							return err
						}
					} else if err := tx.Create(&purchase).Error; err != nil {
						return err
					}
				case preparedPrintJob:
					job := value.Job
					requested := job.PrinterID
					replaceUsage := row.provided["filament"] || row.provided["filament_weight_g"]
					if row.Action == "update" {
						var current models.PrintJob
						if err := tx.First(&current, "id = ?", row.existing).Error; err != nil {
							return err
						}
						job.ID, job.PrinterID, job.PrinterCost, job.CreatedAt = current.ID, current.PrinterID, current.PrinterCost, current.CreatedAt
						if !row.provided["model"] {
							job.ModelID = current.ModelID
						}
						if !row.provided["print_time_minutes"] {
							job.PrintTimeMinutes = current.PrintTimeMinutes
						}
						if current.PrinterID == nil || requested == nil || *current.PrinterID != *requested {
							if err := assignPrintJobPrinter(tx, &job, requested, false); err != nil {
								return err
							}
						}
						if err := tx.Save(&job).Error; err != nil {
							return err
						}
						if replaceUsage {
							if err := tx.Where("print_job_id = ?", job.ID).Delete(&models.PrintJobFilament{}).Error; err != nil {
								return err
							}
						}
					} else {
						job.PrinterID = nil
						if err := assignPrintJobPrinter(tx, &job, requested, true); err != nil {
							return err
						}
						if err := tx.Create(&job).Error; err != nil {
							return err
						}
					}
					if value.FilamentID != nil && (row.Action != "update" || replaceUsage) {
						usage := models.PrintJobFilament{PrintJobID: job.ID, FilamentID: *value.FilamentID, WeightUsed: value.FilamentWeight}
						if err := tx.Create(&usage).Error; err != nil {
							return err
						}
					}
				}
				counts[section.Section]++
			}
		}
		return nil
	})
	return counts, err
}

func normalizeMigrationName(value string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '_'
	}, strings.TrimSpace(value)), "_")
}

func lookupKey(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}
func filamentLookupKey(brand, kind, color string) string {
	return lookupKey(brand + " | " + kind + " | " + color)
}
func purchaseLookupKey(filamentID uuid.UUID, date time.Time) string {
	return filamentID.String() + "|" + date.UTC().Format("2006-01-02")
}
func printJobLookupKey(printerID, modelID *uuid.UUID, date time.Time) string {
	printer, model := "", ""
	if printerID != nil {
		printer = printerID.String()
	}
	if modelID != nil {
		model = modelID.String()
	}
	return printer + "|" + model + "|" + date.UTC().Format(time.RFC3339)
}
func addLookup(values map[string][]uuid.UUID, key string, id uuid.UUID) {
	if key == "" || id == uuid.Nil {
		return
	}
	for _, existing := range values[key] {
		if existing == id {
			return
		}
	}
	values[key] = append(values[key], id)
}

func lookupContains(values map[string][]uuid.UUID, key string, id uuid.UUID) bool {
	for _, existing := range values[key] {
		if existing == id {
			return true
		}
	}
	return false
}

func parseMigrationFloat(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if strings.Contains(value, ",") && !strings.Contains(value, ".") {
		value = strings.ReplaceAll(value, ",", ".")
	}
	return strconv.ParseFloat(value, 64)
}

func parseMigrationBool(raw string) (bool, error) {
	switch lookupKey(raw) {
	case "true", "yes", "1", "sim":
		return true, nil
	case "false", "no", "0", "não", "nao":
		return false, nil
	default:
		return false, errors.New("invalid boolean")
	}
}

func parseMigrationDate(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04", "02/01/2006", "1/2/2006", "1/2/06", "02-01-2006"} {
		if value, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return value, nil
		}
	}
	return time.Time{}, errors.New("invalid date")
}
