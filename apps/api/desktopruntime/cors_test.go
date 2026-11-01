package desktopruntime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSOrigins(t *testing.T) {
	tests := []struct {
		name, origin, override, method string
		want                           int
	}{
		{"Linux native window", "wails://wails", "", http.MethodGet, http.StatusOK},
		{"Linux preflight", "wails://wails", "", http.MethodOptions, http.StatusNoContent},
		{"Linux lookalike", "wails://wails.evil.test", "", http.MethodGet, http.StatusForbidden},
		{"Wails browser", "http://localhost:34115", "", http.MethodGet, http.StatusOK},
		{"Wails loopback", "http://127.0.0.1:34115", "", http.MethodGet, http.StatusOK},
		{"Wails preflight", "http://localhost:34115", "", http.MethodOptions, http.StatusNoContent},
		{"Vite", "http://localhost:5173", "", http.MethodGet, http.StatusOK},
		{"packaged app", "http://wails.localhost", "", http.MethodGet, http.StatusOK},
		{"external origin", "https://example.com", "", http.MethodGet, http.StatusForbidden},
		{"lookalike origin", "http://localhost.evil.test:34115", "", http.MethodGet, http.StatusForbidden},
		{"other local port", "http://localhost:12345", "", http.MethodGet, http.StatusForbidden},
		{"custom origin", "http://localhost:12345", "http://localhost:12345", http.MethodGet, http.StatusOK},
		{"override replaces defaults", "http://localhost:34115", "http://localhost:12345", http.MethodGet, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ALLOWED_ORIGINS", tt.override)
			router := gin.New()
			router.Use(corsMiddleware())
			router.GET("/app-config", func(c *gin.Context) { c.Status(http.StatusOK) })
			req := httptest.NewRequest(tt.method, "/app-config", nil)
			req.Header.Set("Origin", tt.origin)
			if tt.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", http.MethodPut)
				req.Header.Set("Access-Control-Request-Headers", "Content-Type")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
			allowed := response.Header().Get("Access-Control-Allow-Origin")
			if tt.want == http.StatusForbidden {
				if allowed != "" {
					t.Fatalf("blocked origin was allowed: %q", allowed)
				}
			} else if allowed != tt.origin {
				t.Fatalf("allow origin = %q, want %q", allowed, tt.origin)
			}
		})
	}
}
