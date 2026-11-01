package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/desktopruntime"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
)

type serverConfig struct {
	addr             string
	sqlitePath       string
	diagnosticsDir   string
	diagnosticsLevel diagnostics.Level
	shutdownTimeout  time.Duration
	trustedProxies   []string
	lanAccess        *desktopruntime.LANAccess
	backups          *desktopruntime.BackupConfig
}

func loadConfig() (serverConfig, error) {
	secretPath := strings.TrimSpace(os.Getenv("API_PROXY_SECRET_FILE"))
	info, err := os.Stat(secretPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 || info.Mode().Perm()&0o007 != 0 || info.Mode().Perm()&0o022 != 0 {
		return serverConfig{}, errors.New("API_PROXY_SECRET_FILE must be a protected regular file (0600 or 0640)")
	}
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return serverConfig{}, errors.New("could not read proxy secret")
	}
	access := &desktopruntime.LANAccess{Origin: os.Getenv("API_PUBLIC_ORIGIN"), ProxySecret: strings.TrimSpace(string(secret))}
	if len(access.ProxySecret) < 64 {
		return serverConfig{}, errors.New("proxy secret must contain at least 64 characters")
	}
	address := envOr("API_BIND_ADDRESS", "127.0.0.1")
	ip := net.ParseIP(address)
	if ip == nil || !ip.IsLoopback() {
		return serverConfig{}, errors.New("API_BIND_ADDRESS must be a loopback IP address")
	}
	port := 8080
	if raw := os.Getenv("API_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return serverConfig{}, errors.New("API_PORT must be between 1 and 65535")
		}
		port = parsed
	}
	sqlitePath := strings.TrimSpace(os.Getenv("API_SQLITE_PATH"))
	if sqlitePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return serverConfig{}, errors.New("API_SQLITE_PATH is required when the home directory is unavailable")
		}
		sqlitePath = filepath.Join(home, ".local", "share", "maker-track-studio", "server.db")
	}
	level := diagnostics.Info
	if raw := os.Getenv("API_DIAGNOSTICS_LEVEL"); raw != "" {
		level = diagnostics.Level(strings.ToUpper(raw))
		if _, ok := level.SlogLevel(); !ok {
			return serverConfig{}, errors.New("API_DIAGNOSTICS_LEVEL must be DEBUG, INFO, WARNING, ERROR or CRITICAL")
		}
	}
	shutdownSeconds := 10
	if raw := os.Getenv("API_SHUTDOWN_TIMEOUT_SECONDS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 300 {
			return serverConfig{}, errors.New("API_SHUTDOWN_TIMEOUT_SECONDS must be between 1 and 300")
		}
		shutdownSeconds = parsed
	}
	proxies := splitList(os.Getenv("API_TRUSTED_PROXIES"))
	for _, proxy := range proxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return serverConfig{}, errors.New("API_TRUSTED_PROXIES must contain IP addresses or CIDRs")
			}
		}
	}
	dir := strings.TrimSpace(os.Getenv("API_DIAGNOSTICS_DIR"))
	if dir == "" {
		var err error
		dir, err = diagnostics.Directory()
		if err != nil {
			return serverConfig{}, errors.New("diagnostic directory unavailable")
		}
	}
	var backups *desktopruntime.BackupConfig
	backupDir := strings.TrimSpace(os.Getenv("API_BACKUP_DIR"))
	if backupDir != "" {
		retention, err := strconv.Atoi(envOr("API_BACKUP_RETENTION", "14"))
		if err != nil || retention < 1 || retention > 3650 {
			return serverConfig{}, errors.New("API_BACKUP_RETENTION must be between 1 and 3650")
		}
		clock := envOr("API_BACKUP_TIME_UTC", "03:00")
		if _, err := time.Parse("15:04", clock); err != nil || len(clock) != 5 {
			return serverConfig{}, errors.New("API_BACKUP_TIME_UTC must be HH:MM")
		}
		backups = &desktopruntime.BackupConfig{Directory: backupDir, TimeUTC: clock, Retention: retention,
			OnError: func() {
				fmt.Fprintln(os.Stderr, "Scheduled backup failed; check destination availability and free space.")
			}}
	} else if os.Getenv("API_BACKUP_RETENTION") != "" || os.Getenv("API_BACKUP_TIME_UTC") != "" {
		return serverConfig{}, errors.New("API_BACKUP_DIR is required to enable scheduled backups")
	}
	return serverConfig{addr: net.JoinHostPort(address, strconv.Itoa(port)), sqlitePath: sqlitePath,
		diagnosticsDir: dir, diagnosticsLevel: level, shutdownTimeout: time.Duration(shutdownSeconds) * time.Second,
		trustedProxies: proxies, lanAccess: access, backups: backups}, nil
}

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid API configuration:", err)
		os.Exit(2)
	}
	diagConfig := diagnostics.DefaultConfig()
	diagConfig.MinimumLevel = config.diagnosticsLevel
	writer, err := diagnostics.NewRotatingWriter(config.diagnosticsDir, diagConfig)
	var sink diagnostics.Sink
	if err == nil {
		sink = writer
	} else {
		fmt.Fprintln(os.Stderr, "Diagnostic file logging unavailable; using memory only.")
	}
	events, err := diagnostics.New(diagConfig, diagnostics.Identity{Version: "standalone"}, sink)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not initialize diagnostics.")
		os.Exit(1)
	}
	events.Emit(diagnostics.Input{Event: diagnostics.AppStarting})
	runtime, err := desktopruntime.New(desktopruntime.Config{Addr: config.addr, SQLitePath: config.sqlitePath, Events: events, TrustedProxies: config.trustedProxies, SameOriginProxy: true, LANAccess: config.lanAccess, Backups: config.backups})
	if err != nil {
		fmt.Fprintln(os.Stderr, "API startup failed.")
		_ = events.Close(context.Background())
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", runtime.Server.Addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "API bind failed.")
		_ = runtime.Shutdown(context.Background())
		_ = events.Close(context.Background())
		os.Exit(1)
	}
	events.Emit(diagnostics.Input{Event: diagnostics.AppReady})
	serveErr := make(chan error, 1)
	go func() { serveErr <- runtime.Server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-signals:
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "API server stopped unexpectedly.")
		}
	}
	events.Emit(diagnostics.Input{Event: diagnostics.AppStopping})
	ctx, cancel := context.WithTimeout(context.Background(), config.shutdownTimeout)
	if err := runtime.Shutdown(ctx); err != nil {
		events.Emit(diagnostics.Input{Event: diagnostics.ShutdownFailed, ErrorCode: diagnostics.CodeTimeout})
		fmt.Fprintln(os.Stderr, "API shutdown did not complete cleanly.")
	} else {
		events.Emit(diagnostics.Input{Event: diagnostics.AppStopped})
	}
	cancel()
	_ = events.Close(context.Background())
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
