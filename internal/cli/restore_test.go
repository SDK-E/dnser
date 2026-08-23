package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/SDK-E/dnser/internal/setup"
)

func TestRestoreDNSIfAppliedClearsHijackedResolver(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DNSER_HOME", dir)
	r := setup.NewDryRunner()

	state := setup.State{
		DNSApplied:  true,
		DNSServices: map[string][]string{"Wi-Fi": {"192.168.1.1"}},
	}
	if err := setup.SaveState(dir, &state); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	restoreDNSIfApplied(&out)

	got, err := setup.LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.DNSApplied {
		t.Fatal("DNSApplied must be cleared after restore")
	}
	if len(got.DNSServices) != 0 {
		t.Fatal("DNSServices must be cleared after restore")
	}
	cmds := r.Commands()
	_ = cmds
	if !bytes.Contains(out.Bytes(), []byte("restored")) {
		t.Fatalf("expected confirmation, got %q", out.String())
	}
}

func TestRestoreDNSIfAppliedNoopWhenNotApplied(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DNSER_HOME", dir)
	if err := setup.SaveState(dir, &setup.State{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	restoreDNSIfApplied(&out)
	if len(out.Bytes()) != 0 {
		t.Fatalf("expected silence, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "setup-state.json")); err != nil {
		t.Fatal(err)
	}
}
