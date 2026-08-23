package runner

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var helperPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dnser-runner-helper")
	if err != nil {
		panic(err)
	}
	name := "dnser-test-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helperPath = filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", helperPath, "./testdata/helper")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build helper: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitState(t *testing.T, s *Supervisor, domain string, want State, timeout time.Duration) AppInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, ok := s.Get(domain)
		if ok && info.State == want {
			return info
		}
		time.Sleep(50 * time.Millisecond)
	}
	info, _ := s.Get(domain)
	t.Fatalf("state never reached %q (last: %+v)", want, info)
	return AppInfo{}
}

func TestAllocateFreePortExcludes(t *testing.T) {
	exclude := map[int]bool{}
	got, err := AllocateFreePort(exclude)
	if err != nil {
		t.Fatal(err)
	}
	exclude[got] = true
	for i := 0; i < 5; i++ {
		next, err := AllocateFreePort(exclude)
		if err != nil {
			t.Fatal(err)
		}
		if exclude[next] {
			t.Fatalf("returned excluded port %d", next)
		}
	}
}

func TestSubstitutePort(t *testing.T) {
	got := SubstitutePort([]string{"php", "artisan", "serve", "--port={port}", "{port}"}, 4844)
	want := "4844"
	if got[3] != "--port="+want || got[4] != want {
		t.Fatalf("substitution failed: %v", got)
	}
}

func TestReadLinkOverride(t *testing.T) {
	dir := t.TempDir()
	if _, ok := ReadLinkOverride(dir); ok {
		t.Fatal("expected no override for empty dir")
	}
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, dotDnserFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("# dnser project config\ncommand: pnpm dev --port {port}\n")
	got, ok := ReadLinkOverride(dir)
	if !ok || got.Command != "pnpm dev --port {port}" {
		t.Fatalf("inline parse: %+v ok=%v", got, ok)
	}
	write("command:\n  cargo run --release\n")
	got, ok = ReadLinkOverride(dir)
	if !ok || got.Command != "cargo run --release" {
		t.Fatalf("block parse: %+v ok=%v", got, ok)
	}
	write(`command: "npm run dev"` + "\n")
	got, ok = ReadLinkOverride(dir)
	if !ok || got.Command != "npm run dev" {
		t.Fatalf("quoted parse: %+v ok=%v", got, ok)
	}
}

func TestSupervisorLifecycle(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	port := freeTestPort(t)
	sup := NewSupervisor(Options{LogsDir: logsDir})
	defer sup.Shutdown()

	domain := "lifecycle.app.test"
	err := sup.Start(Spec{
		Domain:    domain,
		Framework: "test",
		Port:      port,
		Command:   []string{helperPath},
		PortEnv:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	info := waitState(t, sup, domain, StateUp, 15*time.Second)
	if info.PID == 0 {
		t.Fatal("expected pid to be set")
	}

	data, err := os.ReadFile(filepath.Join(logsDir, domain+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "started pid") || !strings.Contains(string(data), "helper listening") {
		t.Fatalf("log file missing expected lines:\n%s", data)
	}

	if !sup.Stop(domain) {
		t.Fatal("Stop returned false for managed app")
	}
	waitState(t, sup, domain, StateStopped, 10*time.Second)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if dialErr != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("server still accepting connections after stop")
}

func exitSpec(t *testing.T, domain string) Spec {
	return Spec{
		Domain:  domain,
		Port:    freeTestPort(t),
		Command: []string{helperPath},
		Env:     []string{"HELPER_MODE=exit"},
	}
}

func TestSupervisorCrashLoopBackoff(t *testing.T) {
	sup := NewSupervisor(Options{})
	defer sup.Shutdown()

	domain := "crashy.app.test"
	if err := sup.Start(exitSpec(t, domain)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		info, _ := sup.Get(domain)
		if info.Restarts >= 2 && info.State == StateCrashLooping {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	info, _ := sup.Get(domain)
	t.Fatalf("expected crash-looping restarts>=2, got %+v", info)
}

func TestSupervisorStopDuringBackoff(t *testing.T) {
	sup := NewSupervisor(Options{})
	defer sup.Shutdown()

	domain := "backoff-stop.app.test"
	if err := sup.Start(exitSpec(t, domain)); err != nil {
		t.Fatal(err)
	}
	waitState(t, sup, domain, StateCrashLooping, 8*time.Second)
	if !sup.Stop(domain) {
		t.Fatal("stop during crash loop failed")
	}
	waitState(t, sup, domain, StateStopped, 5*time.Second)

	time.Sleep(1500 * time.Millisecond)
	info, _ := sup.Get(domain)
	if info.State != StateStopped || info.Restarts > 6 {
		t.Fatalf("state drifted after stop: %+v", info)
	}
}
