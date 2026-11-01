package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
)

func setupDiagnosticApp(t *testing.T) *App {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("LOCALAPPDATA", state)
	t.Setenv("DIAGNOSTICS_LEVEL", "INFO")
	t.Setenv("DIAGNOSTICS_MAX_FILE_MIB", "10")
	a := NewApp()
	a.initializeDiagnostics()
	if a.events == nil {
		t.Fatal("logger not initialized")
	}
	t.Cleanup(a.closeDiagnostics)
	return a
}

func TestDesktopCreatesLogsBeforeDatabaseInitialization(t *testing.T) {
	a := setupDiagnosticApp(t)
	// An invalid path triggers startup failure before the API can bind a port.
	badParent := filepath.Join(t.TempDir(), "secret-customer")
	if err := os.WriteFile(badParent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SQLITE_PATH", filepath.Join(badParent, "app.db"))
	a.startup(context.Background())
	if a.GetRuntimeStatus().FailureCode != string(diagnostics.CodeSQLiteInit) {
		t.Fatal("startup failure not surfaced")
	}
	a.shutdown(context.Background())
	a.closeDiagnostics()
	dir, err := diagnostics.Directory()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "application.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("app.starting")) || !bytes.Contains(data, []byte("app.startup.failed")) {
		t.Fatalf("missing startup events: %s", data)
	}
	if bytes.Contains(data, []byte("secret-customer")) {
		t.Fatal("sensitive path in log")
	}
}

func TestFrontendAndWailsAdaptersRejectPayloadsAndRateLimit(t *testing.T) {
	a := setupDiagnosticApp(t)
	a.ReportFrontendFailure("secret-password", "secret-password")
	for i := 0; i < 100; i++ {
		a.ReportFrontendFailure("render", "secret-password")
	}
	l := &desktopLogger{app: a}
	l.Error("secret-password")
	l.Error("secret-password")
	var renderCount, desktopCount int
	for _, raw := range a.events.Recent() {
		if bytes.Contains(raw, []byte("secret-password")) {
			t.Fatal("raw adapter value leaked")
		}
		if bytes.Contains(raw, []byte(`"event":"frontend.failed"`)) {
			renderCount++
		}
		if bytes.Contains(raw, []byte(`"event":"desktop.runtime.error"`)) {
			desktopCount++
		}
	}
	if renderCount != 1 || desktopCount != 1 {
		t.Fatal("adapter flood not limited")
	}
}

func TestDiagnosticDirectoryFailureFallsBackToMemory(t *testing.T) {
	state := t.TempDir()
	blocked := filepath.Join(state, "blocked")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocked)
	t.Setenv("LOCALAPPDATA", blocked)
	a := NewApp()
	a.initializeDiagnostics()
	defer a.closeDiagnostics()
	if a.events == nil || a.GetRuntimeStatus().LogsAvailable {
		t.Fatal("expected memory-only fallback")
	}
	if len(a.events.Recent()) == 0 {
		t.Fatal("fallback lost startup context")
	}
}

func TestSecondInstanceDoesNotInitializeDatabaseOrBind(t *testing.T) {
	first := setupDiagnosticApp(t)
	if !first.events.PersistenceAvailable() {
		t.Fatal("first writer unavailable")
	}
	second := NewApp()
	second.initializeDiagnostics()
	defer second.closeDiagnostics()
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "must-not-be-created.db"))
	second.startup(context.Background())
	if second.GetRuntimeStatus().FailureCode != "DIAGNOSTICS_IN_USE" {
		t.Fatal("second instance attempted startup")
	}
	if _, err := os.Stat(os.Getenv("SQLITE_PATH")); !os.IsNotExist(err) {
		t.Fatal("second instance opened database")
	}
}
