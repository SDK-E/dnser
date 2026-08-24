package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/config"
)

func decodeOrPanic(src string) *config.Manifest {
	m, err := config.Decode([]byte(src))
	if err != nil {
		panic("test fixture invalid: " + err.Error())
	}
	return m
}

func baseInput(src string) Input {
	m := decodeOrPanic(src)
	return Input{
		Project:      "auth",
		Root:         "/home/dev/apps/auth",
		Manifest:     m,
		Port:         3000,
		ServicePorts: map[string]int{"redis": 16379},
		LogsDir:      "/home/dev/.dnser/logs/auth",
		DNSPort:      35353,
	}
}

func TestGenerateMinimal(t *testing.T) {
	in := baseInput("domain: auth.acme.io\n")
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Caddyfile)
	if !strings.Contains(s, "auth.acme.io {") || !strings.Contains(s, "tls internal") {
		t.Fatalf("site block missing: %s", s)
	}
	if !strings.Contains(s, "reverse_proxy 127.0.0.1:3000") {
		t.Fatalf("implicit whole-site route missing")
	}
	if len(out.Resolvers) != 1 || out.Resolvers[0].Suffix != "acme.io" || out.Resolvers[0].Port != 35353 {
		t.Fatalf("resolver reg wrong: %+v", out.Resolvers)
	}
	found := false
	for _, a := range out.Answers {
		if a.Name == "auth.acme.io" && a.Type == "A" && a.Value == "127.0.0.1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("apex A answer missing: %+v", out.Answers)
	}
}

func TestGenerateSupervisorShape(t *testing.T) {
	in := baseInput(`domain: auth.acme.io
port: 3000
command: npm run dev -- --port {port}
services:
  redis:
    command: redis-server --port {port:redis}
    readiness: "tcp://127.0.0.1:{port:redis}"
`)
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Supervisor)
	for _, want := range []string{
		`version: "0.5"`,
		"command: npm run dev -- --port 3000",
		"working_dir: /home/dev/apps/auth",
		"- PORT=3000",
		"- DNSER_DOMAIN=auth.acme.io",
		"command: redis-server --port 16379",
		"nc -z 127.0.0.1 16379",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("supervisor YAML missing %q:\n%s", want, s)
		}
	}
}

func TestGenerateOnRequestDisablesProcess(t *testing.T) {
	in := baseInput("domain: auth.acme.io\ncommand: bin/serve\navailability: on_request\n")
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	sup := string(out.Supervisor)
	if !strings.Contains(sup, "disabled: true") {
		t.Fatalf("on_request must emit disabled process (wake glue starts it):\n%s", sup)
	}
	if strings.Contains(sup, "restart: on_failure") {
		t.Fatalf("sleeping project must not auto-restart outside glue control:\n%s", sup)
	}
	cf := string(out.Caddyfile)
	if !strings.Contains(cf, "handle_errors") {
		t.Fatalf("sleeping project site needs branded error page:\n%s", cf)
	}
}

func TestGenerateHTTPSMix(t *testing.T) {
	in := baseInput(`domains:
  - app.acme.io
  - "*.preview.app.acme.io"
https:
  "*.preview.app.acme.io": false
`)
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Caddyfile)
	if strings.Count(s, "tls internal") != 1 {
		t.Fatalf("only apex should have tls internal:\n%s", s)
	}
	if !strings.Contains(s, "http://*.preview.app.acme.io") {
		t.Fatalf("disabled name must be plain http site:\n%s", s)
	}
}

func TestGenerateRawCaddyAndPlaceholders(t *testing.T) {
	in := baseInput(`domain: app.acme.io
port: 4000
caddy:
  encode: gzip
  header:
    +X-Powered-By: dnser
    +X-Frame-Options: DENY
  log:
    output: file
    file: "{logs_dir}/access.log"
  "@denied":
    respond: ["nope", "403"]
`)
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Caddyfile)
	for _, want := range []string{
		"encode gzip",
		"+X-Powered-By dnser",
		"/home/dev/.dnser/logs/auth/access.log",
		"@denied {",
		"respond nope 403",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestGenerateRawProcessMergeLastWriter(t *testing.T) {
	in := baseInput(`domain: app.acme.io
command: bin/serve
process:
  shutdown:
    signal: SIGINT
    timeout_seconds: 15
  availability:
    restart: always
`)
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Supervisor)
	if !strings.Contains(s, "timeout_seconds: 15") {
		t.Fatalf("raw shutdown missing:\n%s", s)
	}
	if !strings.Contains(s, "restart: always") {
		t.Fatalf("delegated restart policy missing:\n%s", s)
	}
}

func TestGenerateEnvFileMergedIntoEnvironment(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envPath, []byte("SECRET_TOKEN=abc123\nSHARED=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := baseInput("domain: app.acme.io\ncommand: bin/serve\nenv_file: " + envPath + "\nenv:\n  SHARED: manifest\n")
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out.Supervisor)
	if !strings.Contains(s, "- SECRET_TOKEN=abc123") {
		t.Fatalf("env_file value not injected:\n%s", s)
	}
	if !strings.Contains(s, "- SHARED=manifest") {
		t.Fatalf("manifest env must win over env_file:\n%s", s)
	}
}

func TestGenerateProxyOnlyNoCommand(t *testing.T) {
	in := baseInput("domain: app.acme.io\n")
	out, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Supervisor != nil {
		t.Fatalf("proxy-only must not emit supervisor config")
	}
	if !strings.Contains(string(out.Caddyfile), "HTML 503") {
		t.Fatalf("proxy-only site should serve branded 503 until backends defined")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	src := `domains: [app.acme.io]
aliases: [app.local]
port: 4000
command: bin/serve --log {logs_dir}/x.log
services:
  redis:
    command: redis-server --port {port:redis}
  smtp:
    type: smtp
    host: 127.0.0.1
    port: 11025
    dns: true
routes:
  - path: /api/*
    port: 4001
records:
  - name: "@"
    type: TXT
    value: v=verify
`
	in := baseInput(src)
	a, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Caddyfile) != string(b.Caddyfile) || string(a.Supervisor) != string(b.Supervisor) {
		t.Fatalf("generation is nondeterministic")
	}
}

func TestGenerateInvalidManifestNeverEmits(t *testing.T) {
	in := baseInput("domains: [app.acme.io]\nport: 3000\nrecords:\n  - {name: evil.example.com, type: TXT, value: x}\n")
	if _, err := Generate(in); err == nil {
		t.Fatalf("invalid manifest must fail generation before any output")
	}
}

func TestEmitFileValidationFailureKeepsLastKnownGood(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gen.conf")
	good := []byte("good-config\n")
	if err := EmitFile(path, good, 0o644, nil); err != nil {
		t.Fatal(err)
	}
	failing := func(string) error { return errFake }
	err := EmitFile(path, []byte("bad-config\n"), 0o644, failing)
	if err == nil || !strings.Contains(err.Error(), "keeping last known good") {
		t.Fatalf("expected LKG error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(good) {
		t.Fatalf("LKG was overwritten: %q", data)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".gen.conf.tmp-") {
			t.Fatalf("temp file leaked: %s", e.Name())
		}
	}
}

func TestEmitFileFirstWriteFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.conf")
	failing := func(string) error { return errFake }
	if err := EmitFile(path, []byte("x"), 0o644, failing); err == nil {
		t.Fatalf("expected failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed first emission must not create target")
	}
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake validator rejection" }
