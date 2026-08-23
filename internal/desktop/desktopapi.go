package desktop

import (
	"encoding/json"
	"net"
	"net/http"
)

type desktopStatusPayload struct {
	Status    Status          `json:"status"`
	Setup     SetupStatusView `json:"setup"`
	Autostart bool            `json:"autostart"`
	Update    UpdateInfo      `json:"update"`
}

func (s *Service) APIRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/desktop/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, desktopStatusPayload{
			Status:    s.Status(),
			Setup:     s.SetupStatus(),
			Autostart: autostartActive(),
			Update:    s.Update(),
		})
	})
	mux.HandleFunc("POST /api/v1/desktop/setup", func(w http.ResponseWriter, r *http.Request) {
		var steps []SetupStep
		err := s.RunSetup(func(step SetupStep) {
			steps = append(steps, step)
		})
		if err != nil && steps == nil {
			steps = []SetupStep{{Name: "setup", Err: err.Error()}}
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"steps": steps})
			return
		}
		writeJSON(w, map[string]any{"steps": steps})
	})
	mux.HandleFunc("POST /api/v1/desktop/revert", func(w http.ResponseWriter, r *http.Request) {
		if err := s.RevertSetup(); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})
	mux.HandleFunc("POST /api/v1/desktop/autostart", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if err := s.SetAutostart(req.Enabled); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]bool{"enabled": req.Enabled})
	})
	return loopbackOnly(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden: desktop control is local-only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
