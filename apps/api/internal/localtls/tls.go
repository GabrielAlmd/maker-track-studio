// Package localtls manages the persistent private CA for self-hosted HTTPS.
package localtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func serial() (*big.Int, error) { return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128)) }

func write(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tls-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func issue(template, parent *x509.Certificate, signer *ecdsa.PrivateKey) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	if parent == nil {
		parent = template
		signer = key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		return nil, nil, err
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), nil
}

// Ensure preserves the CA and returns whether a new server certificate was issued.
// The caller must serialize calls and reload its TLS listener after a change.
func Ensure(dir, host string, now time.Time) (bool, error) {
	if host == "" {
		return false, fmt.Errorf("empty TLS hostname")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, err
	}
	certPath, keyPath := filepath.Join(dir, "root.crt"), filepath.Join(dir, "root.key")
	_, ce := os.Stat(certPath)
	_, ke := os.Stat(keyPath)
	if os.IsNotExist(ce) && os.IsNotExist(ke) {
		n, err := serial()
		if err != nil {
			return false, err
		}
		cert, key, err := issue(&x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: "Maker Track Local CA"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}, nil, nil)
		if err != nil {
			return false, err
		}
		if err = write(keyPath, key); err != nil {
			return false, err
		}
		if err = write(certPath, cert); err != nil {
			return false, err
		}
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return false, fmt.Errorf("load existing CA (manual recovery required): %w", err)
	}
	ca, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false, err
	}
	signer, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok || !ca.IsCA || ca.CheckSignatureFrom(ca) != nil || now.Before(ca.NotBefore) || !now.Add(90*24*time.Hour).Before(ca.NotAfter) {
		return false, fmt.Errorf("CA invalid or near expiry; manual CA replacement required")
	}
	// A combined PEM allows atomic replacement of the server certificate and key.
	server := filepath.Join(dir, "server.pem")
	if current, e := tls.LoadX509KeyPair(server, server); e == nil {
		leaf, e := x509.ParseCertificate(current.Certificate[0])
		if e == nil && leaf.CheckSignatureFrom(ca) == nil && leaf.VerifyHostname(host) == nil && !now.Before(leaf.NotBefore) && leaf.NotAfter.After(now.Add(30*24*time.Hour)) {
			return false, nil
		}
	} else if !os.IsNotExist(e) {
		return false, fmt.Errorf("load existing server certificate: %w", e)
	}
	n, err := serial()
	if err != nil {
		return false, err
	}
	t := &x509.Certificate{SerialNumber: n, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(host); ip != nil {
		t.IPAddresses = []net.IP{ip}
	} else {
		t.DNSNames = []string{host}
	}
	cert, key, err := issue(t, ca, signer)
	if err != nil {
		return false, err
	}
	return true, write(server, append(cert, key...))
}
