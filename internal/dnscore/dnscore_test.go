package dnscore

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/logstream"
)

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Projects = []config.Project{
		{
			Domain:   "myproject.test",
			Port:     3000,
			Wildcard: true,
			HTTPS:    true,
			Aliases:  []string{"myproject.dev"},
			Records: []config.Record{
				{Type: "TXT", Name: "_verify", Value: "token-123"},
				{Type: "A", Name: "static", Value: "10.0.0.5"},
				{Type: "CNAME", Name: "docs", Value: "example.com"},
				{Type: "MX", Name: "@", Value: "mail.myproject.test", Priority: 10},
				{Type: "SRV", Name: "_sip._tcp", Value: "sip.myproject.test", Priority: 1, Weight: 5, Port: 5060},
				{Type: "AAAA", Name: "v6", Value: "::1"},
				{Type: "NS", Name: "delegated", Value: "ns1.other.test"},
			},
		},
		{
			Domain: "nowild.test",
			Port:   8080,
		},
	}
	return cfg
}

func TestEngineExactAndAlias(t *testing.T) {
	e := NewEngine(testConfig())

	rrs, owned := e.Resolve("myproject.test", dns.TypeA)
	if !owned || len(rrs) != 1 {
		t.Fatalf("apex A: owned=%v rrs=%v", owned, rrs)
	}
	if got := rrs[0].(*dns.A).A.String(); got != "127.0.0.1" {
		t.Errorf("apex A = %s, want 127.0.0.1", got)
	}

	rrs, owned = e.Resolve("myproject.dev", dns.TypeA)
	if !owned || len(rrs) != 1 {
		t.Fatalf("alias A: owned=%v rrs=%v", owned, rrs)
	}
}

func TestEngineWildcard(t *testing.T) {
	e := NewEngine(testConfig())
	for _, name := range []string{"api.myproject.test", "a.b.c.myproject.test"} {
		rrs, owned := e.Resolve(name, dns.TypeA)
		if !owned || len(rrs) == 0 {
			t.Fatalf("wildcard %s: owned=%v rrs=%v", name, owned, rrs)
		}
	}
}

func TestEngineExplicitRecordPrecedence(t *testing.T) {
	e := NewEngine(testConfig())
	rrs, _ := e.Resolve("static.myproject.test", dns.TypeA)
	if len(rrs) != 1 || rrs[0].(*dns.A).A.String() != "10.0.0.5" {
		t.Fatalf("explicit A should win: %v", rrs)
	}
	rrs, _ = e.Resolve("_verify.myproject.test", dns.TypeTXT)
	if len(rrs) != 1 || rrs[0].(*dns.TXT).Txt[0] != "token-123" {
		t.Fatalf("TXT record mismatch: %v", rrs)
	}
}

func TestEngineRecordTypes(t *testing.T) {
	e := NewEngine(testConfig())
	cases := []struct {
		name  string
		qtype uint16
		check func(dns.RR) bool
	}{
		{"docs.myproject.test", dns.TypeCNAME, func(rr dns.RR) bool { return rr.(*dns.CNAME).Target == "example.com." }},
		{"myproject.test", dns.TypeMX, func(rr dns.RR) bool { return rr.(*dns.MX).Preference == 10 }},
		{"_sip._tcp.myproject.test", dns.TypeSRV, func(rr dns.RR) bool { return rr.(*dns.SRV).Port == 5060 && rr.(*dns.SRV).Weight == 5 }},
		{"v6.myproject.test", dns.TypeAAAA, func(rr dns.RR) bool { return rr.(*dns.AAAA).AAAA.String() == "::1" }},
		{"delegated.myproject.test", dns.TypeNS, func(rr dns.RR) bool { return rr.(*dns.NS).Ns == "ns1.other.test." }},
	}
	for _, c := range cases {
		rrs, owned := e.Resolve(c.name, c.qtype)
		if !owned || len(rrs) == 0 || !c.check(rrs[0]) {
			t.Errorf("%s type %d: owned=%v rrs=%v", c.name, c.qtype, owned, rrs)
		}
	}
}

