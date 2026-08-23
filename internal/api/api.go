package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/health"
	"github.com/SDK-E/dnser/internal/logstream"
)

type Runtime interface {
	Store() *config.Store
	Stream() *logstream.Stream
	Checker() *health.Checker
	DNSPort() int
	UIPort() int
	DashboardURL() string
}

type Server struct {
	rt      Runtime
	version string
	http    *http.Server
}

func New(rt Runtime, version string) *Server {
	s := &Server{rt: rt, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	mux.HandleFunc("PUT /api/v1/projects/{domain}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/v1/projects/{domain}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/v1/projects/{domain}/records", s.handleAddRecord)
	mux.HandleFunc("DELETE /api/v1/projects/{domain}/records", s.handleRemoveRecord)
	mux.HandleFunc("GET /api/v1/logs", s.handleLogs)
	mux.HandleFunc("GET /api/v1/logs/stream", s.handleLogStream)
	mux.Handle("/", http.HandlerFunc(s.handleStatic))
	s.http = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

func (s *Server) ListenAndServe(bind string, port int) error {
	addr := fmt.Sprintf("%s:%d", bind, port)
	ln, err := newListener(addr)
	if err != nil {
		return fmt.Errorf("bind ui %s: %w", addr, err)
	}
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			slogDebug("ui listener stopped", addr, err)
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

type statusPayload struct {
	Version      string       `json:"version"`
	TLD          string       `json:"tld"`
	Bind         string       `json:"bind"`
	DNSPort      int          `json:"dns_port"`
	Ports        config.Ports `json:"ports"`
	Upstreams    []string     `json:"upstreams"`
	DashboardURL string       `json:"dashboard_url"`
	Projects     int          `json:"projects"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := s.rt.Store().Settings()
	ports := st.Ports
	if effective := s.rt.UIPort(); effective > 0 {
		ports.UI = effective
	}
	writeJSON(w, http.StatusOK, statusPayload{
		Version:      s.version,
		TLD:          st.TLD,
		Bind:         st.Bind,
		DNSPort:      s.rt.DNSPort(),
		Ports:        ports,
		Upstreams:    st.Upstreams,
		DashboardURL: s.rt.DashboardURL(),
		Projects:     len(s.rt.Store().Projects()),
	})
}

type projectView struct {
	config.Project
	BackendHealth []backendHealthView `json:"backend_health,omitempty"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects := s.rt.Store().Projects()
	checker := s.rt.Checker()
	snap := map[string]health.Status{}
	if checker != nil {
		snap = checker.Snapshot()
	}
	out := make([]projectView, 0, len(projects))
	for _, p := range projects {
		view := projectView{Project: p, BackendHealth: backendHealth(p, snap)}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

func backendHealth(p config.Project, snap map[string]health.Status) []backendHealthView {
	var out []backendHealthView
	for _, route := range p.Routes {
		for _, b := range route.Backends {
			st, ok := snap[b]
			if !ok {
				continue
			}
			out = append(out, backendHealthView{
				Backend: b, Host: route.Hostname(p.Domain, ""), TCP: route.TCP,
				Up: st.Up, LatencyMS: st.LatencyMS, CheckedAt: st.CheckedAt, FailCount: st.FailCount,
			})
		}
	}
	return out
}

type backendHealthView struct {
	Backend   string    `json:"backend"`
	Host      string    `json:"host"`
	TCP       bool      `json:"tcp"`
	Up        bool      `json:"up"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	FailCount int       `json:"fail_count"`
}

type createProjectReq struct {
	Domain     string         `json:"domain"`
	Path       string         `json:"path,omitempty"`
	Routes     []config.Route `json:"routes,omitempty"`
	Port       int            `json:"port,omitempty"`
	Wildcard   bool           `json:"wildcard,omitempty"`
	HTTPS      bool           `json:"https,omitempty"`
	ForceHTTPS bool           `json:"force_https,omitempty"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if !decodeJSON(w, r, &req) {
		return
	}
	store := s.rt.Store()
	settings := store.Settings()
	domain, err := config.EnsureTLD(req.Domain, settings.TLD)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	routes := req.Routes
	if len(routes) == 0 && req.Port > 0 {
		routes = legacyRoutes(req.Port, req.Wildcard, req.HTTPS, req.ForceHTTPS)
	}
	err = store.Update(func(c *config.Config) {
		for i := range c.Projects {
			if c.Projects[i].Domain == domain {
				if len(routes) > 0 {
					c.Projects[i].Routes = routes
				}
				if req.Path != "" {
					c.Projects[i].Path = req.Path
				}
				return
			}
		}
		c.Projects = append(c.Projects, config.Project{
			Domain: domain,
			Path:   req.Path,
			Routes: routes,
		})
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _ := store.FindProject(domain)
	writeJSON(w, http.StatusCreated, projectView{Project: p})
}

type updateProjectReq struct {
	Routes   *[]config.Route `json:"routes,omitempty"`
	Path     *string         `json:"path,omitempty"`
	Port     *int            `json:"port,omitempty"`
	Wildcard *bool           `json:"wildcard,omitempty"`
	HTTPS    *bool           `json:"https,omitempty"`
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	domain, err := config.NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid domain")
		return
	}
	var req updateProjectReq
	if !decodeJSON(w, r, &req) {
		return
	}
	var updated bool
	store := s.rt.Store()
	err = store.Update(func(c *config.Config) {
		for i := range c.Projects {
			if c.Projects[i].Domain != domain {
				continue
			}
			updated = true
			if req.Routes != nil {
				c.Projects[i].Routes = *req.Routes
			}
			if req.Path != nil {
				c.Projects[i].Path = *req.Path
			}
			if req.Port != nil && *req.Port > 0 {
				patchLegacyPort(&c.Projects[i], *req.Port, req.Wildcard, req.HTTPS)
			}
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !updated {
		writeErr(w, http.StatusNotFound, "project not linked")
		return
	}
	p, _ := store.FindProject(domain)
	writeJSON(w, http.StatusOK, projectView{Project: p})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	domain, err := config.NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid domain")
		return
	}
	var removed bool
	err = s.rt.Store().Update(func(c *config.Config) {
		kept := c.Projects[:0]
		for _, p := range c.Projects {
			if p.Domain == domain {
				removed = true
				continue
			}
			kept = append(kept, p)
		}
		c.Projects = kept
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !removed {
		writeErr(w, http.StatusNotFound, "project not linked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type addRecordReq = config.Record

func (s *Server) handleAddRecord(w http.ResponseWriter, r *http.Request) {
	domain, err := config.NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid domain")
		return
	}
	var rec addRecordReq
	if !decodeJSON(w, r, &rec) {
		return
	}
	if rec.TTL == 0 {
		rec.TTL = 120
	}
	if err := config.ValidateRecord(rec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var found bool
	err = s.rt.Store().Update(func(c *config.Config) {
		for i := range c.Projects {
			if c.Projects[i].Domain != domain {
				continue
			}
			found = true
			c.Projects[i].Records = append(c.Projects[i].Records, rec)
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "project not linked")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "record": rec})
}

type removeRecordReq struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

func (s *Server) handleRemoveRecord(w http.ResponseWriter, r *http.Request) {
	domain, err := config.NormalizeDomain(r.PathValue("domain"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid domain")
		return
	}
	var req removeRecordReq
	if !decodeJSON(w, r, &req) {
		return
	}
	label, err := config.NormalizeLabel(req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	removed := 0
	err = s.rt.Store().Update(func(c *config.Config) {
		for i := range c.Projects {
			if c.Projects[i].Domain != domain {
				continue
			}
			kept := c.Projects[i].Records[:0]
			for _, rec := range c.Projects[i].Records {
				if rec.Name == label &&
					(req.Type == "" || rec.Type == req.Type) &&
					(req.Value == "" || rec.Value == req.Value) {
					removed++
					continue
				}
				kept = append(kept, rec)
			}
			c.Projects[i].Records = kept
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	events := s.rt.Stream().Recent(limit)
	if events == nil {
		events = []logstream.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := s.rt.Stream().Subscribe(128)
	defer unsub()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(target); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func legacyRoutes(port int, wildcard, https, forceHTTPS bool) []config.Route {
	routes := []config.Route{{
		Host:       "@",
		Backends:   []string{fmt.Sprintf("localhost:%d", port)},
		HTTPS:      https,
		ForceHTTPS: forceHTTPS,
	}}
	if wildcard {
		routes = append(routes, config.Route{
			Host:       "*",
			Backends:   []string{fmt.Sprintf("localhost:%d", port)},
			HTTPS:      https,
			ForceHTTPS: forceHTTPS,
		})
	}
	return routes
}

func patchLegacyPort(p *config.Project, port int, wildcard, https *bool) {
	backend := fmt.Sprintf("localhost:%d", port)
	for j := range p.Routes {
		if p.Routes[j].Host == "@" && len(p.Routes[j].Backends) > 0 {
			p.Routes[j].Backends[0] = backend
		}
		if p.Routes[j].Host == "*" {
			if len(p.Routes[j].Backends) > 0 {
				p.Routes[j].Backends[0] = backend
			}
			if https != nil {
				p.Routes[j].HTTPS = *https
			}
		}
		if p.Routes[j].Host == "@" && https != nil {
			p.Routes[j].HTTPS = *https
		}
	}
	if wildcard != nil && *wildcard {
		hasWildcard := false
		for _, r := range p.Routes {
			if r.Host == "*" {
				hasWildcard = true
			}
		}
		if !hasWildcard {
			p.Routes = append(p.Routes, config.Route{Host: "*", Backends: []string{backend}})
		}
	}
}
