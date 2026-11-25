package handlers_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func requestModel(t *testing.T, router *gin.Engine, method, path string, body any, status int) dto.ModelDTO {
	t.Helper()
	response := performRequest(t, router, method, path, body)
	if response.Code != status {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, status, response.Body)
	}
	var model dto.ModelDTO
	if status < 300 && status != http.StatusNoContent {
		if err := json.Unmarshal(response.Body.Bytes(), &model); err != nil {
			t.Fatal(err)
		}
	}
	return model
}

func TestModelMaterialRequirementsCRUDAndHistory(t *testing.T) {
	router, db, printer := setupTestRouterWithDefaultPrinter(t)
	filament := models.Filament{Brand: "Sample", Type: "PLA", Color: "Blue", AvgPrice: 25}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	row := gin.H{"material_type": " PLA ", "color": "Blue", "part_weight_g": 12.5, "waste_weight_g": 1, "preferred_filament_id": filament.ID, "source_tool": 2}
	body := gin.H{"name": "Bracket", "print_time": 20, "material_requirements": []gin.H{row}}
	single := requestModel(t, router, http.MethodPost, "/models", body, 201)
	rowID := single.MaterialRequirements[0].ID
	if single.MaterialSummary.ColorStatus != "single" || single.MaterialSummary.MaterialStatus != "single" || rowID == uuid.Nil || single.MaterialRequirements[0].MaterialType != "PLA" {
		t.Fatalf("unexpected single requirement: %+v", single)
	}
	jobResponse := performRequest(t, router, http.MethodPost, "/print-jobs", gin.H{
		"printer_id": printer.ID, "model_id": single.ID, "print_date": time.Now().UTC(),
		"units_printed": 1, "print_time_minutes": 20, "labor_hours": 0.25,
	})
	if jobResponse.Code != 201 {
		t.Fatalf("create historical print job: %s", jobResponse.Body)
	}
	var job dto.PrintJobDTO
	decodeBody(t, jobResponse, &job)
	used := models.PrintJobFilament{PrintJobID: job.ID, FilamentID: filament.ID, WeightUsed: 18}
	if err := db.Create(&used).Error; err != nil {
		t.Fatal(err)
	}
	costPath := "/print-jobs/" + job.ID.String() + "/cost"
	beforeCost := performRequest(t, router, http.MethodGet, costPath, nil)
	if beforeCost.Code != 200 || !strings.Contains(beforeCost.Body.String(), `"state":"calculated"`) {
		t.Fatalf("fixture has no calculated historical cost: %s", beforeCost.Body)
	}
	path := "/models/" + single.ID.String()
	row["id"] = rowID
	second := gin.H{"material_type": "pla", "color": "Orange", "part_weight_g": 3.5}
	body["material_requirements"] = []gin.H{row, second}
	twoColors := requestModel(t, router, http.MethodPut, path, body, 200)
	retried := requestModel(t, router, http.MethodPut, path, body, 200)
	if !reflect.DeepEqual(twoColors.MaterialRequirements, retried.MaterialRequirements) {
		t.Fatalf("identical retry changed material IDs: %+v", retried.MaterialRequirements)
	}
	if !twoColors.IsMulticolor || twoColors.MaterialSummary.ColorStatus != "multiple" || twoColors.MaterialSummary.MaterialStatus != "single" ||
		twoColors.MaterialWeight != 16 || twoColors.WasteMaterial != 1 || twoColors.MaterialSummary.WasteComplete ||
		twoColors.MaterialRequirements[0].ID != rowID {
		t.Fatalf("unexpected same-material colors: %+v", twoColors)
	}
	second["id"], second["material_type"], second["color"] = twoColors.MaterialRequirements[1].ID, "PETG", ""
	second["waste_weight_g"] = 0
	body["material_requirements"] = []gin.H{second, row}
	requestModel(t, router, http.MethodPut, path, body, 200)
	saved := requestModel(t, router, http.MethodGet, path, nil, 200)
	if saved.ID != single.ID || saved.MaterialSummary.ColorStatus != "unknown" || saved.MaterialSummary.MaterialStatus != "multiple" ||
		!saved.MaterialSummary.WasteComplete || saved.MaterialRequirements[0].ID != second["id"] || saved.MaterialRequirements[1].ID != rowID ||
		saved.PrintJobCount != 1 || saved.PrintJobs[0].ID != job.ID {
		t.Fatalf("unexpected mixed model: %+v", saved)
	}
	legacy := gin.H{"name": "Renamed", "print_time": 24, "material_weight": 999, "waste_material": 99, "is_multicolor": true}
	requestModel(t, router, http.MethodPut, path, legacy, 200)
	saved = requestModel(t, router, http.MethodGet, path, nil, 200)
	if saved.MaterialWeight != 16 || saved.WasteMaterial != 1 || saved.IsMulticolor || len(saved.MaterialRequirements) != 2 {
		t.Fatalf("legacy update changed structured estimates: %+v", saved)
	}
	var actual models.PrintJobFilament
	if err := db.First(&actual, "id = ?", used.ID).Error; err != nil || actual.WeightUsed != 18 || actual.PrintJobID != job.ID {
		t.Fatalf("historical grams changed: %+v %v", actual, err)
	}
	afterCost := performRequest(t, router, http.MethodGet, costPath, nil)
	if afterCost.Code != 200 || afterCost.Body.String() != beforeCost.Body.String() {
		t.Fatalf("historical cost changed: %s, want %s", afterCost.Body, beforeCost.Body)
	}
	report := performRequest(t, router, http.MethodGet, "/reports/export?mode=full", nil)
	if report.Code != 200 {
		t.Fatalf("CSV export: %s", report.Body)
	}
	export := reportRows(t, reportEntries(t, report.Body.Bytes())["model_material_requirements.csv"])
	if len(export) != 3 {
		t.Fatalf("unexpected exported material rows: %v", export)
	}
	var found bool
	for _, record := range export[1:] {
		found = found || record[0] == rowID.String() && record[1] == single.ID.String() && record[2] == "1" &&
			record[7] == filament.ID.String() && record[8] == "2"
	}
	if !found {
		t.Fatalf("missing stable identifiers or tool in export: %v", export)
	}
	legacy["material_requirements"] = []gin.H{}
	unset := requestModel(t, router, http.MethodPut, path, legacy, 200)
	if unset.MaterialWeight != 999 || !unset.IsMulticolor || unset.MaterialSummary.ColorStatus != "unknown" || len(unset.MaterialRequirements) != 0 {
		t.Fatalf("explicit return to aggregate estimates failed: %+v", unset)
	}
}

