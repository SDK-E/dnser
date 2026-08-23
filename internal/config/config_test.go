package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig() Config {
	cfg := Default()
	return cfg
}

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, out string
		wantErr bool
	}{
		{"MyProject.Test", "myproject.test", false},
		{"myproject.test.", "myproject.test", false},
		{"  API.MyProject.test  ", "api.myproject.test", false},
		{"*.myproject.test", "*.myproject.test", false},
		{"-bad.example.com", "", true},
		{"bad-.example.com", "", true},
		{"a..b", "", true},
		{"under_score.dev", "under_score.dev", false},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeDomain(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeDomain(%q) = %q, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeDomain(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.out {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestEnsureTLD(t *testing.T) {
	cases := []struct {
		in, tld, out string
	}{
		{"myproject", "test", "myproject.test"},
		{"myproject.test", "test", "myproject.test"},
		{"myproject.dev", "test", "myproject.dev.test"},
		{"*.staging.myproject", "test", "*.staging.myproject.test"},
		{"dnser", "test", "dnser.test"},
	}
	for _, c := range cases {
		got, err := EnsureTLD(c.in, c.tld)
		if err != nil {
			t.Errorf("EnsureTLD(%q,%q) error: %v", c.in, c.tld, err)
			continue
		}
		if got != c.out {
			t.Errorf("EnsureTLD(%q,%q) = %q, want %q", c.in, c.tld, got, c.out)
		}
	}
}

func TestValidateRecordTypes(t *testing.T) {
	valid := []Record{
		{Type: "A", Name: "@", Value: "127.0.0.1"},
		{Type: "AAAA", Name: "v6", Value: "::1"},
		{Type: "CNAME", Name: "docs", Value: "example.com"},
		{Type: "TXT", Name: "_verify", Value: "hello world"},
		{Type: "MX", Name: "@", Value: "mail.example.com", Priority: 10},
		{Type: "SRV", Name: "_sip._tcp", Value: "sip.example.com", Priority: 1, Weight: 5, Port: 5060},
		{Type: "NS", Name: "sub", Value: "ns1.example.com"},
	}
	for _, r := range valid {
		if err := ValidateRecord(r); err != nil {
			t.Errorf("ValidateRecord(%+v) unexpected error: %v", r, err)
		}
	}
	invalid := []Record{
		{Type: "A", Name: "@", Value: "::1"},
		{Type: "AAAA", Name: "@", Value: "127.0.0.1"},
		{Type: "CNAME", Name: "@", Value: "*.wild.com"},
		{Type: "TXT", Name: "@", Value: ""},
		{Type: "SRV", Name: "_x._tcp", Value: "host.com"},
		{Type: "BOGUS", Name: "@", Value: "x"},
		{Type: "A", Name: "!", Value: "127.0.0.1"},
	}
	for _, r := range invalid {
		if err := ValidateRecord(r); err == nil {
			t.Errorf("ValidateRecord(%+v) expected error, got nil", r)
		}
	}
}

func writeCfg(t *testing.T, dir string, mutate func(*Config)) string {
	t.Helper()
	path := filepath.Join(dir, "dnser.json")
	cfg := testConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStoreOpenCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "dnser.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if s.Get().Settings.TLD != DefaultTLD {
		t.Errorf("default TLD = %q, want %q", s.Get().Settings.TLD, DefaultTLD)
	}
	if _, err := os.Stat(filepath.Join(dir, "dnser.json")); err != nil {
		t.Fatalf("default config not written: %v", err)
	}
}

func TestStoreUpdatePersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := writeCfg(t, dir, nil)

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = s1.Update(func(c *Config) {
		c.Projects = append(c.Projects, Project{
			Domain: "MyProject.test",
			Routes: []Route{
				{Host: "@", Backends: []string{"localhost:3000"}, HTTPS: true},
				{Host: "*", Backends: []string{"localhost:3000"}, HTTPS: true},
			},
			Records: []Record{{Type: "TXT", Name: "@", Value: "hello"}},
		})
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := s1.FindProject("myproject.test"); !ok {
		t.Fatal("project not found after update")
	}
	raw, _ := os.ReadFile(path)
	var onDisk Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("disk config unmarshal: %v", err)
	}
	if len(onDisk.Projects) != 1 || onDisk.Projects[0].Domain != "myproject.test" {
		t.Errorf("unexpected disk state: %+v", onDisk.Projects)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, ok := s2.FindProject("myproject.test")
	if !ok || len(p.Routes) != 2 || p.Routes[0].Backends[0] != "localhost:3000" || p.Routes[1].Host != "*" || len(p.Records) != 1 {
		t.Errorf("reopened project mismatch: %+v ok=%v", p, ok)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
		t.Errorf("timestamps not maintained: created=%v updated=%v", p.CreatedAt, p.UpdatedAt)
	}
}

func TestStoreRejectsInvalidUpdate(t *testing.T) {
	dir := t.TempDir()
	path := writeCfg(t, dir, nil)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	before := s.Get().Projects
	err = s.Update(func(c *Config) {
		c.Projects = append(c.Projects, Project{Domain: "!!invalid"})
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(s.Get().Projects) != len(before) {
		t.Error("store mutated despite failed update")
	}
}

func TestStoreRejectsNewerVersion(t *testing.T) {
	dir := t.TempDir()
	path := writeCfg(t, dir, func(c *Config) { c.Version = CurrentVersion + 1 })
	if _, err := Open(path); err == nil {
		t.Fatal("expected version error for newer config")
	}
}

func TestStoreExternalChangeReload(t *testing.T) {
	dir := t.TempDir()
	path := writeCfg(t, dir, nil)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ext := testConfig()
	ext.Projects = []Project{{Domain: "external.test", Routes: []Route{{Host: "@", Backends: []string{"localhost:8080"}}}}}
	data, _ := json.MarshalIndent(ext, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := s.FindProject("external.test"); !ok {
		t.Fatal("external change not visible after Reload")
	}
}

func TestWatchNotifiesOnChange(t *testing.T) {
	dir := t.TempDir()
	path := writeCfg(t, dir, nil)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	changed := make(chan Config, 4)
	stop, err := s.Watch(func(c Config) { changed <- c })
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer stop()

	time.Sleep(300 * time.Millisecond)
	ext := testConfig()
	ext.Projects = []Project{{Domain: "hot.test"}}
	data, _ := json.MarshalIndent(ext, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not notify within timeout")
	}
}

func TestDashboardDomain(t *testing.T) {
	if got := DashboardDomain("test"); got != "dnser.test" {
		t.Errorf("DashboardDomain = %q, want dnser.test", got)
	}
}
