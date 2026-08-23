package setup

import (
	"os"
	"path/filepath"
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
