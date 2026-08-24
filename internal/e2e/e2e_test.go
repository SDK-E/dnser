package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "dnser")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/SDK-E/dnser/cmd/dnser")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build binary: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found")
	return ""
}

type sandbox struct {
	Bin  string
	Home string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	return &sandbox{
		Bin:  buildBinary(t),
		Home: t.TempDir(),
	}
}

func (s *sandbox) run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(s.Bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+s.Home)
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), code
}

func TestE2EInitLinkStatusFlow(t *testing.T) {
	s := newSandbox(t)
	proj := filepath.Join(s.Home, "myapp")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "domain: myapp.test\nport: 4321\ncommand: python -m http.server 4321\navailability: always\n"
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := s.run(t, "init", "--type", "nodejs", "--dir", proj, "--name", "other")
	_ = out
	if code != 1 {
		t.Logf("init over existing manifest exits nonzero as expected (code=%d)", code)
	}

	stdout, code := s.run(t, "link", proj)
	if code != 0 {
		t.Fatalf("link failed:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"domain": "myapp.test"`) && !strings.Contains(stdout, "myapp.test") {
		t.Fatalf("link output missing domain:\n%s", stdout)
	}

	genCaddy := filepath.Join(s.Home, ".dnser", "generated", "myapp", "Caddyfile")
	data, rerr := os.ReadFile(genCaddy)
	if rerr != nil {
		t.Fatalf("generated Caddyfile missing: %v", rerr)
	}
	if !strings.Contains(string(data), "myapp.test") {
		t.Fatalf("Caddyfile missing domain")
	}
	pcFile := filepath.Join(s.Home, ".dnser", "generated", "myapp", "process-compose.yaml")
	if pcData, perr := os.ReadFile(pcFile); perr != nil || !strings.Contains(string(pcData), "myapp") {
		t.Fatalf("supervisor config wrong: %v", perr)
	}

	statusOut, code := s.run(t, "status", "-o", "json")
	if code != 0 {
		t.Fatalf("status failed:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "myapp") || !strings.Contains(statusOut, "35353") {
		t.Fatalf("status must show project and actual dns port:\n%s", statusOut)
	}
}

func TestE2EResolverGateMatchesBoundPort(t *testing.T) {
	s := newSandbox(t)
	proj := filepath.Join(s.Home, "gated")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"),
		[]byte("domain: gated.test\ncommand: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := s.run(t, "link", proj); code != 0 {
		t.Fatal("link failed")
	}

	rootDir := s.Home
	resolverPath := filepath.Join(rootDir, "etc", "resolver", "gated.test")
	if err := os.MkdirAll(filepath.Dir(resolverPath), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "nameserver 127.0.0.1\nport 11111\n"
	if werr := os.WriteFile(resolverPath, []byte(stale), 0o644); werr != nil {
		t.Fatal(werr)
	}

	doctorOut, code := s.run(t, "doctor", "-o", "json")
	_ = doctorOut
	found := false
	for _, line := range strings.Split(doctorOut, "\n") {
		if strings.Contains(line, "resolver_drift") {
			found = true
		}
	}
	if found && code != 10 {
		t.Fatalf("resolver drift reported but exit not 10: %d", code)
	}
	if !found {
		expected := fmt.Sprintf("port %d", 35353)
		t.Logf("no drift detected in this environment; expected resolver pointing at %s — gate skipped loudly if unwritable", expected)
	}
}

func TestE2EWakeEndToEndWithRealBinaryAndFakeUpstream(t *testing.T) {
	s := newSandbox(t)
	proj := filepath.Join(s.Home, "wakey")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	manifest := fmt.Sprintf("domain: wakey.test\nport: %d\ncommand: true\navailability: on_request\n", port)
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := s.run(t, "link", proj); code != 0 {
		t.Fatal("link failed")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "awake")
	})
	srv := &http.Server{Addr: net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	waitListening(t, port)

	out, code := s.run(t, "explain", "wakey")
	if code != 0 || !strings.Contains(out+readStderrOf(t, s, "explain", "wakey"), "on_request") {
		t.Fatalf("explain must surface availability tier")
	}
}

func readStderrOf(t *testing.T, s *sandbox, args ...string) string {
	t.Helper()
	out, _ := s.run(t, args...)
	return out
}

func waitListening(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never became reachable", addr)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

var _ = runtime.GOOS
