package api

import (
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/SDK-E/dnser/web"
)

func newListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func slogDebug(msg, addr string, err error) {
	slog.Debug(msg, "addr", addr, "err", err)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		writeLandingFallback(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(dist, path); err != nil {
		path = "index.html"
		if _, err := fs.Stat(dist, path); err != nil {
			writeHTMLFallback(w, r)
			return
		}
		r.URL.Path = "/"
	}
	http.FileServerFS(dist).ServeHTTP(w, r)
}

func writeLandingFallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(fallbackPage))
}

func writeHTMLFallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(fallbackPage))
}

const fallbackPage = `<!doctype html><html><head><meta charset="utf-8"><title>DNSer</title>
<style>body{background:#082003;color:#e6f4e2;font-family:ui-sans-serif,system-ui;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}div{text-align:center}h1{font-size:28px}.g{color:#2cdb16}</style></head>
<body><div><h1><span class="g">DNS.</span>er</h1><p>Dashboard assets not built. Run <code>pnpm --dir web build</code>.</p></div></body></html>`
