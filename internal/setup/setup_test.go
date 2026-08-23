package setup

import (
	"os"
	"strings"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := LoadState(dir)
	if err != nil {
		t.Fatalf("load empty state: %v", err)
	}
	if st.DNSApplied || st.CATrusted {
		t.Errorf("fresh state should be clean: %+v", st)
	}

	st.DNSServices = map[string][]string{"Wi-Fi": {"9.9.9.9", "1.1.1.1"}}
	st.DNSApplied = true
	st.CATrusted = true
	if err := SaveState(dir, st); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.DNSApplied || reloaded.DNSServices["Wi-Fi"][0] != "9.9.9.9" {
		t.Errorf("state mismatch after reload: %+v", reloaded)
	}

	if err := ClearState(dir); err != nil {
		t.Fatalf("clear: %v", err)
	}
	st, _ = LoadState(dir)
	if st.DNSApplied {
		t.Error("state not cleared")
	}
	if err := ClearState(dir); err != nil {
		t.Errorf("clear must tolerate missing file: %v", err)
	}
}

func TestSaveStateIsAtomic(t *testing.T) {
	dir := t.TempDir()
	st := &State{DNSApplied: true}
	if err := SaveState(dir, st); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".setup-state-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestPlatformAvailable(t *testing.T) {
	info := Platform()
	if info.Name == "" {
		t.Error("platform name must not be empty")
	}
	r := SystemRunner()
	out, err := r.CombinedOutput("echo", "ok")
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Errorf("system runner broken: out=%q err=%v", out, err)
	}
}

func TestUsableUpstreamsFiltersLoopbackAndDedups(t *testing.T) {
	got := usableUpstreams([]string{"127.0.0.1", "192.168.1.1", "", "192.168.1.1", "::1", "9.9.9.9"})
	want := []string{"192.168.1.1", "9.9.9.9"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
