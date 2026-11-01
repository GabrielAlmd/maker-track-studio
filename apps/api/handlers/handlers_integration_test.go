package handlers_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/routes"
)

type apiResponse map[string]any

func setupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := "file:" + uuid.New().String() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	for _, stmt := range sqliteSchemaStatements() {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to initialize sqlite schema: %v", err)
		}
	}

	if err := db.AutoMigrate(
		&models.Printer{},
		&models.PrintJob{},
		&salesmodels.SalesChannel{},
		&salesmodels.SalesOrder{},
		&salesmodels.SalesOrderItem{},
		&salesmodels.SalesOrderAllocation{},
	); err != nil {
		t.Fatalf("failed to initialize desktop models: %v", err)
	}

	r := gin.New()
	routes.RegisterRoutes(r, db)
	return r, db
}

func setupTestRouterWithDefaultPrinter(t *testing.T) (*gin.Engine, *gorm.DB, models.Printer) {
	t.Helper()
	r, db := setupTestRouter(t)
	printer := addTestPrinter(t, r, "Default test printer", "A1")
	return r, db, printer
}

type modernCostFixture struct {
	Printer    models.Printer
	ModelID    string
	FilamentID string
	JobID      string
}

func createModernCostFixture(t *testing.T, r *gin.Engine) modernCostFixture {
	t.Helper()

	filamentRes := performRequest(t, r, http.MethodPost, "/filaments", map[string]any{
		"brand": "FlowBrand", "type": "PLA", "color": "Green", "avg_price": 25.0, "personal_score": 4,
	})
	if filamentRes.Code != http.StatusCreated {
		t.Fatalf("create filament: %d %s", filamentRes.Code, filamentRes.Body.String())
	}
	var filament apiResponse
	decodeBody(t, filamentRes, &filament)

	modelRes := performRequest(t, r, http.MethodPost, "/models", map[string]any{
		"name": "FlowModel", "material_weight": 50.0, "print_time": 60,
		"waste_material": 2.0, "is_multicolor": false,
	})
	if modelRes.Code != http.StatusCreated {
		t.Fatalf("create model: %d %s", modelRes.Code, modelRes.Body.String())
	}
	var model apiResponse
	decodeBody(t, modelRes, &model)

	printer := addTestPrinter(t, r, "Cost flow printer", "A1")
	printer.CostConfig["filament_waste_factor"] = 1.10
	printer = configurePrinter(t, r, printer, 0.20, 0.08, 15.0)

	jobRes := performRequest(t, r, http.MethodPost, "/print-jobs", map[string]any{
		"printer_id": printer.ID.String(), "model_id": model["id"],
		"print_date": "2026-09-19T12:00:00Z", "units_printed": 1,
		"labor_hours": 1.0, "print_time_minutes": 60,
	})
	if jobRes.Code != http.StatusCreated {
		t.Fatalf("create print job: %d %s", jobRes.Code, jobRes.Body.String())
	}
	var job apiResponse
	decodeBody(t, jobRes, &job)
	if job["printer_cost"] == nil {
		t.Fatal("modern print job is missing its printer cost snapshot")
	}

	usageRes := performRequest(t, r, http.MethodPost, "/print-job-filaments", map[string]any{
		"print_job_id": job["id"], "filament_id": filament["id"], "weight_used": 100.0,
	})
	if usageRes.Code != http.StatusCreated {
		t.Fatalf("create print job filament: %d %s", usageRes.Code, usageRes.Body.String())
	}

	modelID, ok := model["id"].(string)
	if !ok {
		t.Fatalf("model response has invalid ID: %#v", model["id"])
	}
	filamentID, ok := filament["id"].(string)
	if !ok {
		t.Fatalf("filament response has invalid ID: %#v", filament["id"])
	}
	jobID, ok := job["id"].(string)
	if !ok {
		t.Fatalf("print job response has invalid ID: %#v", job["id"])
	}
	return modernCostFixture{Printer: printer, ModelID: modelID, FilamentID: filamentID, JobID: jobID}
}

func setupDesktopTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	return setupTestRouter(t)
}

