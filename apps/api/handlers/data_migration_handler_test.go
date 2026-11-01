package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func performMigrationRequest(t *testing.T, router *gin.Engine, path, fileName string, file []byte, plan any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatal(err)
	}
	if plan != nil {
		payload, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteField("plan", string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func migrationPlan(sections ...map[string]any) map[string]any {
	return map[string]any{"sections": sections}
}

func xlsxFixture(t *testing.T, sheets map[string][][]string) []byte {
	t.Helper()
	book := excelize.NewFile()
	first := true
	for name, rows := range sheets {
		if first {
			book.SetSheetName("Sheet1", name)
			first = false
		} else if _, err := book.NewSheet(name); err != nil {
			t.Fatal(err)
		}
		for rowIndex, row := range rows {
			for columnIndex, value := range row {
				cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
				if err := book.SetCellStr(name, cell, value); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	var output bytes.Buffer
	if err := book.Write(&output); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestDataMigrationSchemaAndTemplates(t *testing.T) {
	router, _ := setupTestRouter(t)
	schema := performRequest(t, router, http.MethodGet, "/data-migration/schema", nil)
	if schema.Code != http.StatusOK || !bytes.Contains(schema.Body.Bytes(), []byte(`"print_jobs"`)) {
		t.Fatalf("schema: %d %s", schema.Code, schema.Body.String())
	}
	template := performRequest(t, router, http.MethodGet, "/data-migration/templates/models", nil)
	if template.Code != http.StatusOK || template.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !bytes.Contains(template.Body.Bytes(), []byte("name,material_weight,print_time")) {
		t.Fatalf("template: %d %s", template.Code, template.Body.String())
	}
}

func TestDataMigrationWorkbookTemplateCanBePreviewed(t *testing.T) {
	router, _ := setupTestRouter(t)
	template := performRequest(t, router, http.MethodGet, "/data-migration/workbook-template", nil)
	if template.Code != http.StatusOK || template.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || template.Header().Get("Content-Disposition") != `attachment; filename="makertrack-migration-template.xlsx"` {
		t.Fatalf("workbook template: %d %s", template.Code, template.Body.String())
	}
	book, err := excelize.OpenReader(bytes.NewReader(template.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, section := range []struct {
		name   string
		header string
	}{
		{"printers", "name"}, {"filaments", "brand"}, {"purchases", "filament"}, {"models", "name"}, {"print_jobs", "printer"},
	} {
		rows, err := book.GetRows(section.name)
		if err != nil || len(rows) != 2 || len(rows[0]) == 0 || rows[0][0] != section.header {
			t.Fatalf("section %s has invalid headers or example row: %v %v", section.name, rows, err)
		}
	}
	plan := migrationPlan(
		map[string]any{"section": "printers", "source": "printers", "duplicate_strategy": "skip"},
		map[string]any{"section": "filaments", "source": "filaments", "duplicate_strategy": "skip"},
		map[string]any{"section": "purchases", "source": "purchases", "duplicate_strategy": "skip"},
		map[string]any{"section": "models", "source": "models", "duplicate_strategy": "skip"},
		map[string]any{"section": "print_jobs", "source": "print_jobs", "duplicate_strategy": "skip"},
	)
	preview := performMigrationRequest(t, router, "/data-migration/preview", "example.xlsx", template.Body.Bytes(), plan)
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte(`"can_import":true`)) {
		t.Fatalf("workbook template preview: %d %s", preview.Code, preview.Body.String())
	}
}

func TestDataMigrationManualMappingDefaultsAndValidation(t *testing.T) {
	router, _ := setupTestRouter(t)
	file := []byte("Machine,Maker,Series\nWorkshop,Bambu Lab,A1\n")
	plan := migrationPlan(map[string]any{
		"section": "printers", "source": "printers", "duplicate_strategy": "skip",
		"mapping":  map[string]string{"name": "Machine", "manufacturer": "Maker", "model": "Series"},
		"defaults": map[string]string{"notes": "Imported spreadsheet"},
	})
	response := performMigrationRequest(t, router, "/data-migration/preview", "printers.csv", file, plan)
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
	var preview struct {
		CanImport bool `json:"can_import"`
		Sections  []struct {
			EffectiveMapping map[string]string `json:"effective_mapping"`
			Rows             []struct {
				Status string            `json:"status"`
				Values map[string]string `json:"values"`
			} `json:"rows"`
		} `json:"sections"`
	}
	decodeBody(t, response, &preview)
	if !preview.CanImport || preview.Sections[0].Rows[0].Status != "ready" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if preview.Sections[0].Rows[0].Values["notes"] != "Imported spreadsheet" {
		t.Fatalf("default was not applied: %#v", preview)
	}

	invalid := performMigrationRequest(t, router, "/data-migration/preview", "printers.csv", []byte("name,manufacturer,model\nWorkshop,,A1\n"), migrationPlan(map[string]any{"section": "printers", "source": "printers", "duplicate_strategy": "skip"}))
	if invalid.Code != http.StatusOK || !bytes.Contains(invalid.Body.Bytes(), []byte(`"status":"invalid"`)) || !bytes.Contains(invalid.Body.Bytes(), []byte(`"code":"required"`)) {
		t.Fatalf("expected required-field preview error: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestDataMigrationImportsRepresentativeXLSXWithRelationships(t *testing.T) {
	router, db := setupTestRouter(t)
	file := xlsxFixture(t, map[string][][]string{
		"Printers":   {{"name", "manufacturer", "model", "notes"}, {"Workshop A1", "Bambu Lab", "A1", ""}},
		"Filaments":  {{"brand", "type", "color", "avg_price", "personal_score"}, {"Bambu Lab", "PLA", "Black", "24.99", "4"}},
		"Purchases":  {{"filament", "price", "amount", "purchase_date"}, {"Bambu Lab | PLA | Black", "24.99", "1", "2026-10-02"}},
		"Models":     {{"name", "material_weight", "print_time", "waste_material", "is_multicolor"}, {"Desk organiser", "125", "180", "5", "false"}},
		"Print Jobs": {{"printer", "model", "print_date", "units_printed", "labor_hours", "filament", "filament_weight_g"}, {"Workshop A1", "Desk organiser", "2026-10-02", "2", "0.5", "Bambu Lab | PLA | Black", "250"}},
	})
	plan := migrationPlan(
		map[string]any{"section": "printers", "source": "Printers", "duplicate_strategy": "skip"},
		map[string]any{"section": "filaments", "source": "Filaments", "duplicate_strategy": "skip"},
		map[string]any{"section": "purchases", "source": "Purchases", "duplicate_strategy": "skip"},
		map[string]any{"section": "models", "source": "Models", "duplicate_strategy": "skip"},
		map[string]any{"section": "print_jobs", "source": "Print Jobs", "duplicate_strategy": "skip"},
	)
	inspect := performMigrationRequest(t, router, "/data-migration/inspect", "history.xlsx", file, nil)
	if inspect.Code != http.StatusOK || !bytes.Contains(inspect.Body.Bytes(), []byte(`"format":"xlsx"`)) {
		t.Fatalf("inspect: %d %s", inspect.Code, inspect.Body.String())
	}
	preview := performMigrationRequest(t, router, "/data-migration/preview", "history.xlsx", file, plan)
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte(`"can_import":true`)) {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	result := performMigrationRequest(t, router, "/data-migration/import", "history.xlsx", file, plan)
	if result.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", result.Code, result.Body.String())
	}

	for label, model := range map[string]any{"printers": &models.Printer{}, "filaments": &models.Filament{}, "purchases": &models.FilamentPurchase{}, "models": &models.Model{}, "jobs": &models.PrintJob{}, "usage": &models.PrintJobFilament{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", label, count, err)
		}
	}
	var job models.PrintJob
	if err := db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.PrinterID == nil || job.ModelID == nil || job.PrinterCost == nil {
		t.Fatalf("relationships or cost snapshot missing: %#v", job)
	}
}

func TestDataMigrationDuplicateStrategiesAreExplicit(t *testing.T) {
	router, db := setupTestRouter(t)
	existing := models.Filament{Brand: "Prusa", Type: "PLA", Color: "Orange", AvgPrice: 20, PersonalScore: 3, TimesUsed: 7, TimesPurchased: 4}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	file := []byte("brand,type,color,avg_price,personal_score\nPrusa,PLA,Orange,30,5\n")
	entry := func(strategy string) map[string]any {
		return map[string]any{"section": "filaments", "source": "filaments", "duplicate_strategy": strategy}
	}

	skipPreview := performMigrationRequest(t, router, "/data-migration/preview", "filaments.csv", file, migrationPlan(entry("skip")))
	if !bytes.Contains(skipPreview.Body.Bytes(), []byte(`"status":"duplicate"`)) || !bytes.Contains(skipPreview.Body.Bytes(), []byte(`"action":"skip"`)) {
		t.Fatalf("skip was not explicit: %s", skipPreview.Body.String())
	}
	if result := performMigrationRequest(t, router, "/data-migration/import", "filaments.csv", file, migrationPlan(entry("skip"))); result.Code != http.StatusCreated {
		t.Fatalf("skip import: %s", result.Body.String())
	}
	var unchanged models.Filament
	if err := db.First(&unchanged, "id = ?", existing.ID).Error; err != nil || unchanged.AvgPrice != 20 {
		t.Fatalf("skip overwrote data: %#v %v", unchanged, err)
	}

	updatePreview := performMigrationRequest(t, router, "/data-migration/preview", "filaments.csv", file, migrationPlan(entry("update")))
	if !bytes.Contains(updatePreview.Body.Bytes(), []byte(`"action":"update"`)) || !bytes.Contains(updatePreview.Body.Bytes(), []byte(`"code":"duplicate_update"`)) {
		t.Fatalf("update was not explicit: %s", updatePreview.Body.String())
	}
	if result := performMigrationRequest(t, router, "/data-migration/import", "filaments.csv", file, migrationPlan(entry("update"))); result.Code != http.StatusCreated {
		t.Fatalf("update import: %s", result.Body.String())
	}
	if err := db.First(&unchanged, "id = ?", existing.ID).Error; err != nil || unchanged.AvgPrice != 30 || unchanged.PersonalScore != 5 || unchanged.TimesUsed != 7 || unchanged.TimesPurchased != 4 {
		t.Fatalf("explicit update failed: %#v %v", unchanged, err)
	}

	if result := performMigrationRequest(t, router, "/data-migration/import", "filaments.csv", file, migrationPlan(entry("create"))); result.Code != http.StatusCreated {
		t.Fatalf("create import: %s", result.Body.String())
	}
	var count int64
	if err := db.Model(&models.Filament{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("explicit create count=%d err=%v", count, err)
	}
}

func TestDataMigrationReportsRelationshipsBeforePersistence(t *testing.T) {
	router, db := setupTestRouter(t)
	file := xlsxFixture(t, map[string][][]string{
		"Printers":  {{"name", "manufacturer", "model"}, {"Workshop A1", "Bambu Lab", "A1"}},
		"Purchases": {{"filament", "price", "amount", "purchase_date"}, {"Missing | PLA | Red", "20", "1", "2026-10-02"}},
	})
	plan := migrationPlan(
		map[string]any{"section": "printers", "source": "Printers", "duplicate_strategy": "skip"},
		map[string]any{"section": "purchases", "source": "Purchases", "duplicate_strategy": "skip"},
	)
	preview := performMigrationRequest(t, router, "/data-migration/preview", "invalid.xlsx", file, plan)
	if !bytes.Contains(preview.Body.Bytes(), []byte(`"code":"relationship"`)) || !bytes.Contains(preview.Body.Bytes(), []byte(`"can_import":false`)) {
		t.Fatalf("missing relationship preview: %s", preview.Body.String())
	}
	result := performMigrationRequest(t, router, "/data-migration/import", "invalid.xlsx", file, plan)
	if result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422: %d %s", result.Code, result.Body.String())
	}
	var count int64
	if err := db.Model(&models.Printer{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("preflight partially persisted printers: %d %v", count, err)
	}
}

func TestDataMigrationRollsBackOnWriteFailure(t *testing.T) {
	router, db := setupTestRouter(t)
	if err := db.Exec(`CREATE TRIGGER reject_migrated_model BEFORE INSERT ON models BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	file := xlsxFixture(t, map[string][][]string{
		"Printers": {{"name", "manufacturer", "model"}, {"Workshop A1", "Bambu Lab", "A1"}},
		"Models":   {{"name", "material_weight", "print_time"}, {"Desk organiser", "125", "180"}},
	})
	plan := migrationPlan(
		map[string]any{"section": "printers", "source": "Printers", "duplicate_strategy": "skip"},
		map[string]any{"section": "models", "source": "Models", "duplicate_strategy": "skip"},
	)
	result := performMigrationRequest(t, router, "/data-migration/import", "rollback.xlsx", file, plan)
	if result.Code != http.StatusInternalServerError {
		t.Fatalf("expected write failure: %d %s", result.Code, result.Body.String())
	}
	var printers int64
	if err := db.Model(&models.Printer{}).Count(&printers).Error; err != nil || printers != 0 {
		t.Fatalf("transaction did not roll back printer: %d %v", printers, err)
	}
}
