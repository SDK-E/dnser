package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type apiStatus struct {
	Version   string   `json:"version"`
	TLD       string   `json:"tld"`
	DNSPort   int      `json:"dns_port"`
	Projects  int      `json:"projects"`
	Upstreams []string `json:"upstreams"`
}

func apiGet(t *testing.T, url string, target any) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp
}

func apiSend(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestE2E_APIAndDashboard(t *testing.T) {
	d := startDaemon(t, e2eProject())

	var st apiStatus
	apiGet(t, d.baseURL+"/api/v1/status", &st)
	if st.TLD != "test" || st.Projects != 1 || st.DNSPort != d.ports.DNS {
		t.Fatalf("status mismatch: %+v", st)
	}
	if len(st.Upstreams) == 0 || !strings.HasSuffix(st.Upstreams[0], fmt.Sprint(d.ports.Upstream)) {
		t.Errorf("upstreams not sandboxed: %v", st.Upstreams)
	}

	t.Run("dashboard html served", func(t *testing.T) {
		resp, err := http.Get(d.baseURL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode != 200 || !strings.Contains(string(body), "<html") {
			t.Errorf("index: %d %.120q", resp.StatusCode, body)
		}
	})

	t.Run("project CRUD via API drives DNS live", func(t *testing.T) {
		if code := apiSend(t, "POST", d.baseURL+"/api/v1/projects",
			`{"domain":"apitest","port":32777,"wildcard":true,"https":true}`).StatusCode; code != 201 {
			t.Fatalf("create status %d", code)
		}
		waitFor(t, func() bool {
			resp := queryDNS(t, d.ports.DNS, "anything.apitest.test", dns.TypeA)
			return len(resp.Answer) == 1
		}, 6*time.Second, "created project not resolving")

		if code := apiSend(t, "POST", d.baseURL+"/api/v1/projects/apitest.test/records",
			`{"type":"TXT","name":"k","value":"secret1"}`).StatusCode; code != 201 {
			t.Fatalf("add record status %d", code)
		}
		waitFor(t, func() bool {
			resp := queryDNS(t, d.ports.DNS, "k.apitest.test", dns.TypeTXT)
			return len(resp.Answer) == 1 && resp.Answer[0].(*dns.TXT).Txt[0] == "secret1"
		}, 6*time.Second, "record added via API never resolved")

		if code := apiSend(t, "DELETE", d.baseURL+"/api/v1/projects/apitest.test", "").StatusCode; code != 200 {
			t.Fatalf("delete status %d", code)
		}
		waitFor(t, func() bool {
			resp := queryDNS(t, d.ports.DNS, "anything.apitest.test", dns.TypeA)
			return resp.Rcode == dns.RcodeNameError || len(resp.Answer) == 0 && !resp.Authoritative
		}, 6*time.Second, "deleted project still resolving")
	})

	t.Run("query log stream delivers events", func(t *testing.T) {
		req, _ := http.NewRequest("GET", d.baseURL+"/api/v1/logs/stream", nil)
		ctx := context.Background()
		req = req.WithContext(ctx)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("sse connect: %v", err)
		}
		events := make(chan string, 4)
		go func() {
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					events <- strings.TrimPrefix(line, "data: ")
					return
				}
			}
		}()

		time.Sleep(300 * time.Millisecond)
		queryDNS(t, d.ports.DNS, "sse.apitest2.test", dns.TypeA)

		select {
		case ev := <-events:
			if !strings.Contains(ev, "sse.apitest2.test") {
				t.Errorf("unexpected event: %s", ev)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("SSE event never arrived")
		}
		_ = resp.Body.Close()
	})
}