func sqliteSchemaStatements() []string {
	return []string{
		`CREATE TABLE filaments (
			id TEXT PRIMARY KEY,
			brand TEXT NOT NULL,
			type TEXT NOT NULL,
			color TEXT NOT NULL,
			avg_price REAL NOT NULL,
			personal_score INTEGER,
			times_used INTEGER DEFAULT 0,
			times_purchased INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE filament_purchases (
			id TEXT PRIMARY KEY,
			filament_id TEXT NOT NULL,
			price REAL NOT NULL,
			amount REAL NOT NULL,
			purchase_date DATETIME NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE models (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			image_url TEXT,
			material_weight REAL NOT NULL,
			print_time INTEGER NOT NULL,
			waste_material REAL NOT NULL,
			is_multicolor BOOLEAN NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE print_jobs (
			id TEXT PRIMARY KEY,
			model_id TEXT,
			print_date DATETIME NOT NULL,
			units_printed INTEGER NOT NULL,
			labor_hours REAL NOT NULL,
			print_time_minutes INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE print_job_filaments (
			id TEXT PRIMARY KEY,
			print_job_id TEXT NOT NULL,
			filament_id TEXT NOT NULL,
			weight_used REAL NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE app_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			type TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);`,
	}
}

func performRequest(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal payload: %v", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, path, reqBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func performMultipartFileRequest(t *testing.T, r *gin.Engine, method, path, fieldName, fileName string, fileContent []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("failed to create multipart form file: %v", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		t.Fatalf("failed to write multipart file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req, err := http.NewRequest(method, path, &body)
	if err != nil {
		t.Fatalf("failed to create multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func buildZipArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close zip archive: %v", err)
	}

	return buf.Bytes()
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("failed to decode response body: %v | body=%s", err, w.Body.String())
	}
}