func TestModelMaterialRequirementsRejectInvalidRowsAndRollBack(t *testing.T) {
	router, db := setupTestRouter(t)
	valid := gin.H{"material_type": "PLA", "part_weight_g": 5}
	payload := gin.H{"name": "Fixture", "print_time": 5, "material_requirements": []gin.H{valid}}
	original := requestModel(t, router, http.MethodPost, "/models", payload, 201)
	path := "/models/" + original.ID.String()
	payload["name"] = "Should never save"
	id := original.MaterialRequirements[0].ID
	for _, rows := range [][]gin.H{
		{{"material_type": "PLA", "part_weight_g": -1}},
		{{"material_type": "PLA"}},
		{{"material_type": "PLA", "part_weight_g": 1e9, "waste_weight_g": 1}},
		{{"material_type": "PLA", "part_weight_g": 1, "waste_weight_g": -1}},
		{{"material_type": "PLA", "part_weight_g": 1, "preferred_filament_id": uuid.New()}},
		{{"material_type": "PLA", "part_weight_g": 1, "id": uuid.New()}},
		{{"material_type": "PLA", "part_weight_g": 1, "id": id}, {"material_type": "PLA", "part_weight_g": 1, "id": id}},
	} {
		payload["material_requirements"] = rows
		requestModel(t, router, http.MethodPut, path, payload, 400)
	}
	if err := db.Exec(`CREATE TRIGGER reject_new_requirement BEFORE INSERT ON model_material_requirements
		WHEN NEW.material_type = 'PETG' BEGIN SELECT RAISE(ABORT, 'fixture rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	payload["material_requirements"] = []gin.H{{"id": id, "material_type": "PLA", "part_weight_g": 8}, {"material_type": "PETG", "part_weight_g": 2}}
	requestModel(t, router, http.MethodPut, path, payload, 500)
	if saved := requestModel(t, router, http.MethodGet, path, nil, 200); !reflect.DeepEqual(saved, original) {
		t.Fatalf("failed update changed model: %+v", saved)
	}
}

func TestModelMaterialBackupRoundTripAndLegacyRestore(t *testing.T) {
	source, sourceDB := setupDesktopTestRouter(t)
	target, targetDB := setupDesktopTestRouter(t)
	sqlDB, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := targetDB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	filament := models.Filament{Brand: "Sample", Type: "PLA", Color: "Blue"}
	if err := sourceDB.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	legacy := requestModel(t, source, http.MethodPost, "/models", gin.H{"name": "Old", "print_time": 4, "material_weight": 5, "waste_material": 2, "is_multicolor": true}, 201)
	model := requestModel(t, source, http.MethodPost, "/models", gin.H{
		"name": "New", "print_time": 4, "material_requirements": []gin.H{
			{"material_type": "PLA", "part_weight_g": 5, "preferred_filament_id": filament.ID, "source_tool": 3},
			{"material_type": "PETG", "part_weight_g": 2, "waste_weight_g": 0},
		},
	}, 201)
	path := "/models/" + model.ID.String()
	restore := func(router *gin.Engine, version int, expected int) {
		t.Helper()
		export := performRequest(t, router, http.MethodGet, "/desktop-data/export", nil)
		if export.Code != 200 {
			t.Fatalf("backup export: %s", export.Body)
		}
		archive := export.Body.Bytes()
		if version != 20 {
			archive = rewriteBackupSchemaVersion(t, archive, version)
		}
		imported := performMultipartFileRequest(t, target, http.MethodPost, "/desktop-data/import", "file", "fixture.zip", archive)
		if imported.Code != expected {
			t.Fatalf("backup restore: got %d, want %d: %s", imported.Code, expected, imported.Body)
		}
	}
	restore(source, 20, 200)
	if saved := requestModel(t, target, http.MethodGet, path, nil, 200); !reflect.DeepEqual(saved, model) {
		t.Fatalf("round trip changed material rows: %+v", saved)
	}
	if err := sourceDB.Model(&models.ModelMaterialRequirement{}).Where("id = ?", model.MaterialRequirements[0].ID).
		Update("preferred_filament_id", uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	restore(source, 20, 400)
	if saved := requestModel(t, target, http.MethodGet, path, nil, 200); !reflect.DeepEqual(saved, model) {
		t.Fatalf("rejected backup changed target: %+v", saved)
	}
	if err := sourceDB.Migrator().DropTable(&models.ModelMaterialRequirement{}); err != nil {
		t.Fatal(err)
	}
	restore(source, 20, 400)
	restore(source, 19, 200)
	got := requestModel(t, target, http.MethodGet, "/models/"+legacy.ID.String(), nil, 200)
	if !reflect.DeepEqual(got, legacy) || !got.IsMulticolor || got.MaterialSummary.ColorStatus != "unknown" {
		t.Fatalf("restore inferred a legacy material split: %+v", got)
	}
	if saved := requestModel(t, target, http.MethodGet, path, nil, 200); len(saved.MaterialRequirements) != 0 {
		t.Fatalf("legacy restore left target requirements behind: %+v", saved)
	}
}

func TestSpreadsheetUpdateKeepsStructuredModelRequirements(t *testing.T) {
	router, _ := setupTestRouter(t)
	record := requestModel(t, router, http.MethodPost, "/models", gin.H{
		"name": "Housing", "print_time": 25,
		"material_requirements": []gin.H{{"material_type": "PLA", "part_weight_g": 14}},
	}, 201)
	plan := migrationPlan(map[string]any{"section": "models", "source": "models", "duplicate_strategy": "update"})
	file := []byte("name,material_weight,print_time,waste_material,is_multicolor\nHousing,100,32,20,true\n")
	preview := performMigrationRequest(t, router, "/data-migration/preview", "models.csv", file, plan)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), "model_requirements_preserved") {
		t.Fatalf("spreadsheet preview: %d %s", preview.Code, preview.Body.String())
	}
	commit := performMigrationRequest(t, router, "/data-migration/import", "models.csv", file, plan)
	if commit.Code != http.StatusCreated {
		t.Fatalf("spreadsheet import: %d %s", commit.Code, commit.Body.String())
	}
	saved := requestModel(t, router, http.MethodGet, "/models/"+record.ID.String(), nil, 200)
	if saved.PrintTime != 32 || saved.MaterialWeight != 14 || saved.WasteMaterial != 0 || saved.IsMulticolor ||
		!reflect.DeepEqual(saved.MaterialRequirements, record.MaterialRequirements) {
		t.Fatalf("spreadsheet updated the requirement projection: %+v", saved)
	}
}

func TestModelMaterialPreferencesDetachAndRequirementsDelete(t *testing.T) {
	router, db := setupTestRouter(t)
	filament := models.Filament{Brand: "Example", Type: "PLA", Color: "Blue"}
	if err := db.Create(&filament).Error; err != nil {
		t.Fatal(err)
	}
	record := requestModel(t, router, http.MethodPost, "/models", gin.H{
		"name": "Housing", "print_time": 25,
		"material_requirements": []gin.H{{"material_type": "PLA", "part_weight_g": 14, "preferred_filament_id": filament.ID}},
	}, 201)
	requestModel(t, router, http.MethodDelete, "/filaments/"+filament.ID.String(), nil, 204)
	path := "/models/" + record.ID.String()
	saved := requestModel(t, router, http.MethodGet, path, nil, 200)
	if saved.MaterialRequirements[0].ID != record.MaterialRequirements[0].ID || saved.MaterialRequirements[0].PreferredFilamentID != nil {
		t.Fatalf("filament delete changed the requirement: %+v", saved)
	}
	requestModel(t, router, http.MethodDelete, path, nil, 204)
	var count int64
	if err := db.Model(&models.ModelMaterialRequirement{}).Where("model_id = ?", record.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("model delete left %d requirement rows: %v", count, err)
	}
}
