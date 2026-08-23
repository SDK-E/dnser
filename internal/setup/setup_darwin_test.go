//go:build darwin

package setup

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureDNSSavesPreviousServers(t *testing.T) {
	r := NewDryRunner()
	r.output = map[string]string{
		"networksetup -listallnetworkservices":        "An asterisk (*) denotes that a network service is disabled.\nWi-Fi\nUSB 10/100/1000 LAN\n*Disabled Item\n",
		"networksetup -getdnsservers Wi-Fi":           "9.9.9.9\n1.1.1.1\n",
		"networksetup -getdnsservers USB 10/100/1000": "There aren't any DNS Servers set on USB 10/100/1000.\n",
	}
	saved, err := ConfigureDNS(r, "127.0.0.1")
	if err != nil {
		t.Fatalf("configure: %v", err)
	}

	if got := saved["Wi-Fi"]; len(got) != 2 || got[0] != "9.9.9.9" {
		t.Errorf("saved Wi-Fi = %v, want [9.9.9.9 1.1.1.1]", got)
	}
	if got := saved["USB 10/100/1000"]; len(got) != 0 {
		t.Errorf("saved USB = %v, want empty", got)
	}

	setCalls := 0
	for _, c := range r.Commands() {
		if strings.Contains(c, "-setdnsservers") {
			setCalls++
			if !strings.Contains(c, "127.0.0.1") {
				t.Errorf("set call missing bind IP: %s", c)
			}
		}
	}
	if setCalls != 2 {
		t.Errorf("setdnsservers calls = %d, want 2", setCalls)
	}

	if err := RestoreDNS(r, saved); err != nil {
		t.Fatalf("restore: %v", err)
	}
	foundRestoreEmpty, foundRestorePrev := false, false
	for _, c := range r.Commands() {
		if strings.Contains(c, "-setdnsservers Wi-Fi 9.9.9.9 1.1.1.1") {
			foundRestorePrev = true
		}
		if strings.Contains(c, "-setdnsservers USB") && strings.Contains(c, "Empty") {
			foundRestoreEmpty = true
		}
	}
	if !foundRestorePrev || !foundRestoreEmpty {
		t.Errorf("restore incomplete: prev=%v empty=%v\n%v", foundRestorePrev, foundRestoreEmpty, r.Commands())
	}
}

func TestTrustCAPrefersSilentUserDomain(t *testing.T) {
	dir := t.TempDir()
	r := NewDryRunner()
	path, mode, err := TrustCA(r, []byte("FAKE-PEM"), dir)
	if err != nil {
		t.Fatalf("trust CA: %v", err)
	}
	if mode != TrustModeUser {
		t.Errorf("mode = %q, want user (silent first)", mode)
	}
	wantPath := filepath.Join(dir, "certs", "dnser-ca.pem")
	if path != wantPath {
		t.Errorf("install path = %q, want %q", path, wantPath)
	}
	joined := strings.Join(r.Commands(), "\n")
	if !strings.Contains(joined, "add-trusted-cert -r trustRoot") {
		t.Errorf("missing user-domain trust command in:\n%s", joined)
	}
	if strings.Contains(joined, "administrator privileges") {
		t.Errorf("admin prompt should not be needed for user trust:\n%s", joined)
	}
}

func TestTrustCAFallsBackToAdmin(t *testing.T) {
	r := NewDryRunner()
	r.failOn["/bin/zsh"] = fmt.Errorf("user trust denied")
	dir := t.TempDir()
	_, mode, err := TrustCA(r, []byte("FAKE-PEM"), dir)
	if err != nil {
		t.Fatalf("trust CA fallback: %v", err)
	}
	if mode != TrustModeAdmin {
		t.Errorf("mode = %q, want admin after user failure", mode)
	}
	if joined := strings.Join(r.Commands(), "\n"); !strings.Contains(joined, "-d -r trustRoot") {
		t.Errorf("admin trust command missing:\n%s", joined)
	}
}

func TestUntrustCARevertsExactly(t *testing.T) {
	r := NewDryRunner()
	if err := UntrustCA(r, "/Library/Application Support/DNSer/dnser-ca.pem", TrustModeAdmin); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Commands(), "\n")
	if !strings.Contains(joined, "remove-trusted-cert") || !strings.Contains(joined, "rm -f") {
		t.Errorf("untrust commands incomplete:\n%s", joined)
	}

	r2 := NewDryRunner()
	if err := UntrustCA(r2, "/Library/Application Support/DNSer/dnser-ca.pem", TrustModeUser); err != nil {
		t.Fatal(err)
	}
	joined2 := strings.Join(r2.Commands(), "\n")
	if !strings.Contains(joined2, "remove-trusted-cert") || strings.Contains(joined2, "administrator privileges") {
		t.Errorf("user untrust must be silent and complete:\n%s", joined2)
	}
}
