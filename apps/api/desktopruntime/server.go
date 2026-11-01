package desktopruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/appmodules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/routes"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewHTTPServer(addr string) (*http.Server, error) {
	return NewHTTPServerWithDiagnostics(addr, nil)
}

func NewHTTPServerWithDiagnostics(addr string, events *diagnostics.Logger) (*http.Server, error) {
	runtime, err := New(Config{Addr: addr, SQLitePath: strings.TrimSpace(os.Getenv("SQLITE_PATH")), Events: events})
	if err != nil {
		return nil, err
	}
	runtime.Server.RegisterOnShutdown(func() { _ = runtime.close() })
	return runtime.Server, nil
}

// Config selects the shared API runtime. An empty SQLitePath preserves the
// historical desktop location (or SQLITE_PATH override).
type Config struct {
	Addr            string
	SQLitePath      string
	Events          *diagnostics.Logger
	TrustedProxies  []string
	SameOriginProxy bool
	LANAccess       *LANAccess
	Backups         *BackupConfig
}

// Runtime owns the HTTP server and its database connection pool.
type Runtime struct {
	Server      *http.Server
	db          *gorm.DB
	owner       *os.File
	gate        chan struct{}
	stop        context.CancelFunc
	stopped     context.Context
	workers     sync.WaitGroup
	closeOnce   sync.Once
	closeErr    error
	recoveryDir string
}

func New(config Config) (*Runtime, error) {
	if config.LANAccess != nil {
		if err := config.LANAccess.validate(config); err != nil {
			return nil, err
		}
	}
	addr := config.Addr
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if config.Backups != nil {
		if err := config.Backups.validate(); err != nil {
			return nil, err
		}
	}
	path := strings.TrimSpace(config.SQLitePath)
	var err error
	if path == "" {
		path, err = resolveSQLitePath()
		if err != nil {
			return nil, err
		}
	}
	path, owner, err := ownDatabase(path)
	if err != nil {
		return nil, err
	}
	ready := false
	defer func() {
		if !ready {
			_ = owner.Close()
		}
	}()
	installed := appmodules.Default()
	db, err := initSQLiteDBAtPath(config.Events, installed, path)
	if err != nil {
		return nil, err
	}

	stopped, stop := context.WithCancel(context.Background())
	r := &Runtime{db: db, owner: owner, gate: make(chan struct{}, 1), stopped: stopped, stop: stop,
		recoveryDir: path + ".recovery"}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err := router.SetTrustedProxies(config.TrustedProxies); err != nil {
		closeDatabase(db)
		return nil, fmt.Errorf("invalid trusted proxy configuration")
	}
	router.Use(diagnosticMiddleware(config.Events))
	router.Use(securityHeadersMiddleware())
	if config.LANAccess != nil {
		router.Use(config.LANAccess.middleware(config.TrustedProxies))
	}
	if config.SameOriginProxy {
		router.Use(sameOriginProxyMiddleware(config.Events, config.TrustedProxies))
	} else {
		router.Use(corsMiddlewareWithDiagnostics(config.Events))
	}
	router.Use(r.workspaceMiddleware())
	router.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.GET("/health/ready", func(c *gin.Context) {
		sqlDB, dbErr := db.DB()
		if dbErr != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	routes.RegisterRoutesWithModules(router, db, installed, config.Events)

	r.Server = &http.Server{
		Addr:              addr,
		Handler:           router,
		ErrorLog:          safeHTTPErrorLog(config.Events),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	ready = true
	if config.Backups != nil {
		r.startBackups(*config.Backups)
	}
	return r, nil
}

// sameOriginProxyMiddleware supports a browser UI served by the same reverse
// proxy. It emits no CORS headers because same-origin requests need none.
func sameOriginProxyMiddleware(events *diagnostics.Logger, trustedProxies []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		parsed, err := url.Parse(origin)
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		} else if forwarded := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))); trustedProxyPeer(c.Request.RemoteAddr, trustedProxies) && (forwarded == "https" || forwarded == "http") {
			scheme = forwarded
		}
		if err == nil && parsed.User == nil && parsed.Host != "" && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" &&
			strings.EqualFold(parsed.Scheme, scheme) && strings.EqualFold(parsed.Host, c.Request.Host) {
			c.Next()
			return
		}
		emit(events, diagnostics.Input{Event: diagnostics.OriginRejected, RequestID: requestID(c.Request.Context())})
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "CORS_ORIGIN_DENIED", "error": "Request origin is not allowed"})
	}
}

func trustedProxyPeer(remoteAddr string, trusted []string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return false
	}
	for _, entry := range trusted {
		if ip := net.ParseIP(entry); ip != nil && ip.Equal(peer) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(peer) {
			return true
		}
	}
	return false
}

// Shutdown stops accepting requests and closes SQLite even when graceful
// shutdown times out. The caller can report the returned shutdown error.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	err := r.Server.Shutdown(ctx)
	if err != nil {
		_ = r.Server.Close()
	}
	closeErr := r.close()
	if err != nil {
		return err
	}
	return closeErr
}

