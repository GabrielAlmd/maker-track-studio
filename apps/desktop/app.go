package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/desktopruntime"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
)

type App struct {
	mu              sync.Mutex
	ctx             context.Context
	httpServer      *http.Server
	apiRuntime      *desktopruntime.Runtime
	events          *diagnostics.Logger
	failureCode     string
	logsFailureCode string
	stopping        bool
	shutdownOnce    sync.Once
	frontendLast    [4]time.Time
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	defer func() {
		if recover() != nil {
			a.fail(diagnostics.CodePanic, diagnostics.StateUnsafe)
		}
	}()
	a.ctx = ctx
	handlers.ReportApplicationVersion = version
	if a.GetRuntimeStatus().FailureCode != "" {
		return
	}

	runtime, err := desktopruntime.New(desktopruntime.Config{Addr: "127.0.0.1:8080", Events: a.events})
	if err != nil {
		a.fail(diagnostics.CodeSQLiteInit, diagnostics.StartupFailed)
		return
	}
	server := runtime.Server

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		_ = runtime.Shutdown(context.Background())
		a.fail(diagnostics.CodeAPIBind, diagnostics.StartupFailed)
		return
	}

	a.mu.Lock()
	if a.stopping {
		a.mu.Unlock()
		_ = listener.Close()
		_ = runtime.Shutdown(context.Background())
		return
	}
	a.httpServer = server
	a.apiRuntime = runtime
	a.emit(diagnostics.Input{Event: diagnostics.AppReady})
	a.mu.Unlock()

	go func() {
		defer func() {
			if recover() != nil {
				a.fail(diagnostics.CodePanic, diagnostics.StateUnsafe)
			}
		}()
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.fail(diagnostics.CodeAPIServe, diagnostics.APIStopped)
		}
	}()
}

func (a *App) shutdown(_ context.Context) {
	a.shutdownOnce.Do(func() {
		a.mu.Lock()
		a.stopping = true
		server := a.httpServer
		runtime := a.apiRuntime
		a.emit(diagnostics.Input{Event: diagnostics.AppStopping})
		a.mu.Unlock()
		// Wails may cancel its context before OnShutdown. Use an independent
		// bounded context to finish active requests before closing diagnostics.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clean := true
		if runtime != nil {
			if err := runtime.Shutdown(ctx); err != nil {
				clean = false
				a.emit(diagnostics.Input{Event: diagnostics.ShutdownFailed, ErrorCode: diagnostics.CodeTimeout})
			}
		} else if server != nil {
			if err := server.Shutdown(ctx); err != nil {
				clean = false
				_ = server.Close()
			}
		}
		if clean {
			a.emit(diagnostics.Input{Event: diagnostics.AppStopped})
		}
	})
}
