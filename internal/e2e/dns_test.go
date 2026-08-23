package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

func e2eProject() config.Project {
	return config.Project{
		Domain: "myproject.test",
		Routes: []config.Route{
			{Host: "@", Backends: []string{"localhost:32000"}, HTTPS: true},
			{Host: "*", Backends: []string{"localhost:32000"}, HTTPS: true},
			{Host: "myproject.dev.test", Backends: []string{"localhost:32000"}, HTTPS: true},
		},
		Records: []config.Record{
			{Type: "TXT", Name: "_verify", Value: "token-abc"},
			{Type: "A", Name: "static", Value: "10.10.0.5"},
			{Type: "AAAA", Name: "v6", Value: "::1"},
			{Type: "CNAME", Name: "docs", Value: "example.com"},
			{Type: "MX", Name: "@", Value: "mail.myproject.test", Priority: 10},
			{Type: "SRV", Name: "_sip._tcp", Value: "sip.myproject.test", Priority: 1, Weight: 5, Port: 5060},
			{Type: "NS", Name: "delegated", Value: "ns1.other.test"},
		},
	}
}

func TestE2E_DNSLifecycle(t *testing.T) {
	d := startDaemon(t, e2eProject())

	t.Run("apex implicit A", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "myproject.test", dns.TypeA)
		if !resp.Authoritative || len(resp.Answer) != 1 {
			t.Fatalf("auth=%v answers=%v", resp.Authoritative, resp.Answer)
		}
		if got := resp.Answer[0].(*dns.A).A.String(); got != "127.0.0.1" {
			t.Errorf("apex = %s", got)
		}
	})

	t.Run("deep wildcard", func(t *testing.T) {
		for _, name := range []string{"api.myproject.test", "a.b.c.myproject.test"} {
			resp := queryDNS(t, d.ports.DNS, name, dns.TypeA)
			if len(resp.Answer) != 1 || resp.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
				t.Errorf("%s: %v", name, resp.Answer)
			}
		}
	})

	t.Run("explicit record wins over wildcard", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "static.myproject.test", dns.TypeA)
		if len(resp.Answer) != 1 || resp.Answer[0].(*dns.A).A.String() != "10.10.0.5" {
			t.Errorf("static: %v", resp.Answer)
		}
	})

	t.Run("record types", func(t *testing.T) {
		cases := []struct {
			name  string
			qtype uint16
			check func(dns.RR) bool
		}{
			{"_verify.myproject.test", dns.TypeTXT, func(rr dns.RR) bool { return rr.(*dns.TXT).Txt[0] == "token-abc" }},
			{"v6.myproject.test", dns.TypeAAAA, func(rr dns.RR) bool { return rr.(*dns.AAAA).AAAA.String() == "::1" }},
			{"docs.myproject.test", dns.TypeCNAME, func(rr dns.RR) bool { return rr.(*dns.CNAME).Target == "example.com." }},
			{"myproject.test", dns.TypeMX, func(rr dns.RR) bool { return rr.(*dns.MX).Preference == 10 }},
			{"_sip._tcp.myproject.test", dns.TypeSRV, func(rr dns.RR) bool { return rr.(*dns.SRV).Port == 5060 }},
			{"delegated.myproject.test", dns.TypeNS, func(rr dns.RR) bool { return rr.(*dns.NS).Ns == "ns1.other.test." }},
		}
		for _, c := range cases {
			resp := queryDNS(t, d.ports.DNS, c.name, c.qtype)
			if len(resp.Answer) == 0 || !c.check(resp.Answer[0]) {
				t.Errorf("%s/%s: %v", c.name, dns.TypeToString[c.qtype], resp.Answer)
			}
		}
	})

	t.Run("alias zone", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "myproject.dev.test", dns.TypeA)
		if len(resp.Answer) != 1 {
			t.Errorf("alias: %v", resp.Answer)
		}
	})

	t.Run("dashboard domain always resolves", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "dnser.test", dns.TypeA)
		if len(resp.Answer) != 1 {
			t.Errorf("dashboard: %v", resp.Answer)
		}
	})

	t.Run("unknown forwards to upstream", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "host1.forward.test", dns.TypeA)
		if resp.Authoritative || len(resp.Answer) != 1 || resp.Answer[0].(*dns.A).A.String() != "203.0.113.77" {
			t.Fatalf("forwarded: auth=%v answers=%v", resp.Authoritative, resp.Answer)
		}
	})

	t.Run("upstream NXDOMAIN passes through", func(t *testing.T) {
		resp := queryDNS(t, d.ports.DNS, "missing.forward.test", dns.TypeA)
		if resp.Rcode != dns.RcodeNameError {
			t.Fatalf("want NXDOMAIN got %s (%v)", dns.RcodeToString[resp.Rcode], resp.Answer)
		}
	})

	t.Run("cache serves second query consistently", func(t *testing.T) {
		first := queryDNS(t, d.ports.DNS, "cached.forward.test", dns.TypeA)
		second := queryDNS(t, d.ports.DNS, "cached.forward.test", dns.TypeA)
		if len(first.Answer) != 1 || len(second.Answer) != 1 {
			t.Fatalf("answers: %v / %v", first.Answer, second.Answer)
		}
		if first.Answer[0].String() != second.Answer[0].String() {
			t.Errorf("inconsistent: %v vs %v", first.Answer[0], second.Answer[0])
		}
	})

	t.Run("mixed case and trailing dot", func(t *testing.T) {
		client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
		req := new(dns.Msg)
		req.SetQuestion("API.MYPROJECT.TEST.", dns.TypeA)
		resp, _, err := client.Exchange(req, fmt.Sprintf("127.0.0.1:%d", d.ports.DNS))
		if err != nil || len(resp.Answer) != 1 {
			t.Errorf("mixed case: err=%v answers=%v", err, resp.Answer)
		}
	})
}

func TestE2E_HotReload(t *testing.T) {
	d := startDaemon(t, e2eProject())

	if _, ok := lookupAddr(t, d, "hot.second.test"); ok {
		t.Fatal("should not resolve before link")
	}

	appendProjectFile(t, d, config.Project{Domain: "second.test", Routes: []config.Route{{Host: "@", Backends: []string{"localhost:32100"}, HTTPS: true}, {Host: "*", Backends: []string{"localhost:32100"}, HTTPS: true}}})

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if addr, ok := lookupAddr(t, d, "hot.second.test"); ok && addr == "127.0.0.1" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("hot reload never picked up second.test")
}

func lookupAddr(t *testing.T, d *daemon, host string) (string, bool) {
	t.Helper()
	resp := queryDNS(t, d.ports.DNS, host, dns.TypeA)
	if len(resp.Answer) == 0 {
		return "", false
	}
	if a, ok := resp.Answer[0].(*dns.A); ok {
		return a.A.String(), true
	}
	return "", false
}

func appendProjectFile(t *testing.T, d *daemon, p config.Project) {
	t.Helper()
	path := filepath.Join(d.home, "dnser.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	pb, _ := json.Marshal(p)
	var proj map[string]any
	if err := json.Unmarshal(pb, &proj); err != nil {
		t.Fatal(err)
	}
	projects, _ := doc["projects"].([]any)
	replaced := false
	for i, item := range projects {
		m, _ := item.(map[string]any)
		if m["domain"] == p.Domain {
			projects[i] = proj
			replaced = true
		}
	}
	if !replaced {
		doc["projects"] = append(projects, proj)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
