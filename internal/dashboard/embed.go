package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"embed"
)

//go:embed webapp/dist
var distFS embed.FS

func serveAsset(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	sub, err := fs.Sub(distFS, "webapp/dist")
	if err != nil {
		http.Error(w, "embed broken", 500)
		return
	}
	data, err := fs.ReadFile(sub, path)
	if err != nil {
		data, err = fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		path = "index.html"
	}
	switch {
	case strings.HasSuffix(path, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "text/javascript")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(path, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	_, _ = w.Write(data)
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("entropy: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func Serve(ctx context.Context, addr string, deps Deps) error {
	tok, err := EnsureToken()
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: addr, Handler: Handler(deps, tok)}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), shutTimeout)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}
