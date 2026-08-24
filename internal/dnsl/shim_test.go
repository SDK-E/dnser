package dnsl

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type fakeUpstream struct {
	addr string
	ips  map[string]string
}

func startFakeUpstream(t *testing.T, ips map[string]string) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{ips: ips}
	mux := dns.NewServeMux()
	mux.HandleFunc(".", f.handle)
	udp := &dns.Server{Addr: "127.0.0.1:0", Net: "udp", Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- udp.ListenAndServe() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("fake upstream bind: %v", err)
		default:
		}
		if udp.PacketConn != nil {
			ap := udp.PacketConn.LocalAddr().(*net.UDPAddr)
			f.addr = ap.String()
			t.Cleanup(func() { _ = udp.Shutdown() })
			return f
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("fake upstream never became ready")
	return nil
}

func (f *fakeUpstream) handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	q := req.Question[0]
	if ipStr, ok := f.ips[q.Name]; ok && q.Qtype == dns.TypeA {
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP(ipStr).To4(),
		})
	} else {
		resp.SetRcode(req, dns.RcodeNameError)
	}
	_ = w.WriteMsg(resp)
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	client := &dns.Client{Net: "udp", Timeout: probeTimeout}
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	resp, _, err := client.Exchange(msg, addr)
	if err != nil {
		t.Fatalf("query %s @ %s: %v", name, addr, err)
	}
	return resp
}

func answerA(resp *dns.Msg) string {
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func TestShimAnswersKnownRecords(t *testing.T) {
	shim := NewShim([]Answer{
		{Name: "app.test", Type: "A", Value: "127.0.0.1"},
		{Name: "app.test", Type: "TXT", Value: "v=verify"},
	}, []string{"test"}, "1.1.1.1")
	if err := shim.Start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shim.Stop() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(shim.Port()))

	resp := query(t, addr, "app.test", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || answerA(resp) != "127.0.0.1" {
		t.Fatalf("A answer wrong: %+v", resp.Answer)
	}
	txtResp := query(t, addr, "app.test", dns.TypeTXT)
	if txtResp.Rcode != dns.RcodeSuccess || len(txtResp.Answer) == 0 {
		t.Fatalf("TXT answer missing: %+v", txtResp.Answer)
	}
}

func TestShimForwardsUnknownUnderSuffixToUpstream(t *testing.T) {
	upstreamSrv := startFakeUpstream(t, map[string]string{"unknown.deep.test.": "10.9.8.7"})
	shim := NewShim([]Answer{{Name: "app.test", Type: "A", Value: "127.0.0.1"}},
		[]string{"test"}, upstreamSrv.addr)
	if err := shim.Start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shim.Stop() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(shim.Port()))

	resp := query(t, addr, "unknown.deep.test", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || answerA(resp) != "10.9.8.7" {
		t.Fatalf("unhandled name under suffix must forward upstream, got rcode=%d answers=%+v", resp.Rcode, resp.Answer)
	}
}

func TestShimRefusesOutsideSuffixes(t *testing.T) {
	startFakeUpstream(t, map[string]string{"real.example.com.": "1.2.3.4"})
	shim := NewShim(nil, []string{"test"}, "127.0.0.1:1")
	if err := shim.Start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shim.Stop() })
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(shim.Port()))

	resp := query(t, addr, "anything.example.com", dns.TypeA)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("names outside suffixes must be REFUSED by shim (proxy owns them), got %d", resp.Rcode)
	}
}
