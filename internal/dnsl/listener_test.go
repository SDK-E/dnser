package dnsl

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestListenerStartsOnFallbackPortAndProbes(t *testing.T) {
	fake := startFakeUpstream(t, map[string]string{"github.com.": "140.82.121.3"})
	l := New(Config{
		Upstreams:    []string{fake.addr},
		Suffixes:     []string{"test"},
		Answers:      []Answer{{Name: "app.test", Type: "A", Value: "127.0.0.1"}},
		PortChain:    []int{53, 5353, 35353},
		CacheEnabled: true,
	})
	ctx := context.Background()
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Stop(context.Background()) })

	if l.Port() == 53 {
		t.Fatalf("port 53 should be unbindable without privileges; chain must fall back")
	}
	if err := l.Probe(); err != nil {
		t.Fatalf("health probe: %v", err)
	}

	resp := query(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port())), "app.test", dns.TypeA)
	if answerA(resp) != "127.0.0.1" {
		t.Fatalf("split routing to shim broken: %+v", resp.Answer)
	}
	resp = query(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port())), "unknown.test", dns.TypeA)
	if resp.Rcode != dns.RcodeNameError && answerA(resp) == "" {
		t.Fatalf("unhandled suffix name must forward upstream through proxy, got rcode=%d", resp.Rcode)
	}
	resp = query(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port())), "github.com", dns.TypeA)
	if answerA(resp) != "140.82.121.3" {
		t.Fatalf("default upstream forwarding broken: %+v", resp.Answer)
	}
}

func TestListenerPortChainSkipsOccupiedPort(t *testing.T) {
	fake := startFakeUpstream(t, nil)
	occupied := freePort(t)
	blocker := &dns.Server{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(occupied)), Net: "udp", Handler: dns.HandlerFunc(func(dns.ResponseWriter, *dns.Msg) {})}
	errCh := make(chan error, 1)
	go func() { errCh <- blocker.ListenAndServe() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && blocker.PacketConn == nil {
		select {
		case err := <-errCh:
			t.Fatalf("blocker failed to bind %d: %v", occupied, err)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Cleanup(func() { _ = blocker.Shutdown() })

	fallback := freePort(t)
	l := New(Config{
		Upstreams: []string{fake.addr},
		Suffixes:  []string{"test"},
		PortChain: []int{occupied, fallback},
	})
	ctx := context.Background()
	defer func() { _ = l.Stop(ctx) }()
	if err := l.Start(ctx); err != nil {
		t.Fatalf("chain walk must skip occupied port: %v", err)
	}
	if l.Port() != fallback {
		t.Fatalf("expected fallback to %d, got %d", fallback, l.Port())
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	conn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	return conn.Addr().(*net.TCPAddr).Port
}

func TestListenerStopThenProbeFails(t *testing.T) {
	fake := startFakeUpstream(t, nil)
	l := New(Config{Upstreams: []string{fake.addr}, PortChain: []int{0}})
	ctx := context.Background()
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Probe(); err != nil {
		t.Fatalf("probe before stop: %v", err)
	}
	port := l.Port()
	if err := l.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	client := &dns.Client{Net: "udp", Timeout: 500 * time.Millisecond}
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(HealthName), dns.TypeA)
	if _, _, err := client.Exchange(q, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err == nil {
		t.Fatalf("probe after stop must fail (I1 precondition)")
	}
	if err := l.Probe(); err == nil {
		t.Fatalf("probe on stopped listener must error")
	}
}
