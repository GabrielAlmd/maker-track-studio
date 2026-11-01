package localtls

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenewalPreservesCA(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	changed, err := Ensure(dir, "maker.local", now)
	if err != nil || !changed {
		t.Fatalf("initial: %v %v", changed, err)
	}
	root, _ := os.ReadFile(filepath.Join(dir, "root.crt"))
	changed, err = Ensure(dir, "maker.local", now.Add(24*time.Hour))
	if err != nil || changed {
		t.Fatalf("reuse: %v %v", changed, err)
	}
	changed, err = Ensure(dir, "maker.local", now.Add(61*24*time.Hour))
	if err != nil || !changed {
		t.Fatalf("renew: %v %v", changed, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "root.crt"))
	if string(root) != string(after) {
		t.Fatal("CA changed")
	}
	changed, err = Ensure(dir, "192.168.1.2", now.Add(62*24*time.Hour))
	if err != nil || !changed {
		t.Fatalf("host change: %v %v", changed, err)
	}
}
func TestIncompleteCAFailsClosed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "root.key"), []byte("broken"), 0600)
	if _, err := Ensure(dir, "maker.local", time.Now()); err == nil {
		t.Fatal("accepted incomplete CA")
	}
	if _, err := os.Stat(filepath.Join(dir, "root.crt")); !os.IsNotExist(err) {
		t.Fatal("regenerated CA")
	}
}

func TestCertificateTrustAndPermissions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if _, err := Ensure(dir, "192.168.1.2", now); err != nil {
		t.Fatal(err)
	}
	root, err := os.ReadFile(filepath.Join(dir, "root.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(root)
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.pem"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "192.168.1.2", CurrentTime: now}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.key", "server.pem"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe permissions for %s", name)
		}
	}
	if _, err = Ensure(dir, "192.168.1.2", now.AddDate(10, 0, 0)); err == nil {
		t.Fatal("accepted expired CA")
	}
}