func TestFilamentCRUD(t *testing.T) {
	r, db := setupTestRouter(t)
	id := uuid.New().String()
	now := time.Now()

	if err := db.Exec(
		`INSERT INTO filaments (id, brand, type, color, avg_price, personal_score, times_used, times_purchased, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "Prusa", "PLA", "Orange", 28.50, 5, 0, 0, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed filament: %v", err)
	}

	getRes := performRequest(t, r, http.MethodGet, "/filaments/"+id, nil)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", getRes.Code, getRes.Body.String())
	}

	updatePayload := map[string]any{
		"brand":          "Prusa",
		"type":           "PLA",
		"color":          "Red",
		"avg_price":      30.00,
		"personal_score": 4,
	}
	updateRes := performRequest(t, r, http.MethodPut, "/filaments/"+id, updatePayload)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", updateRes.Code, updateRes.Body.String())
	}

	deleteRes := performRequest(t, r, http.MethodDelete, "/filaments/"+id, nil)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", deleteRes.Code, deleteRes.Body.String())
	}
}

func TestCreateFilamentEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)

	createPayload := map[string]any{
		"brand":          "Bambu",
		"type":           "PETG",
		"color":          "White",
		"avg_price":      25.75,
		"personal_score": 4,
	}

	res := performRequest(t, r, http.MethodPost, "/filaments", createPayload)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if body["brand"] != "Bambu" {
		t.Fatalf("expected brand=Bambu, got %#v", body["brand"])
	}
}

func TestDesktopModeDisablesCredentialAuthRoutes(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodPost, "/auth/login", map[string]any{
		"user_identifier": "demo",
		"password":        "demo",
	})
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when auth routes are disabled in desktop mode, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestDesktopModeBusinessRoutesDoNotRequireJWT(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodGet, "/filaments", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 without JWT token in desktop mode test setup, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestLegacyPrintJobCostUsesPersistedWeightWithoutDoubleCountingModelWaste(t *testing.T) {
	r, db := setupTestRouter(t)
	now := time.Now()

	modelID := uuid.New().String()
	filamentID := uuid.New().String()
	printJobID := uuid.New().String()
	printJobFilamentID := uuid.New().String()

	if err := db.Exec(
		`INSERT INTO models (id, name, material_weight, print_time, waste_material, is_multicolor, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		modelID, "Gear", 100.0, 120, 5.0, false, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed model: %v", err)
	}

	if err := db.Exec(
		`INSERT INTO filaments (id, brand, type, color, avg_price, personal_score, times_used, times_purchased, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		filamentID, "Generic", "PLA", "Black", 30.0, 4, 0, 0, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed filament: %v", err)
	}

	if err := db.Exec(
		`INSERT INTO print_jobs (id, model_id, print_date, units_printed, labor_hours, print_time_minutes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		printJobID, modelID, now, 2, 1.5, 120, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed print job: %v", err)
	}

	if err := db.Exec(
		`INSERT INTO print_job_filaments (id, print_job_id, filament_id, weight_used, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		// The usage row already contains the job's complete consumed weight. The
		// model's waste must not be added again by historical cost calculations.
		printJobFilamentID, printJobID, filamentID, 150.0, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed print job filament: %v", err)
	}

	if err := db.Exec(
		`INSERT INTO app_config (key, value, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)`,
		"electricity_cost_kwh", "0.20", "number", now, now,
		"printer_wear_cost_hour", "0.08", "number", now, now,
		"labor_cost_hour", "15", "number", now, now,
		"filament_waste_factor", "1.10", "number", now, now,
		"margin_tier_1", "50", "number", now, now,
		"margin_tier_2", "60", "number", now, now,
		"margin_tier_3", "70", "number", now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed app config: %v", err)
	}

	res := performRequest(t, r, http.MethodGet, "/print-jobs/"+printJobID+"/cost", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)

	assertFloat(t, body["filament_cost"], 4.95)
	assertFloat(t, body["electricity_cost"], 0.40)
	assertFloat(t, body["wear_cost"], 0.16)
	assertFloat(t, body["labor_cost"], 22.50)
	assertFloat(t, body["total_cost"], 28.01)
	for _, removed := range []string{"margin_tier_1", "margin_tier_2", "margin_tier_3", "suggested_price_t1", "suggested_price_t2", "suggested_price_t3"} {
		if _, exists := body[removed]; exists {
			t.Fatalf("removed Print Job pricing field %s is still exposed: %#v", removed, body)
		}
	}
}

func TestPrintJobCostInvalidUUID(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodGet, "/print-jobs/not-a-uuid/cost", nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestCreatePrintJobAllowsNilModelID(t *testing.T) {
	r, _, defaultPrinter := setupTestRouterWithDefaultPrinter(t)

	payload := map[string]any{
		"print_date":    time.Now().Format(time.RFC3339),
		"units_printed": 1,
		"labor_hours":   0.5,
	}

	res := performRequest(t, r, http.MethodPost, "/print-jobs", payload)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if modelID, exists := body["model_id"]; exists && modelID != nil {
		t.Fatalf("expected nil model_id, got %#v", modelID)
	}
	if body["printer_id"] != defaultPrinter.ID.String() {
		t.Fatalf("expected default printer %s, got %#v", defaultPrinter.ID, body["printer_id"])
	}
}

func TestPrintJobLaborHoursValidation(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, tc := range []struct {
			name  string
			value any
			omit  bool
			valid bool
		}{
			{name: "zero", value: 0, valid: true},
			{name: "positive", value: 1.5, valid: true},
			{name: "negative", value: -0.5},
			{name: "missing", omit: true},
			{name: "null", value: nil},
			{name: "empty", value: ""},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				r, db, _ := setupTestRouterWithDefaultPrinter(t)
				payload := map[string]any{
					"print_date":    "2026-04-03T00:00:00Z",
					"units_printed": 1,
					"labor_hours":   2,
				}
				path := "/print-jobs"
				var id string
				if method == http.MethodPut {
					seed := performRequest(t, r, http.MethodPost, path, payload)
					if seed.Code != http.StatusCreated {
						t.Fatalf("seed failed: %d %s", seed.Code, seed.Body.String())
					}
					var body apiResponse
					decodeBody(t, seed, &body)
					id = body["id"].(string)
					path += "/" + id
				}
				delete(payload, "labor_hours")
				if !tc.omit {
					payload["labor_hours"] = tc.value
				}
				res := performRequest(t, r, method, path, payload)
				wantStatus := http.StatusBadRequest
				if tc.valid {
					wantStatus = http.StatusCreated
					if method == http.MethodPut {
						wantStatus = http.StatusOK
					}
				}
				if res.Code != wantStatus {
					t.Fatalf("expected %d, got %d: %s", wantStatus, res.Code, res.Body.String())
				}
				if tc.valid {
					var body apiResponse
					decodeBody(t, res, &body)
					wantHours := 0.0
					if tc.name == "positive" {
						wantHours = 1.5
					}
					assertFloat(t, body["labor_hours"], wantHours)
					var saved struct{ LaborHours float64 }
					if err := db.Table("print_jobs").Where("id = ?", body["id"]).First(&saved).Error; err != nil {
						t.Fatal(err)
					}
					assertFloat(t, saved.LaborHours, wantHours)
				} else if method == http.MethodPut {
					var saved struct{ LaborHours float64 }
					if err := db.Table("print_jobs").Where("id = ?", id).First(&saved).Error; err != nil {
						t.Fatal(err)
					}
					assertFloat(t, saved.LaborHours, 2)
				} else {
					var count int64
					if err := db.Table("print_jobs").Count(&count).Error; err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatalf("invalid input created %d jobs", count)
					}
				}
			})
		}
	}
}

func TestAppConfigCRUD(t *testing.T) {
	r, _ := setupTestRouter(t)

	createRes := performRequest(t, r, http.MethodPost, "/app-config/labor_cost_hour", map[string]any{
		"value": "1.25",
		"type":  "number",
	})
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", createRes.Code, createRes.Body.String())
	}

	getRes := performRequest(t, r, http.MethodGet, "/app-config/labor_cost_hour", nil)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", getRes.Code, getRes.Body.String())
	}

	updateRes := performRequest(t, r, http.MethodPut, "/app-config/labor_cost_hour", map[string]any{
		"value": "2.5",
		"type":  "number",
	})
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", updateRes.Code, updateRes.Body.String())
	}

	deleteRes := performRequest(t, r, http.MethodDelete, "/app-config/labor_cost_hour", nil)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body=%s", deleteRes.Code, deleteRes.Body.String())
	}
}

func TestFilamentPurchaseRequiresFilamentID(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodPost, "/filament-purchases", map[string]any{
		"price":         25.0,
		"amount":        1.0,
		"purchase_date": time.Now().Format(time.RFC3339),
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestDesktopDataExportImportRoundTrip(t *testing.T) {
	exportRouter, exportDB := setupDesktopTestRouter(t)
	now := time.Now().UTC()
	filamentID := uuid.New().String()

	if err := exportDB.Exec(
		`INSERT INTO filaments (id, brand, type, color, avg_price, personal_score, times_used, times_purchased, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		filamentID, "RoundTrip", "PLA", "Black", 27.30, 5, 0, 0, now, now,
	).Error; err != nil {
		t.Fatalf("failed to seed filament for desktop export: %v", err)
	}

	if err := exportDB.Create(&models.AppConfig{
		Key: "obsolete_private_setting", Value: "must-not-survive", Type: "secret", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("failed to seed obsolete private config for desktop export: %v", err)
	}

	exportRes := performRequest(t, exportRouter, http.MethodGet, "/desktop-data/export", nil)
	if exportRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", exportRes.Code, exportRes.Body.String())
	}
	if !strings.Contains(exportRes.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("expected zip content-type, got %q", exportRes.Header().Get("Content-Type"))
	}

	zipReader, err := zip.NewReader(bytes.NewReader(exportRes.Body.Bytes()), int64(exportRes.Body.Len()))
	if err != nil {
		t.Fatalf("failed to parse exported zip: %v", err)
	}
	manifestFound := false
	dbFound := false
	for _, f := range zipReader.File {
		if f.Name == "manifest.json" {
			manifestFound = true
		}
		if f.Name == "app.db" {
			dbFound = true
		}
	}
	if !manifestFound || !dbFound {
		t.Fatalf("expected manifest.json and app.db in backup archive")
	}

	importRouter, importDB := setupDesktopTestRouter(t)
	importRes := performMultipartFileRequest(t, importRouter, http.MethodPost, "/desktop-data/import", "file", "backup.zip", exportRes.Body.Bytes())
	if importRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", importRes.Code, importRes.Body.String())
	}

	var obsoleteCount int64
	if err := importDB.Model(&models.AppConfig{}).Where("key = ?", "obsolete_private_setting").Count(&obsoleteCount).Error; err != nil {
		t.Fatal(err)
	}
	if obsoleteCount != 0 {
		t.Fatal("obsolete private configuration was restored")
	}

	listRes := performRequest(t, importRouter, http.MethodGet, "/filaments", nil)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", listRes.Code, listRes.Body.String())
	}

	var filaments []apiResponse
	if err := json.Unmarshal(listRes.Body.Bytes(), &filaments); err != nil {
		t.Fatalf("failed to decode filaments list: %v", err)
	}
	if len(filaments) != 1 {
		t.Fatalf("expected one imported filament, got %d", len(filaments))
	}
	if filaments[0]["brand"] != "RoundTrip" {
		t.Fatalf("expected imported brand=RoundTrip, got %#v", filaments[0]["brand"])
	}
}

func TestDesktopDataImportRejectsInvalidZIP(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)

	res := performMultipartFileRequest(t, r, http.MethodPost, "/desktop-data/import", "file", "backup.zip", []byte("not-a-zip"))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if body["error"] != "Backup file must be a valid ZIP archive" {
		t.Fatalf("unexpected error response: %#v", body["error"])
	}
}

