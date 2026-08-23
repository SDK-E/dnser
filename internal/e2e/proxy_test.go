package e2e

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SDK-E/dnser/internal/config"
)

func startEchoUpstream(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-E2E", "yes")
		fmt.Fprintf(w, "%s|%s|%s|%s", body, r.Header.Get("X-Forwarded-Proto"), r.Host, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().String()
	return strings.TrimPrefix(port, "http://")
}

func TestE2E_ProxyHTTPAndHTTPS(t *testing.T) {
	upstream := startEchoUpstream(t, "e2e-ok")
	target := upstream
	p := e2eProject()
	backend := fmt.Sprintf("localhost:%d", portOf(target))
	for i := range p.Routes {
		p.Routes[i].Backends = []string{backend}
	}

	d := startDaemon(t, p)
	client := tlsClient(t, d)

	t.Run("https apex proxies to upstream", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", d.ports.HTTPS), nil)
		req.Host = "myproject.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("https request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "e2e-ok|https|myproject.test|/") {
			t.Fatalf("status=%d body=%q", resp.StatusCode, body)
		}
		if resp.Header.Get("X-E2E") != "yes" {
			t.Error("upstream headers must pass through")
		}
	})

	t.Run("https wildcard subdomain", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/deep/path?q=1", d.ports.HTTPS), nil)
		req.Host = "pr.preview.myproject.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("wildcard https: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "/deep/path") {
			t.Errorf("path not proxied: %q", body)
		}
	})

	t.Run("plain http also serves project (dual scheme)", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/plain", d.ports.HTTP), nil)
		req.Host = "myproject.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("http request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || !strings.HasPrefix(string(body), "e2e-ok|http|myproject.test") {
			t.Fatalf("status=%d body=%q", resp.StatusCode, body)
		}
	})

	t.Run("unlinked host gets landing page", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", d.ports.HTTP), nil)
		req.Host = "nobody.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 404 || !strings.Contains(string(body), "DNS.") {
			t.Errorf("landing: %d %q", resp.StatusCode, body[:min(len(body), 120)])
		}
	})

	t.Run("linked host with dead upstream returns branded 502 over TLS", func(t *testing.T) {
		appendProjectFile(t, d, config.Project{Domain: "dead.test", Routes: []config.Route{{Host: "@", Backends: []string{"localhost:32999"}, HTTPS: true}}})

		var resp *http.Response
		var err error
		waitFor(t, func() bool {
			req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/", d.ports.HTTPS), nil)
			req.Host = "dead.test"
			resp, err = client.Do(req)
			if err != nil {
				return false
			}
			return resp.StatusCode == http.StatusBadGateway
		}, 8*time.Second, "hot-reloaded dead.test never returned branded 502")
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 502 || !strings.Contains(string(body), "Dev server not responding") {
			t.Errorf("502 page: %d %.160q", resp.StatusCode, body)
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func portOf(hostPort string) int {
	idx := strings.LastIndex(hostPort, ":")
	if idx < 0 {
		return 0
	}
	n := 0
	for _, c := range hostPort[idx+1:] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal(msg)
}
