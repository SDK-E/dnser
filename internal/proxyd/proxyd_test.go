package proxyd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/certs"
)

func testManager(t *testing.T) *certs.Manager {
	t.Helper()
	ca, err := certs.NewCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return certs.NewManager(ca)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestRouterLookupExactWildcardAndPort(t *testing.T) {
	r := NewRouter()
	r.Replace([]Route{
		{Host: "myproject.test", Target: "127.0.0.1:3000", HTTPS: true, Port: 3000},
		{Host: "*.myproject.test", Target: "127.0.0.1:3000", HTTPS: true, Port: 3000},
		{Host: "plain.dev", Target: "127.0.0.1:5173", Port: 5173},
	})

	cases := []struct {
		host   string
		target string
		ok     bool
	}{
		{"myproject.test", "127.0.0.1:3000", true},
		{"MYPROJECT.test:443", "127.0.0.1:3000", true},
		{"api.myproject.test", "127.0.0.1:3000", true},
		{"a.b.myproject.test", "127.0.0.1:3000", true},
		{"plain.dev", "127.0.0.1:5173", true},
		{"sub.plain.dev", "", false},
		{"unknown.test", "", false},
	}
	for _, c := range cases {
		rt, ok := r.Lookup(c.host)
		if ok != c.ok || (ok && rt.Target != c.target) {
			t.Errorf("Lookup(%q) = (%+v, %v), want target %q ok=%v", c.host, rt, ok, c.target, c.ok)
		}
	}

	targets := r.Targets()
	if len(targets) != 3 {
		t.Errorf("targets = %v, want 3 entries", targets)
	}
}

func TestProxyHTTPPlainAndRedirect(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s proto=%s path=%s", r.Host, r.Header.Get("X-Forwarded-Proto"), r.URL.Path)
	}))
	defer upstream.Close()

	manager := testManager(t)
	router := NewRouter()
	router.Replace([]Route{
		{Host: "plain.dev", Target: strings.TrimPrefix(upstream.URL, "http://"), Port: 1},
		{Host: "myproject.test", Target: strings.TrimPrefix(upstream.URL, "http://"), HTTPS: true, ForceHTTPS: true, Port: 1},
	})

	srv := NewServer(router, manager)
	httpPort := freePort(t)
	httpsPort := freePort(t)
	if err := srv.Serve(fmt.Sprintf("127.0.0.1:%d", httpPort), fmt.Sprintf("127.0.0.1:%d", httpsPort)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/hello?x=1", httpPort), nil)
	req.Host = "plain.dev"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with Host header: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "path=/hello") {
		t.Errorf("unexpected status=%d body=%q", resp.StatusCode, body)
	}

	tlsReq, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/x", httpPort), nil)
	tlsReq.Host = "myproject.test"
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	redirectResp, err := client.Do(tlsReq)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	_ = redirectResp.Body.Close()
	if redirectResp.StatusCode != http.StatusPermanentRedirect {
		t.Errorf("want 308, got %d", redirectResp.StatusCode)
	}
	loc := redirectResp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://myproject.test") {
		t.Errorf("location = %q", loc)
	}
}

func TestProxyHTTPSWithTLS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "yes")
		fmt.Fprint(w, "secure-ok")
	}))
	defer upstream.Close()

	manager := testManager(t)
	router := NewRouter()
	router.Replace([]Route{
		{Host: "myproject.test", Target: strings.TrimPrefix(upstream.URL, "http://"), HTTPS: true, Port: 1},
		{Host: "*.myproject.test", Target: strings.TrimPrefix(upstream.URL, "http://"), HTTPS: true, Port: 1},
	})

	srv := NewServer(router, manager)
	httpPort := freePort(t)
	httpsPort := freePort(t)
	if err := srv.Serve(fmt.Sprintf("127.0.0.1:%d", httpPort), fmt.Sprintf("127.0.0.1:%d", httpsPort)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()

	pool := x509.NewCertPool()
	pool.AddCert(manager.CA().Certificate())
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "myproject.test"},
		},
	}

	req0, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/data", httpsPort), nil)
	req0.Host = "myproject.test"
	resp, err := client.Do(req0)
	if err != nil {
		t.Fatalf("TLS request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "secure-ok" {
		t.Errorf("body = %q", body)
	}
	if resp.Header.Get("X-Upstream") != "yes" {
		t.Error("upstream headers must pass through")
	}

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", httpsPort), nil)
	req.Host = "wild.myproject.test"
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("wildcard TLS request failed: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("wildcard status = %d", resp2.StatusCode)
	}
}

func TestUpstreamDownReturns502Page(t *testing.T) {
	manager := testManager(t)
	router := NewRouter()
	router.Replace([]Route{{Host: "dead.test", Target: "127.0.0.1:1", HTTPS: true, Port: 1}})

	srv := NewServer(router, manager)
	httpPort := freePort(t)
	httpsPort := freePort(t)
	if err := srv.Serve(fmt.Sprintf("127.0.0.1:%d", httpPort), fmt.Sprintf("127.0.0.1:%d", httpsPort)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()

	pool := x509.NewCertPool()
	pool.AddCert(manager.CA().Certificate())
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", httpsPort), nil)
	req.Host = "dead.test"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request to dead upstream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("want 502, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Dev server not responding") {
		t.Errorf("branded 502 page missing: %q", body)
	}
}

func TestLandingAndNotFoundPages(t *testing.T) {
	router := NewRouter()
	srv := NewServer(router, testManager(t))
	httpPort := freePort(t)
	httpsPort := freePort(t)
	if err := srv.Serve(fmt.Sprintf("127.0.0.1:%d", httpPort), fmt.Sprintf("127.0.0.1:%d", httpsPort)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", httpPort), nil)
	req.Host = "unlinked.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "is running") {
		t.Errorf("landing page wrong: %d %q", resp.StatusCode, body)
	}
}