func TestDesktopDataImportRejectsMissingManifest(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	archive := buildZipArchive(t, map[string][]byte{
		"app.db": []byte("sqlite-placeholder"),
	})

	res := performMultipartFileRequest(t, r, http.MethodPost, "/desktop-data/import", "file", "backup.zip", archive)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if body["error"] != "Backup manifest is missing" {
		t.Fatalf("unexpected error response: %#v", body["error"])
	}
}

func TestDesktopDataImportRejectsIncompatibleSchemaVersion(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	manifest := map[string]any{
		"format_version": 1,
		"schema_version": 999,
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"driver":         "sqlite",
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	archive := buildZipArchive(t, map[string][]byte{
		"manifest.json": manifestJSON,
		"app.db":        []byte("sqlite-placeholder"),
	})

	res := performMultipartFileRequest(t, r, http.MethodPost, "/desktop-data/import", "file", "backup.zip", archive)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if !strings.Contains(body["error"].(string), "Backup schema version 999 is not compatible") {
		t.Fatalf("unexpected error response: %#v", body["error"])
	}
}

func TestDesktopDataImportRejectsUnsupportedDriver(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	manifest := map[string]any{
		"format_version": 1,
		"schema_version": 1,
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"driver":         "postgres",
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	archive := buildZipArchive(t, map[string][]byte{
		"manifest.json": manifestJSON,
		"app.db":        []byte("sqlite-placeholder"),
	})

	res := performMultipartFileRequest(t, r, http.MethodPost, "/desktop-data/import", "file", "backup.zip", archive)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if body["error"] != "Backup was not generated from a SQLite desktop database" {
		t.Fatalf("unexpected error response: %#v", body["error"])
	}
}

func TestDesktopDataImportRejectsLargeBackupFile(t *testing.T) {
	r, _ := setupDesktopTestRouter(t)
	largePayload := bytes.Repeat([]byte("a"), (64<<20)+1)

	res := performMultipartFileRequest(t, r, http.MethodPost, "/desktop-data/import", "file", "backup.zip", largePayload)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)
	if body["error"] != "Backup file is too large" {
		t.Fatalf("unexpected error response: %#v", body["error"])
	}
}

func assertFloat(t *testing.T, value any, want float64) {
	t.Helper()
	got, ok := value.(float64)
	if !ok {
		t.Fatalf("expected float64 value, got %#v", value)
	}
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("expected %.2f, got %.2f", want, got)
	}
}

