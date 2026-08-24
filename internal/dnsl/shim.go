package dnsl

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const loopbackIP = "127.0.0.1"

type Answer struct {
	Name  string
	Type  string
	Value string
}

type Shim struct {
	mu           sync.RWMutex
	table        map[string]Answer
	suffixes     []string
	upstreamAddr string
	servers      []*dns.Server
	port         int
}

func NewShim(answers []Answer, suffixes []string, upstream string) *Shim {
	return &Shim{
		table:        answerTable(answers),
		suffixes:     normalizeSuffixes(suffixes),
		upstreamAddr: normalizeUpstream(upstream),
	}
}

func normalizeUpstream(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, "53")
	}
	return addr
}

func answerTable(answers []Answer) map[string]Answer {
	t := make(map[string]Answer, len(answers))
	for _, a := range answers {
		key := strings.ToLower(a.Name) + "|" + strings.ToUpper(a.Type)
		t[key] = a
	}
	return t
}

func normalizeSuffixes(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "*."), "."))
		if s != "" && !strings.HasPrefix(s, ".") && !strings.HasSuffix(s, ".") {
			out = append(out, s)
		}
	}
	return out
}

func (s *Shim) SetAnswers(answers []Answer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.table = answerTable(answers)
}

func (s *Shim) Start(preferred int) error {
	handler := dns.HandlerFunc(s.handle)
	addr := net.JoinHostPort(loopbackIP, strconv.Itoa(preferred))
	udp := &dns.Server{Addr: addr, Net: "udp", Handler: handler, ReadTimeout: readTimeout, WriteTimeout: writeTimeout}
	tcp := &dns.Server{Addr: addr, Net: "tcp", Handler: handler, ReadTimeout: readTimeout, WriteTimeout: writeTimeout}
	udpErr := make(chan error, 1)
	go func() { udpErr <- udp.ListenAndServe() }()
	if !awaitReady(func() bool { return udp.PacketConn != nil }, udpErr, "shim udp bind "+addr) {
		return fmt.Errorf("shim udp bind %s failed", addr)
	}
	tcpErr := make(chan error, 1)
	go func() { tcpErr <- tcp.ListenAndServe() }()
	if !awaitReady(func() bool { return tcp.Listener != nil }, tcpErr, "shim tcp bind "+addr) {
		_ = udp.Shutdown()
		return fmt.Errorf("shim tcp bind %s failed", addr)
	}
	s.servers = []*dns.Server{udp, tcp}
	if ap, ok := udp.PacketConn.LocalAddr().(*net.UDPAddr); ok {
		s.port = ap.Port
	}
	return nil
}

func awaitReady(ready func() bool, errCh chan error, what string) bool {
	deadline := time.Now().Add(StartTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			_ = err
			return false
		default:
		}
		if ready() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (s *Shim) Port() int {
	return s.port
}

func (s *Shim) Stop() error {
	var errs []error
	for _, srv := range s.servers {
		if err := srv.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	s.servers = nil
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

const HealthName = "probe.dnser.internal"

func (s *Shim) handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Compress = true

	q := req.Question[0]
	if strings.EqualFold(strings.TrimSuffix(q.Name, "."), HealthName) && q.Qtype == dns.TypeA {
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: rrHeader(q.Name, dns.TypeA),
			A:   net.ParseIP(loopbackIP).To4(),
		})
		_ = w.WriteMsg(resp)
		return
	}
	answer, ok := s.lookup(q)
	if ok {
		if err := appendRR(resp, q, answer); err != nil {
			resp.SetRcode(req, dns.RcodeServerFailure)
		}
	} else if underAnySuffix(q.Name, s.suffixes) {
		if err := s.forward(resp, req); err != nil {
			resp.SetRcode(req, dns.RcodeServerFailure)
		}
	} else {
		resp.SetRcode(req, dns.RcodeRefused)
	}
	_ = w.WriteMsg(resp)
}

func (s *Shim) lookup(q dns.Question) (Answer, bool) {
	name := strings.ToLower(strings.TrimSuffix(q.Name, "."))
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.table[name+"|"+dns.TypeToString[q.Qtype]]
	return a, ok
}

func underAnySuffix(name string, suffixes []string) bool {
	n := strings.ToLower(strings.TrimSuffix(name, "."))
	for _, sfx := range suffixes {
		if n == sfx || strings.HasSuffix(n, "."+sfx) {
			return true
		}
	}
	return false
}

func appendRR(resp *dns.Msg, q dns.Question, a Answer) error {
	switch q.Qtype {
	case dns.TypeA:
		ip := net.ParseIP(a.Value)
		if ip == nil {
			return fmt.Errorf("invalid A value %q for %s", a.Value, a.Name)
		}
		v4 := ip.To4()
		if v4 == nil {
			return fmt.Errorf("%q is not an IPv4 address", a.Value)
		}
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: rrHeader(q.Name, dns.TypeA),
			A:   v4,
		})
	case dns.TypeAAAA:
		ip := net.ParseIP(a.Value)
		if ip == nil {
			return fmt.Errorf("invalid AAAA value %q for %s", a.Value, a.Name)
		}
		resp.Answer = append(resp.Answer, &dns.AAAA{
			Hdr:  rrHeader(q.Name, dns.TypeAAAA),
			AAAA: ip.To16(),
		})
	case dns.TypeTXT:
		resp.Answer = append(resp.Answer, &dns.TXT{
			Hdr: rrHeader(q.Name, dns.TypeTXT),
			Txt: []string{a.Value},
		})
	case dns.TypeCNAME:
		resp.Answer = append(resp.Answer, &dns.CNAME{
			Hdr:    rrHeader(q.Name, dns.TypeCNAME),
			Target: dns.Fqdn(a.Value),
		})
	default:
		return fmt.Errorf("unsupported answer type %q", a.Type)
	}
	return nil
}

func rrHeader(name string, t uint16) dns.RR_Header {
	return dns.RR_Header{
		Name:   dns.Fqdn(name),
		Rrtype: t,
		Class:  dns.ClassINET,
		Ttl:    60,
	}
}

func (s *Shim) forward(resp, req *dns.Msg) error {
	if s.upstreamAddr == "" {
		return fmt.Errorf("no upstream configured")
	}
	client := &dns.Client{Net: "udp", Timeout: forwardTimeout}
	in, _, err := client.Exchange(req.Copy(), s.upstreamAddr)
	if err != nil {
		return fmt.Errorf("forward %s: %w", req.Question[0].Name, err)
	}
	if in == nil {
		return fmt.Errorf("forward %s: nil response", req.Question[0].Name)
	}
	*resp = *in
	resp.Compress = true
	return nil
}
