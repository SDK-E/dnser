package desktop

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/SDK-E/dnser/internal/config"
)

func reservePorts(t *testing.T) config.Ports {
	t.Helper()
	got := make([]int, 4)
	for i := range got {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		got[i] = ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
	}
	return config.Ports{DNS: got[0], HTTP: got[1], HTTPS: got[2], UI: got[3]}
}

func testService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DNSER_HOME", dir)
	store, err := config.Open(filepath.Join(dir, "dnser.json"))
	if err != nil {
		t.Fatal(err)
	}
	ports := reservePorts(t)
	if err := store.Update(func(c *config.Config) {
		c.Settings.Ports = ports
		c.Projects = []config.Project{{
			Domain: "desktop.test",
			Routes: []config.Route{
				{Host: "@", Backends: []string{"localhost:35100"}, HTTPS: true},
				{Host: "*", Backends: []string{"localhost:35100"}, HTTPS: true},
			},
		}}
	}); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Store: store, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestServiceLifecycle(t *testing.T) {
	svc := testService(t)

	if svc.Running() {
		t.Fatal("service must not be running before Start")
	}
	st := svc.Status()
	if st.Running || st.Version != "test" || st.TLD != "test" || st.Projects != 1 {
		t.Fatalf("unexpected idle status: %+v", st)
	}

	if err := svc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Start(); err == nil {
		t.Fatal("second Start must fail")
	}
	if !svc.Running() {
		t.Fatal("service must be running after Start")
	}
	if svc.Runtime() == nil {
		t.Fatal("runtime must be exposed after Start")
	}

	st = svc.Status()
	if !st.Running || st.DNSPort <= 0 {
		t.Fatalf("unexpected running status: %+v", st)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if svc.Running() {
		t.Fatal("service must not be running after Stop")
	}
	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("stop when stopped must be a no-op, got %v", err)
	}
}

func TestAPIHandlerServesStatus(t *testing.T) {
	svc := testService(t)
	if err := svc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Stop(ctx)
	})

	handler := svc.APIHandler()
	if handler == nil {
		t.Fatal("handler must be available while running")
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/status")
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code: %d", resp.StatusCode)
	}
	var payload struct {
		Version string `json:"version"`
		TLD     string `json:"tld"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != "test" || payload.TLD != "test" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestNewRequiresStore(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error for missing store")
	}
}
