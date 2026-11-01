package desktopruntime

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// LANAccess trusts only the authenticated reverse proxy, never browser-supplied
// identity headers. The secret must be overwritten by the proxy after authentication.
type LANAccess struct {
	Origin      string
	ProxySecret string
}

func (a *LANAccess) validate(config Config) error {
	u, err := url.Parse(a.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("LAN origin must be an exact HTTPS origin without a path")
	}
	if decoded, err := hex.DecodeString(a.ProxySecret); err != nil || len(decoded) != 32 {
		return fmt.Errorf("LAN proxy secret must be a 64-character hexadecimal random key")
	}
	if !config.SameOriginProxy || len(config.TrustedProxies) == 0 {
		return fmt.Errorf("LAN access requires same-origin mode and explicit trusted proxies")
	}
	addr := config.Addr
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(addr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("LAN API must bind to loopback")
	}
	return nil
}

func (a *LANAccess) middleware(proxies []string) gin.HandlerFunc {
	u, _ := url.Parse(a.Origin)
	expected := sha256.Sum256([]byte(a.ProxySecret))
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		actual := sha256.Sum256([]byte(c.GetHeader("X-Maker-Proxy-Key")))
		if !trustedProxyPeer(c.Request.RemoteAddr, proxies) || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}
		if !strings.EqualFold(c.Request.Host, u.Host) || c.GetHeader("X-Forwarded-Proto") != "https" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Request host or transport is not allowed"})
			return
		}
		origin := c.GetHeader("Origin")
		safe := c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead
		if (origin != "" && origin != a.Origin) || (!safe && origin == "") || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Request origin is not allowed"})
			return
		}
		// Global cap includes chunked requests. Existing import handlers retain their
		// smaller format-specific limits.
		const maxBody = 64 << 20
		if c.Request.ContentLength > maxBody {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body is too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		c.Next()
	}
}
