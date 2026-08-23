package dnscore

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/logstream"
)

type Server struct {
	mu       sync.Mutex
	engine   *Engine
	forward  *Forwarder
	cache    *Cache
	stream   *logstream.Stream
	addr     string
	shutdown []func(context.Context) error
}

type Options struct {
	Engine  *Engine
	Forward *Forwarder
	Cache   *Cache
	Stream  *logstream.Stream
}

func NewServer(opts Options) (*Server, error) {
	if opts.Engine == nil {
		return nil, fmt.Errorf("dns server: engine required")
	}
	if opts.Forward == nil {
		return nil, fmt.Errorf("dns server: forwarder required")
	}
	if opts.Cache == nil {
		opts.Cache = NewCache()
	}
	return &Server{
		engine:  opts.Engine,
		forward: opts.Forward,
		cache:   opts.Cache,
		stream:  opts.Stream,
	}, nil
}

func (s *Server) UseEngine(e *Engine) {
	s.mu.Lock()
	s.engine = e
	s.mu.Unlock()
}

func (s *Server) activeEngine() *Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engine
}

func (s *Server) Cache() *Cache {
	return s.cache
}

func (s *Server) Handle(w dns.ResponseWriter, req *dns.Msg) {
	start := time.Now()
	if len(req.Question) == 0 {
		dns.HandleFailed(w, req)
		return
	}
	q := req.Question[0]
	name := strings.ToLower(strings.TrimSuffix(q.Name, "."))

	resp, src := s.answer(req, name, q.Qtype)
	if writeErr := w.WriteMsg(resp); writeErr != nil {
		slog.Debug("dns write failed", "name", name, "err", writeErr)
	}
	s.publish(name, dns.TypeToString[q.Qtype], resp, src, time.Since(start))
}

func (s *Server) answer(req *dns.Msg, name string, qtype uint16) (*dns.Msg, logstream.Source) {
	engine := s.activeEngine()
	if rrs, owned := engine.Resolve(name, qtype); owned {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Authoritative = true
		resp.Answer = append(resp.Answer, rrs...)
		return resp, logstream.SourceLocal
	}
	if qtype == dns.TypeANY || qclassInvalid(req) {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		return resp, logstream.SourceError
	}

	if cached, ok := s.cache.Get(name, qtype); ok {
		cached.Id = req.Id
		return cached, logstream.SourceCache
	}

	msg, err := s.cache.DoSingleFlight(name, qtype, func() (*dns.Msg, error) {
		upstreamResp, fwdErr := s.forward.Forward(QueryFor(req))
		if fwdErr != nil {
			return nil, fwdErr
		}
		s.cache.Put(name, qtype, upstreamResp)
		return upstreamResp.Copy(), nil
	})
	if err != nil || msg == nil {
		slog.Warn("upstream resolution failed", "name", name, "err", err)
		fail := new(dns.Msg)
		fail.SetRcode(req, dns.RcodeServerFailure)
		return fail, logstream.SourceError
	}
	msg.Id = req.Id
	msg.Compress = true
	return msg, logstream.SourceForward
}

func qclassInvalid(req *dns.Msg) bool {
	for _, q := range req.Question {
		if q.Qclass != dns.ClassINET && q.Qclass != dns.ClassANY {
			return true
		}
	}
	return false
}

func (s *Server) publish(name, qtype string, resp *dns.Msg, src logstream.Source, latency time.Duration) {
	if s.stream == nil {
		return
	}
	answer := AnswerSummary(resp)
	if resp.Rcode != dns.RcodeSuccess && len(resp.Answer) == 0 {
		if code, ok := dns.RcodeToString[resp.Rcode]; ok {
			answer = code
		}
	}
	s.stream.Publish(logstream.Event{
		Time:    time.Now(),
		Name:    name,
		Type:    qtype,
		Source:  src,
		Answer:  answer,
		Latency: latency,
	})
}

func (s *Server) ListenAndServe(bind string, port int) error {
	addr := fmt.Sprintf("%s:%d", bind, port)

	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("bind udp %s: %w", addr, err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("bind tcp %s: %w", addr, err)
	}

	udp := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(s.Handle), UDPSize: 1232}
	tcp := &dns.Server{Listener: ln, Handler: dns.HandlerFunc(s.Handle)}

	go func() {
		if err := udp.ActivateAndServe(); err != nil {
			slog.Debug("udp dns listener stopped", "addr", addr, "err", err)
		}
	}()
	go func() {
		if err := tcp.ActivateAndServe(); err != nil {
			slog.Debug("tcp dns listener stopped", "addr", addr, "err", err)
		}
	}()

	s.mu.Lock()
	s.addr = addr
	s.shutdown = []func(context.Context) error{
		func(context.Context) error { return udp.Shutdown() },
		func(context.Context) error { return tcp.Shutdown() },
	}
	s.mu.Unlock()
	return nil
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	fns := append([]func(context.Context) error(nil), s.shutdown...)
	s.shutdown = nil
	s.mu.Unlock()
	var lastErr error
	for _, fn := range fns {
		if err := fn(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func PickPort(bind string, preferred int, fallbacks ...int) (int, error) {
	try := func(port int) bool {
		pc, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", bind, port))
		if err != nil {
			return false
		}
		_ = pc.Close()
		return true
	}
	tried := []int{preferred}
	if try(preferred) {
		return preferred, nil
	}
	for _, fallback := range fallbacks {
		if fallback <= 0 || fallback == preferred {
			continue
		}
		tried = append(tried, fallback)
		if try(fallback) {
			return fallback, nil
		}
	}
	return 0, fmt.Errorf("no available DNS port on %s (tried %v)", bind, tried)
}

func ProbeLocal(bind string, port int, name string) error {
	if name == "" {
		name = "dnser.test"
	}
	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := client.Exchange(req, fmt.Sprintf("%s:%d", bind, port))
	if err != nil {
		return fmt.Errorf("probe %s:%d: %w", bind, port, err)
	}
	if !resp.Authoritative {
		return fmt.Errorf("probe %s:%d: response not authoritative", bind, port)
	}
	return nil
}

func PickFreeTCPPort(bind string, preferred int) (int, bool) {
	try := func(port int) bool {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bind, port))
		if err != nil {
			return false
		}
		_ = l.Close()
		return true
	}
	if preferred > 0 && try(preferred) {
		return preferred, true
	}
	l, err := net.Listen("tcp", fmt.Sprintf("%s:0", bind))
	if err != nil {
		return 0, false
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, false
	}
	return addr.Port, true
}
