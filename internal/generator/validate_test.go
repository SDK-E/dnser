package generator

import (
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/config"
)

func mustDecode(t *testing.T, src string) *config.Manifest {
	t.Helper()
	m, err := config.Decode([]byte(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

func TestValidateReservedCaddyKeys(t *testing.T) {
	m := mustDecode(t, "domain: a.test\nport: 3000\ncaddy:\n  tls: internal_policy_override\n")
	err := validateForGenerate(m, []string{"a.test"}, "a.test")
	if err == nil || !strings.Contains(err.Error(), "reverse_proxy") {
		t.Fatalf("reserved key tls must be rejected with pointer to list: %v", err)
	}
}

func TestValidateConflictingProcessKeys(t *testing.T) {
	m := mustDecode(t, "domain: a.test\ncommand: npm dev\nprocess:\n  command: other\n")
	err := validateForGenerate(m, []string{"a.test"}, "a.test")
	if err == nil || !strings.Contains(err.Error(), "process.command") {
		t.Fatalf("typed conflict must be rejected: %v", err)
	}
	m2 := mustDecode(t, "domain: a.test\ncommand: npm dev\nprocess:\n  availability:\n    restart: always\n  shutdown:\n    timeout_seconds: 15\n")
	if err := validateForGenerate(m2, []string{"a.test"}, "a.test"); err != nil {
		t.Fatalf("delegated process.availability.restart must be allowed: %v", err)
	}
}

func TestValidateRecordsUnderDeclaredDomains(t *testing.T) {
	ok := mustDecode(t, "domains: [app.acme.io]\nrecords:\n  - {name: \"@\", type: TXT, value: v=1}\n  - {name: \"*.app.acme.io\", type: A, value: 127.0.0.1}\n")
	if err := validateForGenerate(ok, ok.EffectiveNames(), "app.acme.io"); err != nil {
		t.Fatalf("in-domain records valid: %v", err)
	}
	bad := mustDecode(t, "domains: [app.acme.io]\nrecords:\n  - {name: evil.example.com, type: TXT, value: nope}\n")
	err := validateForGenerate(bad, bad.EffectiveNames(), "app.acme.io")
	if err == nil || !strings.Contains(err.Error(), "outside every declared domain") {
		t.Fatalf("out-of-domain record must fail: %v", err)
	}
}

func TestValidateRoutesShape(t *testing.T) {
	bad := mustDecode(t, "domain: a.test\nport: 3000\nroutes:\n  - path: /api/*\n")
	err := validateForGenerate(bad, []string{"a.test"}, "a.test")
	if err == nil || !strings.Contains(err.Error(), "exactly one of port or backend") {
		t.Fatalf("port-less route must fail: %v", err)
	}
	badHost := mustDecode(t, "domain: a.test\nport: 3000\nroutes:\n  - host: other.example.com\n    backend: 127.0.0.1:9\n")
	err = validateForGenerate(badHost, []string{"a.test"}, "a.test")
	if err == nil || !strings.Contains(err.Error(), "outside every declared domain") {
		t.Fatalf("host route outside declared names must fail: %v", err)
	}
	goodHost := mustDecode(t, "domain: a.test\nport: 3000\nroutes:\n  - host: admin.a.test\n    backend: 127.0.0.1:4001\n")
	if err := validateForGenerate(goodHost, []string{"a.test"}, "a.test"); err != nil {
		t.Fatalf("subdomain host route valid: %v", err)
	}
}

func TestValidateForceHTTPSContradiction(t *testing.T) {
	m := mustDecode(t, "domain: a.test\nforce_https: true\nhttps:\n  a.test: false\n")
	err := validateForGenerate(m, []string{"a.test"}, "a.test")
	if err == nil || !strings.Contains(err.Error(), "contradicts") {
		t.Fatalf("contradiction must be caught: %v", err)
	}
}
