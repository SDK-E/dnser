package proxyd

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SDK-E/dnser/internal/certs"
)

type Route struct {
	Host       string
	Backends   []string
	HTTPS      bool
	ForceHTTPS bool
	TCP        bool
	Listen     int
}

type Router struct {
	mu     sync.RWMutex
	routes map[string]Route
	rr     map[string]*atomic.Uint64
}

func NewRouter() *Router {
	return &Router{routes: make(map[string]Route), rr: make(map[string]*atomic.Uint64)}
}

func (r *Router) Replace(routes []Route) {
	table := make(map[string]Route, len(routes))
	counters := make(map[string]*atomic.Uint64, len(routes))
	for _, rt := range routes {
		key := strings.ToLower(rt.Host)
		table[key] = rt
		counters[key] = new(atomic.Uint64)
		if old, ok := r.rr[key]; ok {
			counters[key] = old
		}
	}
	r.mu.Lock()
	r.routes = table
	r.rr = counters
	r.mu.Unlock()
}

func (r *Router) Pick(host string, backends []string) (string, bool) {
	n := len(backends)
	if n == 0 {
		return "", false
	}
	if n == 1 {
		return backends[0], true
	}
	r.mu.RLock()
	counter := r.rr[strings.ToLower(host)]
	r.mu.RUnlock()
	if counter == nil {
		return backends[0], true
	}
	return backends[int(counter.Add(1))%n], true
}

func (r *Router) Lookup(host string) (Route, bool) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rt, ok := r.routes[host]; ok {
		return rt, true
	}
	parts := strings.Split(host, ".")
	for i := 1; i < len(parts); i++ {
		wild := "*." + strings.Join(parts[i:], ".")
		if rt, ok := r.routes[wild]; ok {
			return rt, true
		}
	}
	return Route{}, false
}

func (r *Router) Backends() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.routes))
	for _, rt := range r.routes {
		for _, b := range rt.Backends {
			if rt.TCP {
				continue
			}
			out[b] = "http://" + b
		}
	}
	return out
}

func (r *Router) DialBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, rt := range r.routes {
		if !rt.TCP {
			continue
		}
		for _, b := range rt.Backends {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
	}
	return out
}

func (r *Router) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Route, 0, len(r.routes))
	for _, rt := range r.routes {
		out = append(out, rt)
	}
	return out
}

type Server struct {
	router    *Router
	certs     *certs.Manager
	http      *http.Server
	https     *http.Server
	mu        sync.Mutex
	httpAddr  string
	httpsAddr string

	healthMu sync.RWMutex
	healthy  func(backend string) bool
}

func (s *Server) SetHealthFunc(fn func(backend string) bool) {
	s.healthMu.Lock()
	s.healthy = fn
	s.healthMu.Unlock()
}

func (s *Server) isHealthy(backend string) bool {
	s.healthMu.RLock()
	fn := s.healthy
	s.healthMu.RUnlock()
	if fn == nil {
		return true
	}
	return fn(backend)
}

func (s *Server) HTTPAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpAddr
}

func (s *Server) HTTPSAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpsAddr
}

func NewServer(router *Router, manager *certs.Manager) *Server {
	s := &Server{router: router, certs: manager}

	s.http = &http.Server{
		Handler:           http.HandlerFunc(s.handleHTTP),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.https = &http.Server{
		Handler:           http.HandlerFunc(s.handleHTTPS),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         s.certs.TLSConfig(),
	}
	return s
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	rt, ok := s.router.Lookup(r.Host)
	switch {
	case ok && rt.ForceHTTPS && rt.HTTPS:
		http.Redirect(w, r, "https://"+hostOnly(r.Host)+r.URL.RequestURI(), http.StatusPermanentRedirect)
	case ok:
		s.proxy(w, r, rt)
	default:
		writeLanding(w, r)
	}
}

func (s *Server) handleHTTPS(w http.ResponseWriter, r *http.Request) {
	rt, ok := s.router.Lookup(r.Host)
	if !ok {
		writeNotFound(w, r)
		return
	}
	s.proxy(w, r, rt)
}

func (s *Server) candidates(rt Route) []string {
	healthy := make([]string, 0, len(rt.Backends))
	for _, b := range rt.Backends {
		if s.isHealthy(b) {
			healthy = append(healthy, b)
		}
	}
	if len(healthy) == 0 {
		return rt.Backends
	}
	return healthy
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, rt Route) {
	tried := map[string]bool{}
	for range s.candidates(rt) {
		backend, ok := s.router.Pick(rt.Host, s.candidates(rt))
		if !ok || tried[backend] {
			continue
		}
		tried[backend] = true
		target := &url.URL{Scheme: "http", Host: backend}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.Out.Host = pr.In.Host
				pr.SetXForwarded()
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Debug("upstream unavailable", "host", rt.Host, "backend", backend, "err", err)
				writeUpstreamDown(w, r, rt)
			},
			FlushInterval: 100 * time.Millisecond,
		}
		proxy.ServeHTTP(w, r)
		return
	}
	writeUpstreamDown(w, r, rt)
}

func (s *Server) Serve(bindHTTP, bindHTTPS string) error {
	httpLn, httpNote := listenWithFallback(bindHTTP)
	if httpLn == nil {
		return fmt.Errorf("bind http %s: no available port (tried preferred and fallback)", bindHTTP)
	}
	httpsLn, httpsNote := listenTLSWithFallback(bindHTTPS, s.https.TLSConfig)
	if httpsLn == nil {
		_ = httpLn.Close()
		return fmt.Errorf("bind https %s: no available port (tried preferred and fallback)", bindHTTPS)
	}
	if httpNote != "" {
		slog.Warn(httpNote)
	}
	if httpsNote != "" {
		slog.Warn(httpsNote)
	}

	s.mu.Lock()
	s.httpAddr = httpLn.Addr().String()
	s.httpsAddr = httpsLn.Addr().String()
	s.mu.Unlock()

	go func() {
		if err := s.http.Serve(httpLn); err != nil && err != http.ErrServerClosed {
			slog.Debug("http listener stopped", "addr", bindHTTP, "err", err)
		}
	}()
	go func() {
		if err := s.https.Serve(httpsLn); err != nil && err != http.ErrServerClosed {
			slog.Debug("https listener stopped", "addr", bindHTTPS, "err", err)
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	errHTTPS := s.https.Shutdown(ctx)
	errHTTP := s.http.Shutdown(ctx)
	if errHTTPS != nil {
		return errHTTPS
	}
	return errHTTP
}

func hostOnly(host string) string {
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		return host[:i]
	}
	return host
}

func listenWithFallback(addr string) (net.Listener, string) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, ""
	}
	fallback := swapPort(addr, "8080")
	if fallback == addr {
		return nil, ""
	}
	ln2, err2 := net.Listen("tcp", fallback)
	if err2 != nil {
		return nil, ""
	}
	return ln2, fmt.Sprintf("port %s unavailable; HTTP proxy falling back to %s", addr, fallback)
}

func listenTLSWithFallback(addr string, cfg *tls.Config) (net.Listener, string) {
	ln, err := tls.Listen("tcp", addr, cfg)
	if err == nil {
		return ln, ""
	}
	fallback := swapPort(addr, "8443")
	if fallback == addr {
		return nil, ""
	}
	ln2, err2 := tls.Listen("tcp", fallback, cfg)
	if err2 != nil {
		return nil, ""
	}
	return ln2, fmt.Sprintf("port %s unavailable; HTTPS proxy falling back to %s", addr, fallback)
}

func swapPort(addr, newPort string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return net.JoinHostPort(host, newPort)
}
