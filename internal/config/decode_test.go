package config

import (
	"strings"
	"testing"
)

func TestDecodeMinimal(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, m *Manifest)
	}{
		{
			name: "single domain",
			yaml: "domain: auth.mycompany.internal\n",
			check: func(t *testing.T, m *Manifest) {
				if m.Domain != "auth.mycompany.internal" {
					t.Fatalf("domain = %q", m.Domain)
				}
			},
		},
		{
			name: "full featured manifest",
			yaml: `
type: nodejs
domains:
  - auth.mycompany.internal
  - "*.preview.auth.mycompany.internal"
aliases: [auth.local]
port: 3000
command: npm run dev -- --port {port}
shell: /bin/zsh
cwd: packages/auth
https:
  "*.preview.auth.mycompany.internal": false
force_https: true
env:
  NODE_ENV: development
env_file: [.env.local]
services:
  redis:
    image_hint: redis
    command: redis-server --port {port:redis}
    readiness: "tcp://127.0.0.1:{port:redis}"
routes:
  - path: /api/*
    port: 4000
records:
  - {name: "@", type: TXT, value: "v=verification abc"}
forward:
  - {proto: tcp, listen: 11025, to: 11025}
process:
  restart: always
caddy:
  encode: gzip
availability: always
idle_stop: 45m
min_uptime: 3m
`,
			check: func(t *testing.T, m *Manifest) {
				if len(m.Domains) != 2 || !m.HTTPS.PerName["*.preview.auth.mycompany.internal"] == true && m.HTTPS.PerName == nil {
					t.Fatalf("domains/https misdecoded: %+v %+v", m.Domains, m.HTTPS)
				}
				if m.Shell.Path != "/bin/zsh" || !m.Shell.Enabled {
					t.Fatalf("shell = %+v", m.Shell)
				}
				if m.ForceHTTPS != true {
					t.Fatalf("force_https = %v", m.ForceHTTPS)
				}
				if len(m.EnvFile) != 1 || m.EnvFile[0] != ".env.local" {
					t.Fatalf("env_file = %v", m.EnvFile)
				}
				if m.IdleStop.Value.Minutes() != 45 || m.MinUptime.Value.Minutes() != 3 {
					t.Fatalf("durations = %v %v", m.IdleStop, m.MinUptime)
				}
				if m.Process == nil || m.Process.Value["restart"] != "always" {
					t.Fatalf("process raw = %+v", m.Process)
				}
			},
		},
		{
			name:    "unknown key rejected with line number",
			yaml:    "domain: a.test\ndomian: b.test\n",
			wantErr: true,
		},
		{
			name:    "bad availability value",
			yaml:    "domain: a.test\navailability: sometimes\n",
			wantErr: true,
		},
		{
			name:    "invalid forward proto",
			yaml:    "domain: a.test\nforward:\n  - {proto: sctp, listen: 1, to: 2}\n",
			wantErr: true,
		},
		{
			name:    "on_request with forward rejected (push protocol rule)",
			yaml:    "domain: a.test\navailability: on_request\nforward:\n  - {proto: tcp, listen: 25, to: 25}\n",
			wantErr: true,
		},
		{
			name:    "on_request with smtp-class service rejected",
			yaml:    "domain: a.test\navailability: on_request\nservices:\n  mailer: {type: smtp, port: 1025}\n",
			wantErr: true,
		},
		{
			name:    "on_request with web service allowed",
			yaml:    "domain: a.test\navailability: on_request\nservices:\n  ui: {type: http, port: 3000}\n",
			wantErr: false,
		},
		{
			name:    "https as mapping of wrong value type",
			yaml:    "domain: a.test\nhttps:\n  a.test: notabool\n",
			wantErr: true,
		},
		{
			name:    "idle_stop garbage duration",
			yaml:    "domain: a.test\nidle_stop: soonish\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Decode([]byte(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if strings.Contains(tt.yaml, "domian") && !strings.Contains(err.Error(), "line") {
					t.Fatalf("expected line number in error, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	src := []byte("type: nodejs\ndomain: app.acme.io\nport: 3000\ncommand: npm run dev -- --port {port}\n")
	m, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Decode(out)
	if err != nil {
		t.Fatalf("re-decode failed: %v\n%s", err, out)
	}
	if m2.Command != m.Command || m2.Domain != m.Domain {
		t.Fatalf("round trip mismatch")
	}
}
