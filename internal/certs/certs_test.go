package certs

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestCACreateAndReload(t *testing.T) {
	dir := t.TempDir()

	ca1, err := NewCA(dir)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	if ca1.Certificate() == nil || len(ca1.CertificatePEM()) == 0 {
		t.Fatal("CA not initialized")
	}
	if !ca1.Certificate().IsCA {
		t.Error("root cert must be CA")
	}
	pem1 := ca1.CertificatePEM()

	ca2, err := NewCA(dir)
	if err != nil {
		t.Fatalf("reload CA: %v", err)
	}
	if string(ca2.CertificatePEM()) != string(pem1) {
		t.Error("CA must be stable across reloads")
	}

	info, err := os.Stat(filepath.Join(dir, caKeyFile))
	if err != nil {
		t.Fatalf("CA key missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("CA key perms = %v, want 0600", info.Mode().Perm())
	}
}

func TestLeafIssuanceAndCache(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(ca)

	leaf1, err := m.Leaf("myproject.test")
	if err != nil {
		t.Fatalf("issue leaf: %v", err)
	}
	leaf2, err := m.Leaf("myproject.test")
	if err != nil {
		t.Fatal(err)
	}
	if leaf1.PrivateKey != leaf2.PrivateKey {
		t.Error("leaf must be cached per domain")
	}

	cert, err := x509.ParseCertificate(leaf1.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	foundWild, foundLocalhost := false, false
	for _, n := range cert.DNSNames {
		switch n {
		case "*.myproject.test":
			foundWild = true
		case "localhost":
			foundLocalhost = true
		}
	}
	if !foundWild || !foundLocalhost {
		t.Errorf("SANs incomplete: %v", cert.DNSNames)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Certificate())
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "api.myproject.test", Roots: pool}); err != nil {
		t.Errorf("wildcard verification failed: %v", err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "myproject.test", Roots: pool}); err != nil {
		t.Errorf("apex verification failed: %v", err)
	}
}

func TestTLSConfigServesCertificate(t *testing.T) {
	dir := t.TempDir()
	ca, _ := NewCA(dir)
	m := NewManager(ca)
	cfg := m.TLSConfig()
	if cfg.GetCertificate == nil {
		t.Fatal("GetCertificate hook required")
	}
	got, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "anything.dev"})
	if err != nil || got == nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
}
