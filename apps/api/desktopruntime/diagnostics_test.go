package desktopruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testEvents(t *testing.T) *diagnostics.Logger {
	t.Helper()
	c := diagnostics.DefaultConfig()
	c.MinimumLevel = diagnostics.Debug
	e, err := diagnostics.New(c, diagnostics.Identity{Version: "dev"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

func TestRequestDiagnosticsRecoverWithoutLeakingPayload(t *testing.T) {
	events := testEvents(t)
	r := gin.New()
	r.Use(diagnosticMiddleware(events))
	r.POST("/models", func(c *gin.Context) { panic("secret-password from request") })
	r.GET("/desktop-data/export", func(c *gin.Context) { c.Status(200) })
	r.GET("/models", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("POST", "/models?token=secret-password", strings.NewReader("secret-password"))
	req.Header.Set("Authorization", "Bearer secret-password")
	req.Header.Set("X-Request-ID", "secret-password")
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if out.Code != 500 || strings.Contains(out.Body.String(), "secret-password") {
		t.Fatalf("unsafe response: %s", out.Body.String())
	}
	id, err := uuid.Parse(out.Header().Get("X-Request-ID"))
	if err != nil || id.Version() != 4 {
		t.Fatal("request ID not generated locally")
	}
	for path, want := range map[string]int{"/models": 503, "/desktop-data/export": 200} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != want {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
	for _, raw := range events.Recent() {
		if bytes.Contains(raw, []byte("secret-password")) {
			t.Fatal("sensitive request data leaked")
		}
	}
}

func TestDatabaseLoggerNeverEvaluatesSQL(t *testing.T) {
	events := testEvents(t)
	l := databaseLogger{events: events, level: logger.Warn}
	id := uuid.NewString()
	ctx := context.WithValue(context.Background(), requestIDKey{}, id)
	sql := func() (string, int64) { t.Fatal("SQL callback evaluated"); return "", 0 }
	l.Trace(ctx, time.Now(), sql, gorm.ErrRecordNotFound)
	if len(events.Recent()) != 0 {
		t.Fatal("expected missing row logged as failure")
	}
	l.Trace(ctx, time.Now(), sql, errors.New("secret SQL values"))
	l.Trace(ctx, time.Now().Add(-time.Second), sql, nil)
	for _, raw := range events.Recent() {
		if bytes.Contains(raw, []byte("secret")) || !bytes.Contains(raw, []byte(id)) {
			t.Fatalf("unsafe or uncorrelated event: %s", raw)
		}
	}
}

func TestRuntimeRoutesAndCORSContracts(t *testing.T) {
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "app.db"))
	events := testEvents(t)
	server, err := NewHTTPServerWithDiagnostics("127.0.0.1:8080", events)
	if err != nil {
		t.Fatal(err)
	}
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 || server.MaxHeaderBytes <= 0 {
		t.Fatal("HTTP server must enforce finite request and connection limits")
	}
	for path, want := range map[string]int{"/desktop-data/export": 200, "/filaments": 200} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Origin", "wails://wails")
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("Access-Control-Expose-Headers") != "X-Request-ID" {
			t.Fatal("correlation header not exposed")
		}
	}
	req := httptest.NewRequest("GET", "/desktop-data/export", nil)
	req.Header.Set("Origin", "https://secret-client.invalid")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, req)
	if response.Code != 403 {
		t.Fatal("CORS bypass")
	}
	// Missing Origin is intentionally accepted for Wails/native and local
	// process clients. This is not caller authentication.
	req = httptest.NewRequest("GET", "/desktop-data/export", nil)
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("missing Origin request returned %d", response.Code)
	}
	// Import is a state-changing endpoint: a browser origin outside the
	// allowlist must be rejected before the handler sees its request.
	req = httptest.NewRequest(http.MethodPost, "/desktop-data/import", strings.NewReader("invalid upload"))
	req.Header.Set("Origin", "https://secret-client.invalid")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unlisted-origin import returned %d", response.Code)
	}
	// Origin-free callers still reach the import handler. The handler rejects
	// this malformed payload, but local-process access remains unauthenticated.
	req = httptest.NewRequest(http.MethodPost, "/desktop-data/import", strings.NewReader("invalid upload"))
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing-Origin import did not reach handler validation: %d", response.Code)
	}
	for _, raw := range events.Recent() {
		if bytes.Contains(raw, []byte("secret-client")) {
			t.Fatal("raw origin logged")
		}
	}
}

func TestDatabaseRequestContextAndMutationLogging(t *testing.T) {
	events := testEvents(t)
	r := gin.New()
	r.Use(diagnosticMiddleware(events))
	r.POST("/models", func(c *gin.Context) {
		databaseLogger{events: events, level: logger.Warn}.Trace(c.Request.Context(), time.Now(), nil, errors.New("secret"))
		c.Status(http.StatusCreated)
	})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", "/models", nil))
	for _, raw := range events.Recent() {
		var event struct {
			RequestID string `json:"request_id"`
			Event     string `json:"event"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.RequestID != response.Header().Get("X-Request-ID") {
			t.Fatal("missing request correlation")
		}
	}
	if !bytes.Contains(events.Recent()[1], []byte("http.mutation.completed")) {
		t.Fatal("successful mutation missing")
	}
}
