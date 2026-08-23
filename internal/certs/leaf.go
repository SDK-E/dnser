package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const leafValidity = 825 * 24 * time.Hour

type Manager struct {
	mu     sync.Mutex
	ca     *CA
	leaves map[string]*tls.Certificate
}

func NewManager(ca *CA) *Manager {
	return &Manager{ca: ca, leaves: make(map[string]*tls.Certificate)}
}

func (m *Manager) CA() *CA {
	return m.ca
}

func (m *Manager) Leaf(domain string) (*tls.Certificate, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if domain == "" {
		return nil, fmt.Errorf("empty domain for leaf certificate")
	}
	m.mu.Lock()
	cached, ok := m.leaves[domain]
	m.mu.Unlock()
	if ok && valid(cached) {
		return cached, nil
	}
	names := leafNames(domain)
	cert, err := issueLeaf(m.ca, names)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.leaves[domain] = cert
	m.mu.Unlock()
	return cert, nil
}

func leafNames(domain string) []string {
	var names []string
	if strings.HasPrefix(domain, "*.") || domain == "localhost" || !strings.Contains(domain, ".") {
		names = append(names, domain)
	} else {
		names = append(names, domain, "*."+domain, "localhost")
	}
	seen := make(map[string]bool, len(names))
	var out []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func valid(c *tls.Certificate) bool {
	if c == nil || len(c.Certificate) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return false
	}
	return time.Now().Before(cert.NotAfter.Add(-30 * 24 * time.Hour))
}

func issueLeaf(ca *CA, dnsNames []string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"DNSer Local"},
			CommonName:   dnsNames[0],
		},
		DNSNames:    dnsNames,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(leafValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign leaf certificate: %w", err)
	}
	pair := &tls.Certificate{
		Certificate: [][]byte{der, ca.cert.Raw},
		PrivateKey:  key,
	}
	return pair, nil
}

func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := hello.ServerName
			if name == "" {
				name = "localhost"
			}
			return m.Leaf(name)
		},
		NextProtos: []string{"h2", "http/1.1"},
	}
}