func TestEngineDashboardZone(t *testing.T) {
	e := NewEngine(testConfig())
	rrs, owned := e.Resolve("dnser.test", dns.TypeA)
	if !owned || len(rrs) != 1 {
		t.Fatalf("dashboard zone: owned=%v rrs=%v", owned, rrs)
	}
}

func TestEngineNonWildcardZoneForwards(t *testing.T) {
	e := NewEngine(testConfig())
	if _, owned := e.Resolve("unknown.nowild.test", dns.TypeA); owned {
		t.Error("unknown sub of non-wildcard zone must forward upstream")
	}
	if _, owned := e.Resolve("google.com", dns.TypeA); owned {
		t.Error("public domain must not be owned")
	}
	if _, owned := e.Resolve("", dns.TypeA); owned {
		t.Error("empty name must not be owned")
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.LocalAddr().(*net.UDPAddr).Port
	_ = ln.Close()
	return port
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func waitUDPReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	c := &dns.Client{Net: "udp", Timeout: 300 * time.Millisecond}
	req := new(dns.Msg)
	req.SetQuestion("ready.test.", dns.TypeA)
	for time.Now().Before(deadline) {
		if _, _, err := c.Exchange(req, addr); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("fake upstream at %s never became ready", addr)
}

func startFakeUpstream(t *testing.T) string {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", freeUDPPort(t))
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		q := req.Question[0]
		m := new(dns.Msg)
		m.SetReply(req)
		if strings.Contains(q.Name, "missing.") || q.Qtype != dns.TypeA {
			soa, _ := dns.NewRR(fmt.Sprintf("%s 30 IN SOA ns.%s host.%s 1 2 3 25 5", q.Name, q.Name, q.Name))
			m.Ns = append(m.Ns, soa)
			m.SetRcode(req, dns.RcodeNameError)
		} else {
			rr, _ := dns.NewRR(fmt.Sprintf("%s 60 IN A 93.184.216.34", q.Name))
			m.Answer = append(m.Answer, rr)
		}
		_ = w.WriteMsg(m)
	})
	srv := &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	waitUDPReady(t, addr)
	t.Cleanup(func() { _ = srv.Shutdown() })
	return addr
}

func TestForwarderRoundTrip(t *testing.T) {
	upstream := startFakeUpstream(t)
	fwd, err := NewForwarder([]string{upstream})
	if err != nil {
		t.Fatal(err)
	}
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	resp, err := fwd.Forward(QueryFor(req))
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if len(resp.Answer) != 1 || resp.Answer[0].(*dns.A).A.String() != "93.184.216.34" {
		t.Fatalf("unexpected answer %v", resp.Answer)
	}
}

func TestCacheHitExpirySingleFlight(t *testing.T) {
	c := NewCache()
	now := time.Now()
	c.now = func() time.Time { return now }

	resp := &dns.Msg{}
	resp.SetQuestion("cached.test.", dns.TypeA)
	rr, _ := dns.NewRR("cached.test. 60 IN A 1.2.3.4")
	resp.Answer = append(resp.Answer, rr)

	fetches := 0
	msg, err := c.DoSingleFlight("cached.test", dns.TypeA, func() (*dns.Msg, error) {
		fetches++
		c.Put("cached.test", dns.TypeA, resp)
		return resp.Copy(), nil
	})
	if err != nil || fetches != 1 || len(msg.Answer) != 1 {
		t.Fatalf("first fetch: fetches=%d err=%v", fetches, err)
	}
	if _, ok := c.Get("cached.test", dns.TypeA); !ok {
		t.Fatal("cache get failed")
	}
	now = now.Add(120 * time.Second)
	if _, ok := c.Get("cached.test", dns.TypeA); ok {
		t.Error("entry should be expired after TTL window")
	}
}

