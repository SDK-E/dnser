package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/runner"
)

func testStore(t *testing.T) *config.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DNSER_HOME", dir)
	s, err := config.Open(filepath.Join(dir, "dnser.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(c *config.Config) {
		c.Settings.Ports.DNS = 35000
		c.Settings.Ports.HTTP = 35001
		c.Settings.Ports.HTTPS = 35002
		c.Settings.Ports.UI = 35003
		c.Projects = []config.Project{{
			Domain: "wizard.test",
			Routes: []config.Route{
				{Host: "@", Backends: []string{"localhost:35100"}, HTTPS: true},
				{Host: "*", Backends: []string{"localhost:35100"}, HTTPS: true},
			},
		}}
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func startEchoServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "echo-ok")
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func TestRuntimeServesDNSAndProxy(t *testing.T) {
	store := testStore(t)
	target := startEchoServer(t)
	if err := store.Update(func(c *config.Config) {
		for i := range c.Projects {
			backend := fmt.Sprintf("localhost:%d", portOf(target))
			for j := range c.Projects[i].Routes {
				c.Projects[i].Routes[j].Backends = []string{backend}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	rt, err := New(Options{Store: store})
	if err != nil {
		t.Fatalf("daemon start: %v", err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion("api.wizard.test.", dns.TypeA)
	resp, _, err := client.Exchange(req, fmt.Sprintf("127.0.0.1:%d", rt.DNSPort()))
	if err != nil {
		t.Fatalf("dns query failed: %v", err)
	}
	if !resp.Authoritative || len(resp.Answer) != 1 {
		t.Fatalf("dns response wrong: auth=%v answers=%v", resp.Authoritative, resp.Answer)
	}

	pool := x509.NewCertPool()
	pool.AddCert(rt.Manager().CA().Certificate())
	hc := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   3 * time.Second,
	}
	httpReq, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", store.Settings().Ports.HTTPS), nil)
	httpReq.Host = "deep.wizard.test"
	hresp, err := hc.Do(httpReq)
	if err != nil {
		t.Fatalf("proxied request failed: %v", err)
	}
	body, _ := io.ReadAll(hresp.Body)
	_ = hresp.Body.Close()
	if string(body) != "echo-ok" {
		t.Errorf("proxy body = %q", body)
	}

	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) && !found {
		for _, st := range rt.Checker().Snapshot() {
			if st.Up {
				found = true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Error("health checker never reported wizard.test up")
	}
}

func TestRuntimeHotReloadPicksUpNewProject(t *testing.T) {
	store := testStore(t)
	rt, err := New(Options{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	if _, ok := rt.Router().Lookup("second.test", "/"); ok {
		t.Fatal("second.test should not be routed before link")
	}

	reloaded := make(chan struct{}, 1)
	go func() {
		select {
		case <-rt.NotifyReload():
			reloaded <- struct{}{}
		case <-time.After(3 * time.Second):
		}
	}()

	if err := store.Update(func(c *config.Config) {
		c.Projects = append(c.Projects, config.Project{Domain: "second.test", Routes: []config.Route{{Host: "@", Backends: []string{"localhost:35200"}, HTTPS: true}}})
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-reloaded:
	case <-time.After(4 * time.Second):
		t.Fatal("reload notification never arrived")
	}

	if _, ok := rt.Router().Lookup("second.test", "/"); !ok {
		t.Error("hot reload did not add route for second.test")
	}
}

func portOf(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p := 0
	_, _ = fmt.Sscanf(portStr, "%d", &p)
	return p
}

func TestRuntimeUIPortFallsBackWhenOccupied(t *testing.T) {
	store := testStore(t)

	held, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", store.Settings().Ports.UI))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()

	rt, err := New(Options{Store: store, Version: "test", SkipListeners: false})
	if err != nil {
		t.Fatalf("daemon must start despite occupied UI port: %v", err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	got := rt.UIPort()
	if got == store.Settings().Ports.UI || got <= 0 {
		t.Fatalf("expected ephemeral fallback port, got %d", got)
	}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", got))
	if err != nil {
		t.Fatalf("dashboard unreachable on fallback port: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestRunnerSyncStartsManagedProject(t *testing.T) {
	store := testStore(t)
	binPath := filepath.Join(t.TempDir(), "apphelper")
	build := exec.Command("go", "build", "-o", binPath, "./testdata/apphelper")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build apphelper: %v\n%s", err, out)
	}

	projectDir := t.TempDir()
	if err := store.Update(func(c *config.Config) {
		c.Projects = append(c.Projects, config.Project{
			Domain: "app.wizard.test",
			Path:   projectDir,
			Run:    &config.RunConfig{Command: binPath},
			Routes: []config.Route{
				{Host: "@", Backends: []string{"127.0.0.1:35199"}, HTTPS: true},
			},
		})
	}); err != nil {
		t.Fatal(err)
	}

	rt, err := New(Options{Store: store, Version: "test", SkipListeners: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	deadline := time.Now().Add(15 * time.Second)
	var info runner.AppInfo
	for time.Now().Before(deadline) {
		if got, ok := rt.Runner().Get("app.wizard.test"); ok && got.State == runner.StateUp {
			info = got
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if info.State != runner.StateUp || info.Port == 0 {
		got, _ := rt.Runner().Get("app.wizard.test")
		t.Fatalf("app never came up: %+v depsMissing=%v", got, rt.DepsMissing())
	}

	snap := store.Get()
	var runPort int
	for _, p := range snap.Projects {
		if p.Domain == "app.wizard.test" && p.Run != nil {
			runPort = p.Run.Port
		}
	}
	if runPort != info.Port {
		t.Fatalf("persisted port %d != running port %d", runPort, info.Port)
	}
}

func TestRunnerSyncRemovesUnmanagedProject(t *testing.T) {
	store := testStore(t)
	projectDir := t.TempDir()
	if err := store.Update(func(c *config.Config) {
		c.Projects = append(c.Projects, config.Project{
			Domain: "gone.wizard.test",
			Path:   projectDir,
			Run:    &config.RunConfig{Command: "sleep 30"},
		})
	}); err != nil {
		t.Fatal(err)
	}
	rt, err := New(Options{Store: store, Version: "test", SkipListeners: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	waitFor(t, func() bool {
		_, ok := rt.Runner().Get("gone.wizard.test")
		return ok
	}, "project to start")

	if err := store.Update(func(c *config.Config) {
		kept := c.Projects[:0]
		for _, p := range c.Projects {
			if p.Domain != "gone.wizard.test" {
				kept = append(kept, p)
			}
		}
		c.Projects = kept
	}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool {
		_, ok := rt.Runner().Get("gone.wizard.test")
		return !ok
	}, "project removal")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
