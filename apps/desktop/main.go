package main

import (
	"context"
	"embed"
	"encoding/json"
	"os"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/gin-gonic/gin"

	"github.com/wailsapp/wails/v2"
	wailslogger "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if err := json.NewEncoder(os.Stdout).Encode(buildInfo()); err != nil {
			return 1
		}
		return 0
	}
	if len(os.Args) == 3 && (os.Args[1] == "--set-locale" || os.Args[1] == "--set-locale-if-unset") {
		if err := writeLocalePreference(os.Args[2], os.Args[1] == "--set-locale-if-unset"); err != nil {
			return 1
		}
		return 0
	}
	app := NewApp()
	app.initializeDiagnostics()
	defer app.closeDiagnostics()
	defer app.shutdown(context.Background())
	gin.SetMode(gin.ReleaseMode)

	err := wails.Run(&options.App{
		Title:  buildTitle(),
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:          app.startup,
		OnShutdown:         app.shutdown,
		Logger:             &desktopLogger{app: app},
		LogLevelProduction: wailslogger.WARNING,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		app.fail(diagnostics.CodeUnknown, diagnostics.DesktopFatal)
		return 1
	}
	if app.GetRuntimeStatus().FailureCode != "" {
		return 1
	}
	return 0
}
