package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/desktopruntime"
)

func TestLoadConfigDefaultsToLoopbackAndSeparateDatabase(t *testing.T) {
	configureSecurity(t)
	t.Setenv("API_BIND_ADDRESS", "")
	t.Setenv("API_PORT", "")
	t.Setenv("API_SQLITE_PATH", "")
	t.Setenv("API_DIAGNOSTICS_DIR", t.TempDir())
	t.Setenv("API_DIAGNOSTICS_LEVEL", "")
	t.Setenv("API_SHUTDOWN_TIMEOUT_SECONDS", "")
	t.Setenv("API_TRUSTED_PROXIES", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != "127.0.0.1:8080" {
		t.Fatalf("unsafe default bind address: %s", cfg.addr)
	}
	if filepath.Base(cfg.sqlitePath) != "server.db" || cfg.shutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadConfigRejectsNonLoopbackAndInvalidValues(t *testing.T) {
	configureSecurity(t)
	t.Setenv("API_BIND_ADDRESS", "0.0.0.0")
	if _, err := loadConfig(); err == nil {
		t.Fatal("non-loopback bind accepted")
	}
	t.Setenv("API_BIND_ADDRESS", "127.0.0.1")
	t.Setenv("API_PORT", "70000")
	if _, err := loadConfig(); err == nil {
		t.Fatal("invalid port accepted")
	}
	t.Setenv("API_PORT", "8080")
	t.Setenv("API_DIAGNOSTICS_DIR", t.TempDir())
	t.Setenv("API_TRUSTED_PROXIES", "not-an-ip")
	if _, err := loadConfig(); err == nil {
		t.Fatal("invalid trusted proxy accepted")
	}
}

func TestStandaloneRuntimeSmokeReadinessCRUDAndShutdown(t *testing.T) {
	runtime, err := desktopruntime.New(desktopruntime.Config{Addr: "127.0.0.1:0", SQLitePath: filepath.Join(t.TempDir(), "api.db"), SameOriginProxy: true, TrustedProxies: []string{"127.0.0.1"}, LANAccess: &desktopruntime.LANAccess{Origin: "https://studio.example.test", ProxySecret: strings.Repeat("a", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", runtime.Server.Addr)
	if err != nil {
		_ = runtime.Shutdown(context.Background())
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- runtime.Server.Serve(listener) }()
	base := "http://" + listener.Addr().String()
	direct, err := http.Get(base + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	direct.Body.Close()
	if direct.StatusCode != http.StatusUnauthorized {
		t.Fatalf("direct API status=%d", direct.StatusCode)
	}
	client := &http.Client{Transport: proxyTransport{}}

	for _, path := range []string{"/health/live", "/health/ready", "/printers", "/reports/export"} {
		resp, err := client.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("GET %s returned %d", path, resp.StatusCode)
		}
	}
	createReq, err := http.NewRequest(http.MethodPost, base+"/printers", strings.NewReader(`{"name":"Test printer","manufacturer":"Test","model":"A1"}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Origin", "https://studio.example.test")
	createResp, err := client.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		createResp.Body.Close()
		t.Fatal(err)
	}
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("printer create status=%d id=%q", createResp.StatusCode, created.ID)
	}
	getResp, err := client.Get(base + "/printers/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("printer read status=%d", getResp.StatusCode)
	}
	forgedReq, err := http.NewRequest(http.MethodGet, base+"/printers", nil)
	if err != nil {
		t.Fatal(err)
	}
	forgedReq.Header.Set("Origin", "https://attacker.invalid")
	forgedResp, err := client.Do(forgedReq)
	if err != nil {
		t.Fatal(err)
	}
	forgedResp.Body.Close()
	if forgedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin status=%d", forgedResp.StatusCode)
	}
	proxyReq, err := http.NewRequest(http.MethodGet, base+"/printers", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyReq.Host = "studio.example.test"
	proxyReq.Header.Set("Origin", "https://studio.example.test")
	proxyReq.Header.Set("X-Forwarded-Proto", "https")
	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		t.Fatal(err)
	}
	proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Fatalf("same-origin TLS proxy status=%d", proxyResp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-serveDone; err != http.ErrServerClosed {
		t.Fatalf("serve result = %v", err)
	}
}

func TestStandaloneBindFailureClosesDatabase(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := desktopruntime.New(desktopruntime.Config{Addr: occupied.Addr().String(), SQLitePath: filepath.Join(t.TempDir(), "api.db")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := net.Listen("tcp", runtime.Server.Addr); err == nil {
		t.Fatal("expected bind conflict")
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = occupied.Close()
}

func TestStandaloneDatabaseInitializationFailure(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "not-a-directory"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := desktopruntime.New(desktopruntime.Config{SQLitePath: filepath.Join(directory, "not-a-directory", "db.sqlite")}); err == nil {
		t.Fatal("database initialization unexpectedly succeeded")
	}
}

func configureSecurity(t *testing.T) {
	t.Helper()
	secret := filepath.Join(t.TempDir(), "proxy-key")
	if err := os.WriteFile(secret, []byte(strings.Repeat("a", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("API_PROXY_SECRET_FILE", secret)
	t.Setenv("API_PUBLIC_ORIGIN", "https://studio.home.arpa")
}

func TestLoadConfigRequiresProtectedProxySecret(t *testing.T) {
	configureSecurity(t)
	t.Setenv("API_PROXY_SECRET_FILE", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("missing secret accepted")
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 64)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("API_PROXY_SECRET_FILE", path)
	if _, err := loadConfig(); err == nil {
		t.Fatal("world-readable secret accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(); err == nil {
		t.Fatal("short secret accepted")
	}
}

// proxyTransport models the headers added by the authenticated TLS proxy.
type proxyTransport struct{}

func (proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Host = "studio.example.test"
	cloned.Header.Set("X-Maker-Proxy-Key", strings.Repeat("a", 64))
	cloned.Header.Set("X-Forwarded-Proto", "https")
	return http.DefaultTransport.RoundTrip(cloned)
}

func TestLoadConfigScheduledBackupsOptInAndValidation(t *testing.T) {
	configureSecurity(t)
	t.Setenv("API_DIAGNOSTICS_DIR", t.TempDir())
	for _, key := range []string{"API_BACKUP_DIR", "API_BACKUP_RETENTION", "API_BACKUP_TIME_UTC"} {
		t.Setenv(key, "")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.backups != nil {
		t.Fatalf("backups enabled by default: %v", err)
	}
	t.Setenv("API_BACKUP_DIR", t.TempDir())
	cfg, err = loadConfig()
	if err != nil || cfg.backups == nil || cfg.backups.TimeUTC != "03:00" || cfg.backups.Retention != 14 {
		t.Fatalf("backup defaults: %#v %v", cfg.backups, err)
	}
	for _, clock := range []string{"25:00", "3:00", "03:00:00"} {
		t.Setenv("API_BACKUP_TIME_UTC", clock)
		if _, err := loadConfig(); err == nil {
			t.Fatalf("accepted time %q", clock)
		}
	}
	t.Setenv("API_BACKUP_TIME_UTC", "03:00")
	for _, retention := range []string{"0", "-1", "3651", "bad"} {
		t.Setenv("API_BACKUP_RETENTION", retention)
		if _, err := loadConfig(); err == nil {
			t.Fatalf("accepted retention %q", retention)
		}
	}
	t.Setenv("API_BACKUP_RETENTION", "14")
	t.Setenv("API_BACKUP_DIR", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("silently ignored schedule without destination")
	}
}