func TestDeleteMissingResourceReturns204ForPrintJobs(t *testing.T) {
	r, _ := setupTestRouter(t)
	id := uuid.New().String()

	res := performRequest(t, r, http.MethodDelete, "/print-jobs/"+id, nil)
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for idempotent delete, got %d, body=%s", res.Code, res.Body.String())
	}
}

// TestCrossResourceCostFlow verifies the end-to-end cost calculation pipeline:
// create filament → model → print job → print job filament → GET /print-jobs/{id}/cost
func TestCrossResourceCostFlow(t *testing.T) {
	r, _ := setupTestRouter(t)
	fixture := createModernCostFixture(t, r)

	listRes := performRequest(t, r, http.MethodGet, "/print-jobs", nil)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list print jobs: expected 200, got %d", listRes.Code)
	}
	var jobs []map[string]any
	if err := json.Unmarshal(listRes.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("decode jobs list: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 print job in list, got %d", len(jobs))
	}
	filaments, ok := jobs[0]["filaments"].([]any)
	if !ok || len(filaments) != 1 {
		t.Fatalf("expected 1 filament on print job, got %#v", jobs[0]["filaments"])
	}

	costRes := performRequest(t, r, http.MethodGet, "/print-jobs/"+fixture.JobID+"/cost", nil)
	if costRes.Code != http.StatusOK {
		t.Fatalf("cost endpoint: expected 200, got %d, body=%s", costRes.Code, costRes.Body.String())
	}
	var costBody apiResponse
	decodeBody(t, costRes, &costBody)
	assertFloat(t, costBody["filament_cost"], 2.75)
	assertFloat(t, costBody["electricity_cost"], 0.20)
	assertFloat(t, costBody["wear_cost"], 0.08)
	assertFloat(t, costBody["labor_cost"], 15.00)
	assertFloat(t, costBody["total_cost"], 18.03)

	filteredRes := performRequest(t, r, http.MethodGet, "/print-jobs?printer_id="+fixture.Printer.ID.String(), nil)
	if filteredRes.Code != http.StatusOK {
		t.Fatalf("filter by printer: %d %s", filteredRes.Code, filteredRes.Body.String())
	}
	var filtered apiResponse
	decodeBody(t, filteredRes, &filtered)
	if filtered["total"] != float64(1) {
		t.Fatalf("expected one job for printer, got %#v", filtered["total"])
	}
	missingRes := performRequest(t, r, http.MethodGet, "/print-jobs?printer_id="+uuid.NewString(), nil)
	decodeBody(t, missingRes, &filtered)
	if missingRes.Code != http.StatusOK || filtered["total"] != float64(0) {
		t.Fatalf("unexpected unmatched printer filter: %d %s", missingRes.Code, missingRes.Body.String())
	}
}

