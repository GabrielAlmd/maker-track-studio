package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
)

func (a *App) initializeDiagnostics() {
	config, fallback := diagnostics.EnvironmentConfig()
	directory, dirErr := diagnostics.Directory()
	if dirErr != nil {
		a.logsFailureCode = "DIAG_DIRECTORY_PATH"
	}
	var sink diagnostics.Sink
	if dirErr == nil {
		writer, err := diagnostics.NewRotatingWriter(directory, config)
		if err == nil {
			sink = writer
		}
		if err != nil {
			a.logsFailureCode = diagnostics.FailureCode(err)
		}
		if errors.Is(err, diagnostics.ErrWriterInUse) {
			a.failureCode = "DIAGNOSTICS_IN_USE"
		}
	}
	events, err := diagnostics.New(config, diagnostics.Identity{Version: version, Commit: commit}, sink)
	if err != nil {
		if sink != nil {
			_ = sink.Close()
		}
		_, _ = os.Stderr.WriteString("Application diagnostics unavailable.\n")
		return
	}
	a.events = events
	if sink == nil {
		events.Emit(diagnostics.Input{Event: diagnostics.PersistenceFailed})
		_, _ = os.Stderr.WriteString("Diagnostic files unavailable; using bounded memory only. " + a.logsFailureCode + "\n")
	}
	if fallback {
		events.Emit(diagnostics.Input{Event: diagnostics.ConfigFallback})
	}
	events.Emit(diagnostics.Input{Event: diagnostics.AppStarting})
}

func (a *App) emit(input diagnostics.Input) {
	if a.events != nil {
		a.events.Emit(input)
	}
}

type RuntimeStatus struct {
	APIReady        bool   `json:"api_ready"`
	FailureCode     string `json:"failure_code"`
	LogsAvailable   bool   `json:"logs_available"`
	LogsFailureCode string `json:"logs_failure_code"`
}

// GetRuntimeStatus exposes only technical flags, never paths or raw errors.
func (a *App) GetRuntimeStatus() RuntimeStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	available := a.events != nil && a.events.PersistenceAvailable()
	code := a.logsFailureCode
	if available {
		code = ""
	} else if code == "" {
		code = "DIAG_UNAVAILABLE"
	}
	return RuntimeStatus{APIReady: a.httpServer != nil && a.failureCode == "" && !a.stopping,
		FailureCode: a.failureCode, LogsAvailable: available, LogsFailureCode: code}
}

func (a *App) fail(code diagnostics.ErrorCode, event diagnostics.EventID) {
	a.mu.Lock()
	a.failureCode = string(code)
	a.mu.Unlock()
	a.emit(diagnostics.Input{Event: event, ErrorCode: code})
}

// ReportFrontendFailure accepts a fixed vocabulary, not a browser Error object.
// Both JS and Go rate-limit; a modified frontend cannot flood the log bridge.
func (a *App) ReportFrontendFailure(kind, requestID string) {
	var index int
	var event diagnostics.EventID
	switch kind {
	case "render":
		index, event = 0, diagnostics.FrontendFailed
	case "script":
		index, event = 1, diagnostics.FrontendScriptFailed
	case "promise":
		index, event = 2, diagnostics.FrontendPromiseFailed
	case "network":
		index, event = 3, diagnostics.FrontendNetworkFailed
	default:
		return
	}
	if len(requestID) > 36 {
		requestID = ""
	}
	a.mu.Lock()
	now := time.Now()
	if now.Sub(a.frontendLast[index]) < 5*time.Second {
		a.mu.Unlock()
		return
	}
	a.frontendLast[index] = now
	a.mu.Unlock()
	a.emit(diagnostics.Input{Event: event, RequestID: requestID, ErrorCode: diagnostics.CodeFrontend})
}

// Wails messages may include arbitrary values. Keep severity notifications only,
// with a bounded rate, rather than forwarding their text to any output.
type desktopLogger struct {
	app  *App
	mu   sync.Mutex
	last [3]time.Time
}

func (*desktopLogger) Print(string) {}
func (*desktopLogger) Trace(string) {}
func (*desktopLogger) Debug(string) {}
func (*desktopLogger) Info(string)  {}
func (l *desktopLogger) record(index int, event diagnostics.EventID) {
	l.mu.Lock()
	now := time.Now()
	if now.Sub(l.last[index]) < 5*time.Second {
		l.mu.Unlock()
		return
	}
	l.last[index] = now
	l.mu.Unlock()
	l.app.emit(diagnostics.Input{Event: event})
}
func (l *desktopLogger) Warning(string) { l.record(0, diagnostics.DesktopWarning) }
func (l *desktopLogger) Error(string)   { l.record(1, diagnostics.DesktopError) }
func (l *desktopLogger) Fatal(string) {
	l.record(2, diagnostics.DesktopFatal)
	l.app.mu.Lock()
	l.app.failureCode = "DESKTOP_RUNTIME_FAILED"
	l.app.mu.Unlock()
}

func (a *App) closeDiagnostics() {
	if a.events != nil {
		_ = a.events.Close(context.Background())
	}
}