func closeDatabase(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func initSQLiteDB() (*gorm.DB, error) {
	return initSQLiteDBWithDiagnostics(nil)
}

func initSQLiteDBWithDiagnostics(events *diagnostics.Logger) (*gorm.DB, error) {
	return initSQLiteDBWithModules(events, appmodules.Default())
}

func initSQLiteDBWithModules(events *diagnostics.Logger, installed *modules.Registry) (*gorm.DB, error) {
	return initSQLiteDBAtPath(events, installed, "")
}

func initSQLiteDBAtPath(events *diagnostics.Logger, installed *modules.Registry, configuredPath string) (*gorm.DB, error) {
	sqlitePath := strings.TrimSpace(configuredPath)
	var err error
	if sqlitePath == "" {
		sqlitePath, err = resolveSQLitePath()
	}
	if err != nil {
		return nil, err
	}
	sqlitePath, err = filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o700); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(sqliteDSN(sqlitePath)), &gorm.Config{Logger: databaseLogger{events: events, level: logger.Warn}})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	ready := false
	defer func() {
		if !ready {
			if sqlDB, dbErr := db.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
		}
	}()
	if err := installed.CheckBackupTables(db); err != nil {
		return nil, fmt.Errorf("database module compatibility: %w", err)
	}

	legacyPrinterSchema := !db.Migrator().HasTable(&models.Printer{}) &&
		(db.Migrator().HasTable(&models.AppConfig{}) || db.Migrator().HasTable(&models.PrintJob{}))

	if err := db.AutoMigrate(
		&models.Filament{},
		&models.FilamentPurchase{},
		&models.Model{},
		&models.PrintJob{},
		&models.PrintJobFilament{},
		&models.Printer{},
		&models.AppConfig{},
	); err != nil {
		return nil, err
	}
	if err := installed.Migrate(db); err != nil {
		return nil, err
	}

	if err := seedDefaultAppConfig(db, installed); err != nil {
		return nil, err
	}
	if err := migrateMissingPrinterCostConfigs(db); err != nil {
		return nil, err
	}
	if legacyPrinterSchema {
		err = db.Transaction(models.MigrateLegacyPrintJobCosts)
	} else {
		err = migrateLegacyPrintJobCosts(db)
	}
	if err != nil {
		return nil, err
	}

	emit(events, diagnostics.Input{Event: diagnostics.DatabaseReady})
	ready = true
	return db, nil
}

func seedDefaultAppConfig(db *gorm.DB, installed *modules.Registry) error {
	now := time.Now()
	featureValue, err := json.Marshal(installed.DefaultFeatures())
	if err != nil {
		return err
	}
	defaults := []models.AppConfig{
		{Key: models.AppFeaturesConfigKey, Value: string(featureValue), Type: "json", CreatedAt: now, UpdatedAt: now},
		{Key: "electricity_cost_kwh", Value: "0.20", Type: "number", CreatedAt: now, UpdatedAt: now},
		{Key: "printer_wear_cost_hour", Value: "0.08", Type: "number", CreatedAt: now, UpdatedAt: now},
		{Key: "labor_cost_hour", Value: "15.00", Type: "number", CreatedAt: now, UpdatedAt: now},
		{Key: "filament_waste_factor", Value: "1.10", Type: "number", CreatedAt: now, UpdatedAt: now},
	}

	for _, entry := range defaults {
		cfg := entry
		if err := db.Where("key = ?", cfg.Key).FirstOrCreate(&cfg).Error; err != nil {
			return fmt.Errorf("failed to seed app config %s: %w", cfg.Key, err)
		}
	}

	return nil
}

// migrateMissingPrinterCostConfigs completes the additive printer-profile
// migration for installations created before printer cost ownership existed.
// It copies the current new-printer template only into missing profiles; it
// never overwrites a configured printer or a Print Job snapshot.
func migrateMissingPrinterCostConfigs(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var printers []models.Printer
		if err := tx.Where("cost_config IS NULL").Find(&printers).Error; err != nil {
			return err
		}
		if len(printers) == 0 {
			return nil
		}
		var rows []models.AppConfig
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		template := models.PrinterCostConfigFromAppConfig(rows)
		if err := models.ValidatePrinterCostConfig(template); err != nil {
			return fmt.Errorf("invalid new-printer cost template: %w", err)
		}
		for i := range printers {
			printers[i].CostConfig = template
			if err := tx.Save(&printers[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func migrateLegacyPrintJobCosts(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.PrintJob{}).
		Where("printer_id IS NULL OR printer_cost IS NULL").
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return db.Transaction(models.MigrateLegacyPrintJobCosts)
}

func resolveSQLitePath() (string, error) {
	sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if sqlitePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		if runtime.GOOS == "windows" {
			sqlitePath = filepath.Join(homeDir, "AppData", "Roaming", "3d-printing-cost-tracker", "app.db")
		} else {
			sqlitePath = filepath.Join(homeDir, ".local", "share", "3d-printing-cost-tracker", "app.db")
		}
	}

	if err := os.MkdirAll(filepath.Dir(sqlitePath), 0o700); err != nil {
		return "", err
	}

	return sqlitePath, nil
}

func corsMiddleware() gin.HandlerFunc {
	return corsMiddlewareWithDiagnostics(nil)
}

func corsMiddlewareWithDiagnostics(events *diagnostics.Logger) gin.HandlerFunc {
	allowedOrigins := parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))
	allowedOriginsSet := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowedOriginsSet[origin] = struct{}{}
	}

	return gin.HandlerFunc(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowedOriginsSet[origin]; !ok {
				emit(events, diagnostics.Input{Event: diagnostics.OriginRejected, RequestID: requestID(c.Request.Context())})
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "CORS_ORIGIN_DENIED", "error": "Request origin is not allowed"})
				return
			}
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})
}

func securityHeadersMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	})
}

func parseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			// Wails development window and browser server.
			"http://localhost:34115",
			"http://127.0.0.1:34115",
			"wails://wails",
			"http://wails.localhost",
			"https://wails.localhost",
		}
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func sqliteDSN(path string) string {
	normalized := filepath.ToSlash(path)
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	u := url.URL{Scheme: "file", Path: normalized}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "DELETE")
	q.Set("_synchronous", "FULL")
	u.RawQuery = q.Encode()
	return u.String()
}
