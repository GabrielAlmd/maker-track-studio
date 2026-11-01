package desktopruntime

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLANBoundary(t *testing.T) {
	key := strings.Repeat("a", 64)
	runtime, err := New(Config{SQLitePath: filepath.Join(t.TempDir(), "lan.db"), SameOriginProxy: true, TrustedProxies: []string{"127.0.0.1"}, LANAccess: &LANAccess{Origin: "https://studio.home.arpa", ProxySecret: key}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(context.Background())
	for _, path := range []string{"/health/live", "/health/ready", "/printers", "/desktop-data/export", "/reports/export", "/desktop-data/import", "/data-migration/import", "/sales-orders", "/models/id"} {
		for _, tc := range []struct {
			name, method, secret, host, origin, peer, proto string
			status                                          int
		}{
			{"unauthenticated", "GET", "", "studio.home.arpa", "", "127.0.0.1:1234", "https", 401},
			{"invalid key", "POST", "wrong", "studio.home.arpa", "https://studio.home.arpa", "127.0.0.1:1234", "https", 401},
			{"direct LAN", "GET", key, "studio.home.arpa", "", "192.168.1.2:1234", "https", 401},
			{"bad host", "GET", key, "evil.invalid", "", "127.0.0.1:1234", "https", 403},
			{"missing host", "GET", key, "", "", "127.0.0.1:1234", "https", 403},
			{"http transport", "GET", key, "studio.home.arpa", "", "127.0.0.1:1234", "http", 403},
			{"missing origin", "POST", key, "studio.home.arpa", "", "127.0.0.1:1234", "https", 403},
			{"cross origin", "DELETE", key, "studio.home.arpa", "https://evil.invalid", "127.0.0.1:1234", "https", 403},
			{"null origin", "PUT", key, "studio.home.arpa", "null", "127.0.0.1:1234", "https", 403},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(tc.method, "http://studio.home.arpa"+path, nil)
				req.Host = tc.host
				req.RemoteAddr = tc.peer
				req.Header.Set("X-Maker-Proxy-Key", tc.secret)
				req.Header.Set("X-Forwarded-Proto", tc.proto)
				req.Header.Set("Origin", tc.origin)
				res := httptest.NewRecorder()
				runtime.Server.Handler.ServeHTTP(res, req)
				if res.Code != tc.status {
					t.Fatalf("got %d want %d: %s", res.Code, tc.status, res.Body.String())
				}
				if res.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("missing security header")
				}
			})
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/health/ready", "", 200},
		{"GET", "/printers", "", 200},
		{"GET", "/desktop-data/export", "", 200},
		{"POST", "/printers", `{"name":"LAN printer","manufacturer":"Test","model":"A1"}`, 201},
	} {
		req := httptest.NewRequest(tc.method, "http://studio.home.arpa"+tc.path, strings.NewReader(tc.body))
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Maker-Proxy-Key", key)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("Origin", "https://studio.home.arpa")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		runtime.Server.Handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, res.Code, res.Body.String())
		}
	}
}

func TestLANConfigurationFailsClosed(t *testing.T) {
	for _, tc := range []struct{ origin, key, addr string }{
		{"http://studio.home.arpa", strings.Repeat("a", 64), "127.0.0.1:8080"},
		{"https://studio.home.arpa/path", strings.Repeat("a", 64), "127.0.0.1:8080"},
		{"https://studio.home.arpa", "short", "127.0.0.1:8080"},
		{"https://studio.home.arpa", strings.Repeat("a", 64), "0.0.0.0:8080"},
	} {
		_, err := New(Config{Addr: tc.addr, SameOriginProxy: true, TrustedProxies: []string{"127.0.0.1"}, LANAccess: &LANAccess{Origin: tc.origin, ProxySecret: tc.key}})
		if err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestLANRequestLimitsAndMetadata(t *testing.T) {
	key := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, site string
		size       int64
		status     int
	}{
		{"cross-site read", "cross-site", 0, 403},
		{"oversized body", "same-origin", (64 << 20) + 1, 413},
		{"authorized", "same-origin", 0, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			access := &LANAccess{Origin: "https://studio.home.arpa", ProxySecret: key}
			router.Use(access.middleware([]string{"127.0.0.1"}))
			router.GET("/", func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest("GET", "https://studio.home.arpa/", nil)
			req.RemoteAddr = "127.0.0.1:1"
			req.Header.Set("X-Maker-Proxy-Key", key)
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("Sec-Fetch-Site", tc.site)
			req.ContentLength = tc.size
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("got %d want %d", res.Code, tc.status)
			}
		})
	}
}
