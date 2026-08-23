package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/etc/resolver/test": `'/etc/resolver/test'`,
		"it's":               `'it'\''s'`,
		"":                   "''",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteResolverDomainCommand(t *testing.T) {
	r := NewDryRunner("sudo")
	if err := WriteResolverDomain(r, "test", "127.0.0.1", 35410); err != nil {
		t.Fatalf("write resolver domain: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	if !strings.Contains(full, "osascript") {
		t.Fatalf("expected osascript elevation when sudo unavailable:\n%s", full)
	}
	for _, want := range []string{"mkdir -p /etc/resolver", "nameserver 127.0.0.1", "port 35410", "'/etc/resolver/test'"} {
		if !strings.Contains(full, want) {
			t.Errorf("command missing %q:\n%s", want, full)
		}
	}
}

func TestWriteResolverDomainSudoPath(t *testing.T) {
	r := NewDryRunner()
	if err := WriteResolverDomain(r, "test", "127.0.0.1", 53); err != nil {
		t.Fatalf("write resolver domain: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	if !strings.Contains(full, "sudo") || !strings.Contains(full, "/bin/sh") {
		t.Fatalf("expected sudo path when available:\n%s", full)
	}
}

func TestRemoveResolverFilesCommand(t *testing.T) {
	r := NewDryRunner("sudo")
	if err := RemoveResolverFiles(r, []string{"test", "dev.local"}); err != nil {
		t.Fatalf("remove resolver files: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	for _, want := range []string{`rm -f '/etc/resolver/test'`, `rm -f '/etc/resolver/dev.local'`, "rmdir /etc/resolver"} {
		if !strings.Contains(full, want) {
			t.Errorf("commands missing %q:\n%s", want, full)
		}
	}
}

func TestStateRoundTripWithDesktopFields(t *testing.T) {
	dir := t.TempDir()
	st := &State{
		CATrusted:       true,
		CATrustMode:     TrustModeAdmin,
		CAInstallPath:   "/Library/Application Support/DNSer/dnser-ca.pem",
		ResolverDomains: []string{"test", "localhost"},
		CapGranted:      true,
		DNSApplied:      true,
		DNSServices:     map[string][]string{"Wi-Fi": {"9.9.9.9"}},
	}
	if err := SaveState(dir, st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.CATrustMode != st.CATrustMode || got.CAInstallPath != st.CAInstallPath {
		t.Errorf("CA fields mismatch: %+v", got)
	}
	if len(got.ResolverDomains) != 2 || got.ResolverDomains[0] != "test" {
		t.Errorf("resolver domains mismatch: %v", got.ResolverDomains)
	}
	if !got.CapGranted || !got.DNSApplied || len(got.DNSServices) != 1 {
		t.Errorf("desktop/service fields mismatch: %+v", got)
	}
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "setup-state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(bad); err == nil {
		t.Error("loading corrupt state must fail")
	}
}
