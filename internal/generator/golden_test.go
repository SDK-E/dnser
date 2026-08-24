package generator

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/SDK-E/dnser/internal/config"
)

var updateGoldens = flag.Bool("update", false, "rewrite golden files")

type goldenCase struct {
	name string
	in   Input
}

func goldenCases(t *testing.T) []goldenCase {
	t.Helper()
	root := filepath.Join("testdata", "fixture-root")
	if err := os.MkdirAll(filepath.Join(root, "apps", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.development"), []byte("FROM_FILE=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newInput := func(src string) Input {
		in := baseInput(src)
		in.Root = root
		return in
	}
	return []goldenCase{
		{
			name: "minimal",
			in:   newInput("domain: auth.mycompany.internal\n"),
		},
		{
			name: "pin",
			in: newInput(`domain: app.acme.io
command: npm run dev -- --port {port}
`),
		},
		{
			name: "monorepo",
			in: func() Input {
				in := newInput(`domain: app.acme.io
cwd: apps/web
command: pnpm dev
services:
  db:
    type: postgres
    host: 127.0.0.1
    port: 55432
  redis:
    command: redis-server --port {port:redis}
    readiness: "tcp://127.0.0.1:{port:redis}"
routes:
  - path: /api/*
    port: 4000
env_file: .env.development
`)
				in.ServicePorts["redis"] = 16379
				return in
			}(),
		},
		{
			name: "full-control",
			in: newInput(`domain: app.acme.io
port: 4000
command: bin/serve
process:
  shutdown:
    timeout_seconds: 15
  availability:
    restart: always
caddy:
  encode: gzip
  header:
    +X-Frame-Options: DENY
    +X-Powered-By: dnser
  log:
    output: file
    file: "{logs_dir}/access.log"
  "@denied":
    path:
      - /private/*
  abort: "@denied"
  handle:
    - path:
        - /assets/*
      root: apps/web/public
routes:
  - host: admin.app.acme.io
    backend: 127.0.0.1:4001
records:
  - name: "@"
    type: TXT
    value: v=verification abc
forward:
  - proto: tcp
    listen: 11025
    to: 11025
`),
		},
		{
			name: "on-request",
			in: newInput(`domain: sleepy.acme.io
command: bin/serve
availability: on_request
idle_stop: 45m
min_uptime: 3m
`),
		},
		{
			name: "https-mix",
			in: func() Input {
				in := newInput(`domains:
  - app.acme.io
  - "*.preview.app.acme.io"
aliases: [app.local]
https:
  "*.preview.app.acme.io": false
command: bin/serve
services:
  smtp:
    type: smtp
    port: 11025
    dns: true
    command: maildev --smtp-port {port:smtp}
`)
				in.ServicePorts["smtp"] = 11025
				return in
			}(),
		},
	}
}

func TestGoldenFiles(t *testing.T) {
	cases := goldenCases(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Generate(tc.in)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			dir := filepath.Join("testdata", "golden")

			gotCaddy := string(out.Caddyfile)
			gotPC := ""
			if out.Supervisor != nil {
				gotPC = string(out.Supervisor)
			}
			gotResolvers := renderResolvers(out)
			gotAnswers := renderAnswers(out)

			if *updateGoldens {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".caddyfile"), []byte(gotCaddy), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".pc.yaml"), []byte(gotPC), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".resolvers.txt"), []byte(gotResolvers), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".answers.txt"), []byte(gotAnswers), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			assertGolden(t, filepath.Join(dir, tc.name+".caddyfile"), gotCaddy)
			assertGolden(t, filepath.Join(dir, tc.name+".pc.yaml"), gotPC)
			assertGolden(t, filepath.Join(dir, tc.name+".resolvers.txt"), gotResolvers)
			assertGolden(t, filepath.Join(dir, tc.name+".answers.txt"), gotAnswers)
		})
	}
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing (%s); run go test ./internal/generator -update", path)
	}
	if string(want) != got {
		t.Fatalf("%s drifted:\n--- want ---\n%s\n--- got ---\n%s\n\nupdate: go test ./internal/generator -update", path, want, got)
	}
}

func renderResolvers(out *Output) string {
	s := ""
	for _, r := range out.Resolvers {
		s += fmt.Sprintf("%s %d\n", r.Suffix, r.Port)
	}
	return s
}

func renderAnswers(out *Output) string {
	s := ""
	for _, a := range out.Answers {
		s += fmt.Sprintf("%s IN %-4s %s\n", a.Name, a.Type, a.Value)
	}
	return s
}

func TestGenerateDeterministicAcrossRuns(t *testing.T) {
	for _, tc := range goldenCases(t) {
		a, err := Generate(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Generate(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if string(a.Caddyfile) != string(b.Caddyfile) || string(a.Supervisor) != string(b.Supervisor) ||
			renderResolvers(a) != renderResolvers(b) || renderAnswers(a) != renderAnswers(b) {
			t.Fatalf("%s: generation is nondeterministic", tc.name)
		}
	}
}

func TestGenerateEnvFileFixture(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("FROM_FILE=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := decodeOrPanic("domain: x.test\ncommand: run\nenv_file: " + envPath + "\n")
	in := Input{Project: "x", Root: "/tmp/x", Manifest: m, LogsDir: "/logs/x"}
	if _, err := Generate(in); err != nil {
		t.Fatalf("env_file loading inside generator failed: %v", err)
	}
}

func TestGenerateMissingFieldsRejected(t *testing.T) {
	m := decodeOrPanic("domain: x.test\n")
	in := Input{Project: "", Root: "/tmp/x", Manifest: m, LogsDir: "/l"}
	if _, err := Generate(in); err == nil {
		t.Fatalf("missing project must be rejected")
	}
	in2 := Input{Project: "x", Root: "/tmp/x", Manifest: nil, LogsDir: "/l"}
	if _, err := Generate(in2); err == nil {
		t.Fatalf("missing manifest must be rejected")
	}
}

var _ = config.DefaultTLD