func TestFilamentWeightPrecisionRoundTripsAndDrivesCost(t *testing.T) {
	r, db := setupTestRouter(t)
	fixture := createModernCostFixture(t, r)

	var usage models.PrintJobFilament
	if err := db.Where("print_job_id = ?", fixture.JobID).First(&usage).Error; err != nil {
		t.Fatalf("load seeded filament usage: %v", err)
	}
	if err := db.Model(&models.Filament{}).Where("id = ?", fixture.FilamentID).Update("avg_price", 1000.0).Error; err != nil {
		t.Fatalf("set filament price: %v", err)
	}

	updatePayload := map[string]any{
		"print_job_id": fixture.JobID, "filament_id": fixture.FilamentID, "weight_used": 12.34,
	}
	updateRes := performRequest(t, r, http.MethodPut, "/print-job-filaments/"+usage.ID.String(), updatePayload)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("update filament weight: %d %s", updateRes.Code, updateRes.Body.String())
	}
	var updated apiResponse
	decodeBody(t, updateRes, &updated)
	assertFloat(t, updated["weight_used"], 12.34)

	getRes := performRequest(t, r, http.MethodGet, "/print-job-filaments/"+usage.ID.String(), nil)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get updated filament weight: %d %s", getRes.Code, getRes.Body.String())
	}
	var retrieved apiResponse
	decodeBody(t, getRes, &retrieved)
	assertFloat(t, retrieved["weight_used"], 12.34)

	var storedWeight float64
	if err := db.Raw("SELECT weight_used FROM print_job_filaments WHERE id = ?", usage.ID.String()).Scan(&storedWeight).Error; err != nil {
		t.Fatalf("read stored filament weight: %v", err)
	}
	if storedWeight != 12.34 {
		t.Fatalf("SQLite changed weight: got %.17g, want 12.34", storedWeight)
	}

	costRes := performRequest(t, r, http.MethodGet, "/print-jobs/"+fixture.JobID+"/cost", nil)
	if costRes.Code != http.StatusOK {
		t.Fatalf("get cost: %d %s", costRes.Code, costRes.Body.String())
	}
	var cost apiResponse
	decodeBody(t, costRes, &cost)
	assertFloat(t, cost["filament_cost"], 13.57)

	for _, weight := range []float64{0.01, 0.12, 1.23, 12.34, 999.99, 1000.01} {
		createRes := performRequest(t, r, http.MethodPost, "/print-job-filaments", map[string]any{
			"print_job_id": fixture.JobID, "filament_id": fixture.FilamentID, "weight_used": weight,
		})
		if createRes.Code != http.StatusCreated {
			t.Fatalf("create %.2f g usage: %d %s", weight, createRes.Code, createRes.Body.String())
		}
		var created apiResponse
		decodeBody(t, createRes, &created)
		assertFloat(t, created["weight_used"], weight)
		usageID, ok := created["id"].(string)
		if !ok {
			t.Fatalf("created usage has invalid ID: %#v", created["id"])
		}
		getRes := performRequest(t, r, http.MethodGet, "/print-job-filaments/"+usageID, nil)
		if getRes.Code != http.StatusOK {
			t.Fatalf("get %.2f g usage: %d %s", weight, getRes.Code, getRes.Body.String())
		}
		var fetched apiResponse
		decodeBody(t, getRes, &fetched)
		assertFloat(t, fetched["weight_used"], weight)
	}
}
func TestPrintJobsListIncludesEstimatedCost(t *testing.T) {

	r, _ := setupTestRouter(t)
	createModernCostFixture(t, r)

	res := performRequest(t, r, http.MethodGet, "/print-jobs", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", res.Code, res.Body.String())
	}

	var body []apiResponse
	decodeBody(t, res, &body)
	if len(body) != 1 {
		t.Fatalf("expected one print job, got %d", len(body))
	}

	assertFloat(t, body[0]["estimated_cost"], 18.03)
}

