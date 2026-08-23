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
	st.CAInstallPath = "/Library/Application Support/DNSer/dnser-ca.pem"
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

func TestConfigureDNSDarwinSavesPreviousServers(t *testing.T) {
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

func TestTrustCAWritesAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	r := NewDryRunner()
	path, err := TrustCA(r, []byte("FAKE-PEM"), dir)
	if err != nil {
		t.Fatalf("trust CA: %v", err)
	}
	if path != "/Library/Application Support/DNSer/dnser-ca.pem" {
		t.Errorf("install path = %q", path)
	}
	joined := strings.Join(r.Commands(), "\n")
	if !strings.Contains(joined, "add-trusted-cert") {
		t.Errorf("missing add-trusted-cert in:\n%s", joined)
	}
	if !strings.Contains(joined, "with administrator privileges") {
		t.Errorf("admin elevation missing:\n%s", joined)
	}
}

func TestUntrustCARevertsExactly(t *testing.T) {
	r := NewDryRunner()
	err := UntrustCA(r, "/Library/Application Support/DNSer/dnser-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Commands(), "\n")
	if !strings.Contains(joined, "remove-trusted-cert") || !strings.Contains(joined, "rm -f") {
		t.Errorf("untrust commands incomplete:\n%s", joined)
	}
}
