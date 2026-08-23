package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRaw(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateV1DesugarsRoutes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dnser.json")
	v1 := `{
  "version": 1,
  "settings": {"tld": "test", "bind": "127.0.0.1", "upstreams": ["1.1.1.1:53"], "ports": {"dns": 53, "http": 80, "https": 443, "ui": 4500}},
  "projects": [
    {
      "domain": "myproject.test",
      "port": 3000,
      "wildcard": true,
      "https": true,
      "force_https": true,
      "aliases": ["legacy.other.test"],
      "records": [{"type": "TXT", "name": "@", "value": "kept"}],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-02T00:00:00Z"
    },
    {
      "domain": "dnsonly.test",
      "port": 0
    }
  ]
}`
	writeRaw(t, path, v1)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open v1 config: %v", err)
	}
	cfg := s.Get()
	if cfg.Version != CurrentVersion {
		t.Errorf("version = %d, want %d", cfg.Version, CurrentVersion)
	}

	p := cfg.Projects[0]
	if len(p.Routes) != 3 {
		t.Fatalf("routes = %d (%+v), want 3 (apex, wildcard, alias)", len(p.Routes), p.Routes)
	}
	wantHosts := []string{"@", "*", "legacy.other.test"}
	for i, host := range wantHosts {
		if p.Routes[i].Host != host {
			t.Errorf("routes[%d].host = %q, want %q", i, p.Routes[i].Host, host)
		}
	}
	for i := range wantHosts {
		if got := strings.Join(p.Routes[i].Backends, ","); got != "localhost:3000" {
			t.Errorf("routes[%d] backends = %q", i, got)
		}
	}
	if !p.Routes[0].HTTPS || !p.Routes[0].ForceHTTPS || !p.Routes[2].HTTPS {
		t.Error("https flags not inherited by migrated routes")
	}
	if len(p.Records) != 1 || p.Records[0].Value != "kept" {
		t.Error("records not preserved through migration")
	}
	if !p.CreatedAt.Equal(p.CreatedAt) || p.CreatedAt.IsZero() {
		t.Error("timestamps lost")
	}
	if len(cfg.Projects[1].Routes) != 0 {
		t.Error("port-0 project should migrate with no routes")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Version != CurrentVersion {
		t.Errorf("config file not migrated on disk: version %d", onDisk.Version)
	}
	if strings.Contains(string(raw), `"port": 3000`) {
		t.Error("legacy port field still present after migration write-back")
	}
}

func TestValidateRouteRules(t *testing.T) {
	base := func(mut func(*Config)) Config {
		cfg := Default()
		cfg.Projects = []Project{{Domain: "app.test"}}
		mut(&cfg)
		return cfg
	}

	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid mixed routes",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{
					{Host: "@", Backends: []string{"localhost:3000"}, HTTPS: true},
					{Host: "*", Backends: []string{"localhost:3000"}, HTTPS: true},
					{Host: "api", Backends: []string{"10.0.0.5:3001", "localhost:3002"}},
					{Host: "smtp.other.test", Backends: []string{"localhost:1025"}},
					{Host: "relay", TCP: true, Listen: 30250, Backends: []string{"localhost:1025"}},
				}
			}),
		},
		{
			name: "empty backends rejected",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "@"}}
			}),
			wantErr: "at least one backend",
		},
		{
			name: "bad backend format",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "@", Backends: []string{"localhost"}}}
			}),
			wantErr: "must be host:port",
		},
		{
			name: "backend port out of range",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "@", Backends: []string{"localhost:70000"}}}
			}),
			wantErr: "out of range",
		},
		{
			name: "duplicate route hosts",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{
					{Host: "@", Backends: []string{"localhost:3000"}},
					{Host: "api", Backends: []string{"localhost:3000"}},
					{Host: "api", Backends: []string{"localhost:3001"}},
				}
			}),
			wantErr: `duplicate route host "api.app.test"`,
		},
		{
			name: "tcp without listen",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "smtp", TCP: true, Backends: []string{"localhost:1025"}}}
			}),
			wantErr: "listen port required",
		},
		{
			name: "listen on non-tcp route",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "@", Listen: 4000, Backends: []string{"localhost:3000"}}}
			}),
			wantErr: "only valid for tcp",
		},
		{
			name: "tcp listen collides with core port",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "x", TCP: true, Listen: c.Settings.Ports.DNS, Backends: []string{"localhost:1025"}}}
			}),
			wantErr: "already used by settings.ports.dns",
		},
		{
			name: "tcp listen collides across projects",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "a", TCP: true, Listen: 40000, Backends: []string{"localhost:1"}}}
				c.Projects = append(c.Projects, Project{Domain: "b.test"})
				c.Projects[1].Routes = []Route{{Host: "b", TCP: true, Listen: 40000, Backends: []string{"localhost:2"}}}
			}),
			wantErr: "already used by projects[0]",
		},
		{
			name: "force_https without https",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "@", ForceHTTPS: true, Backends: []string{"localhost:3000"}}}
			}),
			wantErr: "force_https requires https",
		},
		{
			name: "route collides with other project domain",
			cfg: base(func(c *Config) {
				c.Projects[0].Routes = []Route{{Host: "www", Backends: []string{"localhost:3000"}}}
				c.Projects = append(c.Projects, Project{Domain: "www.app.test"})
			}),
			wantErr: `duplicate domain`,
		},
		{
			name: "bad run mode",
			cfg: base(func(c *Config) {
				c.Projects[0].Run = &RunConfig{Mode: "production"}
			}),
			wantErr: `run.mode`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveHost(t *testing.T) {
	cases := []struct{ host, domain, want string }{
		{"@", "app.test", "app.test"},
		{"*", "app.test", "*.app.test"},
		{"api", "app.test", "api.app.test"},
		{"deep.sub", "app.test", "deep.sub.app.test"},
		{"legacy.other.test", "app.test", "legacy.other.test"},
	}
	for _, tc := range cases {
		if got := ResolveHost(tc.host, tc.domain, "test"); got != tc.want {
			t.Errorf("ResolveHost(%q, %q) = %q, want %q", tc.host, tc.domain, got, tc.want)
		}
	}
}