func TestCreateAppConfigRejectsUnsupportedKey(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodPost, "/app-config/unknown_key", map[string]any{
		"value": "1.25",
		"type":  "number",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestCreateAppConfigRejectsNonNumberType(t *testing.T) {
	r, _ := setupTestRouter(t)

	res := performRequest(t, r, http.MethodPost, "/app-config/labor_cost_hour", map[string]any{
		"value": "15",
		"type":  "string",
	})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", res.Code, res.Body.String())
	}
}

func TestDashboardSummaryEndpoint(t *testing.T) {
	r, db := setupTestRouter(t)
	fixture := createModernCostFixture(t, r)
	now := time.Now()
	purchaseID := uuid.New().String()
	if err := db.Exec(`INSERT INTO filament_purchases (id, filament_id, price, amount, purchase_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		purchaseID, fixture.FilamentID, 25.0, 1000.0, now, now, now).Error; err != nil {
		t.Fatalf("seed purchase: %v", err)
	}
	var printer models.Printer
	if err := db.First(&printer, "id = ?", fixture.Printer.ID).Error; err != nil {
		t.Fatal(err)
	}
	printer.CostConfig["printer_purchase_cost"] = 400
	printer.CostConfig["printer_upfront_cost"] = 88 // Calculation input: must not be counted as another purchase.
	if err := db.Save(&printer).Error; err != nil {
		t.Fatal(err)
	}
	equipment := `[{"description":"Nozzle","date":"2026-09-01","amount":12,"category":"spare_parts"},{"description":"Ventilation","date":"","amount":3,"category":"additional"},{"description":"Workshop fee","date":"","amount":5,"category":"other"}]`
	if err := db.Create(&models.AppConfig{Key: "printer_equipment_costs", Value: equipment, Type: "json"}).Error; err != nil {
		t.Fatal(err)
	}

	res := performRequest(t, r, http.MethodGet, "/print-jobs/dashboard-summary", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", res.Code, res.Body.String())
	}

	var body apiResponse
	decodeBody(t, res, &body)

	if int(body["total_models"].(float64)) != 1 {
		t.Fatalf("expected total_models=1, got %#v", body["total_models"])
	}
	if int(body["total_filaments"].(float64)) != 1 {
		t.Fatalf("expected total_filaments=1, got %#v", body["total_filaments"])
	}
	if int(body["total_print_jobs"].(float64)) != 1 {
		t.Fatalf("expected total_print_jobs=1, got %#v", body["total_print_jobs"])
	}
	if int(body["total_purchases"].(float64)) != 1 {
		t.Fatalf("expected total_purchases=1, got %#v", body["total_purchases"])
	}
	expenses := body["expenses"].(map[string]any)
	assertFloat(t, expenses["printer_cost"], 400)
	assertFloat(t, expenses["filament_cost"], 25)
	assertFloat(t, expenses["spare_parts_cost"], 12)
	assertFloat(t, expenses["additional_cost"], 3)
	assertFloat(t, expenses["other_cost"], 5)
	assertFloat(t, expenses["total_expenses"], 445)

	costStats, ok := body["cost_stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected cost_stats object, got %#v", body["cost_stats"])
	}
	assertFloat(t, costStats["total_cost"], 18.03)
	if int(costStats["calculated_jobs"].(float64)) != 1 {
		t.Fatalf("expected calculated_jobs=1, got %#v", costStats["calculated_jobs"])
	}

	recent, ok := body["recent_print_jobs"].([]any)
	if !ok || len(recent) != 1 {
		t.Fatalf("expected one recent print job, got %#v", body["recent_print_jobs"])
	}

	firstRecent, ok := recent[0].(map[string]any)
	if !ok {
		t.Fatalf("expected recent print job object, got %#v", recent[0])
	}
	if firstRecent["model_name"] != "FlowModel" {
		t.Fatalf("expected model_name=FlowModel, got %#v", firstRecent["model_name"])
	}
	if firstRecent["printer_name"] != fixture.Printer.Name {
		t.Fatalf("expected printer_name=%s, got %#v", fixture.Printer.Name, firstRecent["printer_name"])
	}
	assertFloat(t, firstRecent["estimated_cost"], 18.03)
}

func TestDashboardSummaryUsesSafeZeroExpenseTotals(t *testing.T) {
	r, _ := setupTestRouter(t)
	res := performRequest(t, r, http.MethodGet, "/print-jobs/dashboard-summary", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", res.Code, res.Body.String())
	}
	var body apiResponse
	decodeBody(t, res, &body)
	for key, value := range body["expenses"].(map[string]any) {
		assertFloat(t, value, 0)
		if key == "total_expenses" && value.(float64) != 0 {
			t.Fatal("empty dashboard fabricated expense data")
		}
	}
}

func TestPrintJobRelationshipStatesAreConsistentAcrossEndpoints(t *testing.T) {
	r, db := setupTestRouter(t)
	now := time.Now()
	modelID, emptyModelID, filamentID := uuid.New(), uuid.New(), uuid.New()
	validJobID, secondValidJobID, unassignedJobID, deletedJobID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	missingModelID, missingFilamentID := uuid.New(), uuid.New()
	if err := db.Create(&models.Model{ID: modelID, Name: "Canonical model", MaterialWeight: 10, PrintTime: 60}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Model{ID: emptyModelID, Name: "No jobs", MaterialWeight: 10, PrintTime: 60}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Filament{ID: filamentID, Brand: "Maker", Type: "PLA", Color: "Blue", AvgPrice: 20}).Error; err != nil {
		t.Fatal(err)
	}
	minutes := 60
	for _, job := range []models.PrintJob{
		{ID: validJobID, ModelID: &modelID, PrintDate: now, UnitsPrinted: 1, LaborHours: 0, PrintTimeMinutes: &minutes},
		{ID: secondValidJobID, ModelID: &modelID, PrintDate: now.Add(-30 * time.Second), UnitsPrinted: 1, LaborHours: 0, PrintTimeMinutes: &minutes},
		{ID: unassignedJobID, PrintDate: now.Add(-time.Minute), UnitsPrinted: 1, LaborHours: 0, PrintTimeMinutes: &minutes},
		{ID: deletedJobID, ModelID: &missingModelID, PrintDate: now.Add(-2 * time.Minute), UnitsPrinted: 1, LaborHours: 0},
	} {
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.PrintJobFilament{ID: uuid.New(), PrintJobID: validJobID, FilamentID: filamentID, WeightUsed: 10}).Error; err != nil {
		t.Fatal(err)
	}
	// This legacy row is deliberately orphaned: it remains visible as unavailable and cannot calculate a cost.
	if err := db.Create(&models.PrintJobFilament{ID: uuid.New(), PrintJobID: deletedJobID, FilamentID: missingFilamentID, WeightUsed: 10}).Error; err != nil {
		t.Fatal(err)
	}

	jobsResponse := performRequest(t, r, http.MethodGet, "/print-jobs", nil)
	if jobsResponse.Code != http.StatusOK {
		t.Fatal(jobsResponse.Body.String())
	}
	var jobs []apiResponse
	decodeBody(t, jobsResponse, &jobs)
	states := map[string]apiResponse{}
	for _, job := range jobs {
		states[job["id"].(string)] = job["model"].(map[string]any)
	}
	if states[validJobID.String()]["state"] != "assigned" || states[validJobID.String()]["name"] != "Canonical model" {
		t.Fatalf("valid model state: %#v", states[validJobID.String()])
	}
	if states[unassignedJobID.String()]["state"] != "not_assigned" || states[unassignedJobID.String()]["name"] != "Model not assigned" {
		t.Fatalf("unassigned model state: %#v", states[unassignedJobID.String()])
	}
	if states[deletedJobID.String()]["state"] != "deleted" || states[deletedJobID.String()]["name"] != "Deleted model" {
		t.Fatalf("deleted model state: %#v", states[deletedJobID.String()])
	}
	for _, job := range jobs {
		if job["id"] == deletedJobID.String() && job["cost"].(map[string]any)["state"] != "not_calculated" {
			t.Fatalf("orphaned usage must not calculate: %#v", job["cost"])
		}
	}

	modelsResponse := performRequest(t, r, http.MethodGet, "/models", nil)
	var modelsBody []apiResponse
	decodeBody(t, modelsResponse, &modelsBody)
	modelCounts := map[string]int{}
	for _, model := range modelsBody {
		modelCounts[model["id"].(string)] = int(model["print_job_count"].(float64))
	}
	if modelCounts[modelID.String()] != 2 || modelCounts[emptyModelID.String()] != 0 {
		t.Fatalf("model count must use model_id association: %#v", modelsBody)
	}

	dashboardResponse := performRequest(t, r, http.MethodGet, "/print-jobs/dashboard-summary", nil)
	var dashboard apiResponse
	decodeBody(t, dashboardResponse, &dashboard)
	for _, recent := range dashboard["recent_print_jobs"].([]any) {
		recentJob := recent.(map[string]any)
		if got, want := recentJob["model"].(map[string]any)["state"], states[recentJob["id"].(string)]["state"]; got != want {
			t.Fatalf("dashboard model state %v differs from print jobs %v", got, want)
		}
	}
}
