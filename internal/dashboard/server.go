package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SDK-E/dnser/internal/state"
)

type Issue struct {
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix"`
}

type Deps struct {
	State         *state.Store
	ListIssues    func(ctx context.Context) []Issue
	LogLines      func(ctx context.Context, project string, tail int) ([]string, error)
	DNSPort       func() int
	SupervisorRun bool
}

func tokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".dnser", "dashboard-token"), nil
}

func EnsureToken() (string, error) {
	path, err := tokenPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create ~/.dnser: %w", err)
	}
	if data, rerr := os.ReadFile(path); rerr == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return strings.TrimSpace(string(data)), nil
	}
	tok, terr := randomToken()
	if terr != nil {
		return "", terr
	}
	if werr := os.WriteFile(path, []byte(tok+"\n"), 0o600); werr != nil {
		return "", fmt.Errorf("store token: %w", werr)
	}
	return tok, nil
}

func Handler(deps Deps, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"daemon":   map[string]any{"running": deps.SupervisorRun},
			"dns_port": deps.DNSPort(),
			"projects": projectRows(deps.State),
		})
	})
	mux.HandleFunc("GET /api/doctor", func(w http.ResponseWriter, r *http.Request) {
		issues := deps.ListIssues(r.Context())
		writeJSON(w, map[string]any{"issues": issues, "count": len(issues)})
	})
	mux.HandleFunc("GET /api/logs/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/logs/")
		if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") {
			http.Error(w, "bad project", 400)
			return
		}
		lines, err := deps.LogLines(r.Context(), name, 500)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		out := make([]map[string]string, 0, len(lines))
		for _, l := range lines {
			out = append(out, map[string]string{"ts": "", "stream": "stdout", "line": l})
		}
		writeJSON(w, map[string]any{"lines": out})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r)
	})
	return authMiddleware(mux, token)
}

func authMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		auth := r.Header.Get("Authorization")
		got := strings.TrimPrefix(auth, "Bearer ")
		if got == "" {
			got = r.URL.Query().Get("token")
		}
		if got != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func projectRows(st *state.Store) []map[string]any {
	rows := make([]map[string]any, 0)
	for _, lp := range st.ListLinked() {
		rows = append(rows, map[string]any{
			"name":         lp.Name,
			"domain":       lp.Domain,
			"port":         lp.Port,
			"availability": lp.Availability,
			"phase":        "stopped",
		})
	}
	return rows
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}
