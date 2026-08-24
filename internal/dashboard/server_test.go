package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/state"
)

func testDeps(t *testing.T) Deps {
	t.Helper()
	dir := t.TempDir()
	stPath := filepath.Join(dir, "state.json")
	st, err := state.Open(stPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Link(state.LinkedProject{Name: "api", Domain: "api.test", Port: 3001, Availability: "always"}); err != nil {
		t.Fatal(err)
	}
	return Deps{
		State: st,
		ListIssues: func(ctx context.Context) []Issue {
			return []Issue{{Kind: "fallback_dns", Evidence: "2 of 3 entries absent", Fix: "dnser elevate"}}
		},
		DNSPort:       func() int { return 35353 },
		SupervisorRun: false,
		LogLines: func(ctx context.Context, project string, tail int) ([]string, error) {
			if project == "api" {
				return []string{"listening on 3001", "ready"}, nil
			}
			return nil, fmt.Errorf("no log yet for %s", project)
		},
	}
}

func TestAuthRejectsMissingAndBadTokens(t *testing.T) {
	deps := testDeps(t)
	h := Handler(deps, "tok-1234567890abcdef")
	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token must 401, got %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token must 401, got %d", res.StatusCode)
	}

	req.Header.Set("Authorization", "Bearer tok-1234567890abcdef")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("good token must pass, got %d", res.StatusCode)
	}
}

func TestLoopbackOnlyEnforcement(t *testing.T) {
	deps := testDeps(t)
	handler := Handler(deps, "tok")
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "10.1.2.3:55555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback source must be refused even with token: got %d", rec.Code)
	}
	req.RemoteAddr = "127.0.0.1:55555"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code == http.StatusForbidden {
		t.Fatalf("loopback refused incorrectly: %d", rec2.Code)
	}
}

func TestEndpointsGoldenShapes(t *testing.T) {
	deps := testDeps(t)
	h := Handler(deps, "tok")
	ts := httptest.NewServer(h)
	defer ts.Close()
	get := func(path string) map[string]any {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var m map[string]any
		if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	status := get("/api/status")
	if status["dns_port"].(float64) != 35353 {
		t.Fatalf("dns_port wrong: %+v", status)
	}
	rows := status["projects"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["name"] != "api" {
		t.Fatalf("project rows wrong: %+v", rows)
	}

	doctor := get("/api/doctor")
	if doctor["count"].(float64) != 1.0 {
		t.Fatalf("doctor count wrong: %+v", doctor)
	}

	logs := get("/api/logs/api")
	lines := logs["lines"].([]any)
	if len(lines) != 2 || lines[0].(map[string]any)["line"] != "listening on 3001" {
		t.Fatalf("log lines wrong: %+v", logs)
	}

	req404, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/logs/ghost", nil)
	req404.Header.Set("Authorization", "Bearer tok")
	res, err := http.DefaultClient.Do(req404)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("missing log must 404, got %d", res.StatusCode)
	}
}

func TestEnsureTokenPersistsAndReuses(t *testing.T) {
	tok1, err := EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok1) < 32 {
		t.Fatalf("token too short: %q", tok1)
	}
	tok2, err := EnsureToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok1 != tok2 {
		t.Fatal("token must be stable across calls")
	}
	path, _ := tokenPath()
	info, serr := os.Stat(path)
	if serr != nil {
		t.Fatal(serr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token file perms must be 0600, got %v", info.Mode().Perm())
	}
	if !strings.Contains(path, ".dnser") {
		t.Fatalf("token path unexpected: %s", path)
	}
}
