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
	"time"

	"github.com/SDK-E/dnser/internal/certs"
)

type Route struct {
	Host   string
	Target string
	HTTPS  bool
	Port   int
}

type Router struct {
	mu     sync.RWMutex
	routes map[string]Route
}

func NewRouter() *Router {
	return &Router{routes: make(map[string]Route)}
}

func (r *Router) Replace(routes []Route) {
	table := make(map[string]Route, len(routes))
	for _, rt := range routes {
		table[strings.ToLower(rt.Host)] = rt
	}
	r.mu.Lock()
	r.routes = table
	r.mu.Unlock()
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

func (r *Router) Targets() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.routes))
	for host, rt := range r.routes {
		if rt.Port > 0 {
			out[host] = fmt.Sprintf("http://%s", rt.Target)
		}
	}
	return out
}

type Server struct {
	router *Router
	certs  *certs.Manager
	http   *http.Server
	https  *http.Server
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
	case ok && !rt.HTTPS:
		s.proxy(w, r, rt)
	case ok && rt.HTTPS:
		http.Redirect(w, r, "https://"+hostOnly(r.Host)+r.URL.RequestURI(), http.StatusPermanentRedirect)
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

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, rt Route) {
	target := &url.URL{Scheme: "http", Host: rt.Target}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Debug("upstream unavailable", "host", rt.Host, "target", rt.Target, "err", err)
			writeUpstreamDown(w, r, rt)
		},
		FlushInterval: 100 * time.Millisecond,
	}
	proxy.ServeHTTP(w, r)
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