func TestCacheNegativeTTLFromSOA(t *testing.T) {
	c := NewCache()
	req := &dns.Msg{}
	req.SetQuestion("missing.test.", dns.TypeA)
	resp := req.Copy()
	resp.Rcode = dns.RcodeNameError
	soa, _ := dns.NewRR("missing.test. 30 IN SOA a.missing.test. b.missing.test. 1 2 3 25 5")
	resp.Ns = append(resp.Ns, soa)
	c.Put("missing.test", dns.TypeA, resp)
	if _, ok := c.Get("missing.test", dns.TypeA); !ok {
		t.Error("NXDOMAIN with SOA should be cached")
	}
	c.InvalidateAll()
	if _, ok := c.Get("missing.test", dns.TypeA); ok {
		t.Error("InvalidateAll should clear entries")
	}
}

func TestServerIntegrationLocalAndForward(t *testing.T) {
	cfg := testConfig()
	engine := NewEngine(cfg)
	upstream := startFakeUpstream(t)
	fwd, err := NewForwarder([]string{upstream})
	if err != nil {
		t.Fatal(err)
	}
	stream := logstream.New(64)
	srv, err := NewServer(Options{Engine: engine, Forward: fwd, Stream: stream})
	if err != nil {
		t.Fatal(err)
	}

	preferred := freeUDPPort(t)
	bindPort, err := PickPort("127.0.0.1", preferred, preferred+7, preferred+13)
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	if err := srv.ListenAndServe("127.0.0.1", bindPort); err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	serverAddr := fmt.Sprintf("127.0.0.1:%d", bindPort)

	localReq := new(dns.Msg)
	localReq.SetQuestion("api.myproject.test.", dns.TypeA)
	resp, _, err := client.Exchange(localReq, serverAddr)
	if err != nil {
		t.Fatalf("local query: %v", err)
	}
	if !resp.Authoritative || len(resp.Answer) != 1 {
		t.Fatalf("local response wrong: auth=%v answers=%v", resp.Authoritative, resp.Answer)
	}

	fwdReq := new(dns.Msg)
	fwdReq.SetQuestion("example.com.", dns.TypeA)
	resp, _, err = client.Exchange(fwdReq, serverAddr)
	if err != nil {
		t.Fatalf("forwarded query: %v", err)
	}
	if resp.Authoritative || len(resp.Answer) != 1 {
		t.Fatalf("forwarded response wrong: auth=%v answers=%v", resp.Authoritative, resp.Answer)
	}

	resp2, _, err := client.Exchange(fwdReq, serverAddr)
	if err != nil || len(resp2.Answer) != 1 {
		t.Fatalf("cached query failed: err=%v answers=%v", err, resp2.Answer)
	}

	nxReq := new(dns.Msg)
	nxReq.SetQuestion("totally-missing.invalid.", dns.TypeA)
	resp, _, err = client.Exchange(nxReq, serverAddr)
	if err != nil {
		t.Fatalf("nxdomain query: %v", err)
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("want NXDOMAIN, got %s", dns.RcodeToString[resp.Rcode])
	}

	deadline := time.Now().Add(2 * time.Second)
	var sawForward, sawCache, sawNXDomain bool
	for time.Now().Before(deadline) && (!sawForward || !sawCache || !sawNXDomain) {
		events := stream.Recent(stream.Len())
		for _, ev := range events {
			if ev.Source == logstream.SourceForward {
				sawForward = true
			}
			if ev.Source == logstream.SourceCache {
				sawCache = true
			}
			if ev.Answer == "NXDOMAIN" {
				sawNXDomain = true
			}
			if ev.Source == logstream.SourceLocal && ev.Name == "example.com" {
				t.Errorf("public domain answered locally")
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawForward || !sawCache || !sawNXDomain {
		t.Errorf("log stream incomplete: forward=%v cache=%v nxdomain=%v events=%d",
			sawForward, sawCache, sawNXDomain, stream.Len())
	}
}

func TestPickPortFallback(t *testing.T) {
	tcpPort := freeTCPPort(t)

	blockerLn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", tcpPort))
	if err != nil {
		t.Skipf("cannot occupy udp port %d: %v", tcpPort, err)
	}
	defer func() { _ = blockerLn.Close() }()

	port, err := PickPort("127.0.0.1", tcpPort, tcpPort+1)
	if err != nil {
		t.Fatalf("fallback pick failed: %v", err)
	}
	if port != tcpPort+1 {
		t.Errorf("picked %d, want %d", port, tcpPort+1)
	}
}
