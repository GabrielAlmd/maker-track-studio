package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/internal/localtls"
	"golang.org/x/crypto/bcrypt"
)

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: selfhostctl hash-password|certificates|prepare|renew")
	}
	if os.Args[1] == "hash-password" {
		b, e := io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if e != nil {
			return e
		}
		password := strings.TrimSuffix(string(b), "\n")
		if password == "" {
			return fmt.Errorf("empty password")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
		if e != nil {
			return e
		}
		fmt.Printf("maker:%s\n", hash)
		return nil
	}
	u, e := parseOrigin(os.Getenv("API_PUBLIC_ORIGIN"))
	if e != nil {
		return e
	}
	host := u.Hostname()
	dir := os.Getenv("MAKER_TLS_DIR")
	if dir == "" {
		dir = "/data/tls"
	}
	ensure := func() (bool, error) { return localtls.Ensure(dir, host, time.Now()) }
	switch os.Args[1] {
	case "certificates":
		_, e = ensure()
		return e
	case "prepare":
		if _, e = ensure(); e != nil {
			return e
		}
		key, e := os.ReadFile(os.Getenv("API_PROXY_SECRET_FILE"))
		if e != nil {
			return e
		}
		secret := strings.TrimSpace(string(key))
		if len(secret) != 64 || strings.Trim(secret, "0123456789abcdefABCDEF") != "" {
			return fmt.Errorf("invalid proxy key")
		}
		template, e := os.ReadFile("/etc/nginx/maker-track.conf.template")
		if e != nil {
			return e
		}
		config := strings.NewReplacer("@@HOST@@", host, "@@AUTHORITY@@", u.Host, "@@TLS@@", dir, "@@KEY@@", secret).Replace(string(template))
		if e = os.MkdirAll("/tmp/nginx", 0700); e != nil {
			return e
		}
		return os.WriteFile("/tmp/nginx/nginx.conf", []byte(config), 0600)
	case "renew":
		for {
			time.Sleep(12 * time.Hour)
			changed, e := ensure()
			if e != nil {
				return e
			}
			if changed {
				cmd := exec.Command("nginx", "-e", "stderr", "-c", "/tmp/nginx/nginx.conf", "-s", "reload")
				cmd.Stderr = os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			}
		}
	default:
		return fmt.Errorf("unknown command")
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOrigin(origin string) (*url.URL, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.ForceQuery || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("API_PUBLIC_ORIGIN must be an exact HTTPS origin")
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil {
		for _, c := range host {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
				return nil, fmt.Errorf("invalid hostname")
			}
		}
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port")
		}
	}

	return u, nil
}
