package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/runner"
)

func (s *Server) handleRunner(w http.ResponseWriter, r *http.Request) {
	sup := s.rt.Runner()
	if sup == nil {
		writeJSON(w, http.StatusOK, map[string]any{"apps": []runner.AppInfo{}, "deps_missing": map[string]string{}})
		return
	}
	infos := sup.Info()
	out := make([]runner.AppInfo, 0, len(infos))
	for _, info := range infos {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "deps_missing": s.rt.DepsMissing()})
}

func (s *Server) handleRunnerAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	key := r.PathValue("key")
	sup := s.rt.Runner()
	if sup == nil {
		writeErr(w, http.StatusServiceUnavailable, "runner unavailable")
		return
	}
	switch action {
	case "restart":
		if err := sup.Restart(key); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	case "stop":
		if !sup.Stop(key) {
			writeErr(w, http.StatusNotFound, "project is not managed by the runner")
			return
		}
	case "start":
		found := false
		for _, p := range s.rt.Store().Projects() {
			if p.Domain == key || strings.HasPrefix(key, p.Domain+"/") {
				found = true
				break
			}
		}
		if !found {
			writeErr(w, http.StatusNotFound, "unknown project")
			return
		}
		s.rt.SyncRunner()
	default:
		writeErr(w, http.StatusNotFound, "unknown action")
		return
	}
	time.Sleep(200 * time.Millisecond)
	info, _ := sup.Get(key)
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleRunnerRestart(w http.ResponseWriter, r *http.Request) {
	sup := s.rt.Runner()
	if sup == nil {
		writeErr(w, http.StatusServiceUnavailable, "runner unavailable")
		return
	}
	domain := r.PathValue("domain")
	if err := sup.Restart(domain); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	info, _ := sup.Get(domain)
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleRunnerStop(w http.ResponseWriter, r *http.Request) {
	sup := s.rt.Runner()
	if sup == nil {
		writeErr(w, http.StatusServiceUnavailable, "runner unavailable")
		return
	}
	domain := r.PathValue("domain")
	if !sup.Stop(domain) {
		writeErr(w, http.StatusNotFound, "project is not managed by the runner")
		return
	}
	info, _ := sup.Get(domain)
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleRunnerStart(w http.ResponseWriter, r *http.Request) {
	if s.rt.Runner() == nil {
		writeErr(w, http.StatusServiceUnavailable, "runner unavailable")
		return
	}
	domain := r.PathValue("domain")
	found := false
	for _, p := range s.rt.Store().Projects() {
		if p.Domain == domain {
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "unknown project")
		return
	}
	s.rt.SyncRunner()
	info, ok := s.rt.Runner().Get(domain)
	if !ok || info.State == runner.StateStopped || info.State == "" {
		time.Sleep(300 * time.Millisecond)
		info, _ = s.rt.Runner().Get(domain)
	}
	writeJSON(w, http.StatusOK, info)
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	checks := make([]doctorCheck, 0, 8)

	st := s.rt.Store().Settings()
	if dnsPort := s.rt.DNSPort(); dnsPort > 0 {
		status, detail := "ok", fmt.Sprintf("DNS listening on %s:%d", st.Bind, dnsPort)
		if dnsPort != st.Ports.DNS {
			status, detail = "warn", fmt.Sprintf("preferred DNS port %d unavailable; using %d", st.Ports.DNS, dnsPort)
		}
		checks = append(checks, doctorCheck{Name: "dns-port", Status: status, Detail: detail})
	} else {
		checks = append(checks, doctorCheck{Name: "dns-port", Status: "fail", Detail: "DNS server not running"})
	}

	for _, pc := range []struct {
		name string
		port int
	}{{"http-port", st.Ports.HTTP}, {"https-port", st.Ports.HTTPS}} {
		status, detail := probePort(st.Bind, pc.port)
		checks = append(checks, doctorCheck{Name: pc.name, Status: status, Detail: detail})
	}

	upstreams := st.Upstreams
	failed := 0
	for _, up := range upstreams {
		if !dialUDP(up) {
			failed++
		}
	}
	switch {
	case len(upstreams) == 0:
		checks = append(checks, doctorCheck{Name: "upstreams", Status: "warn", Detail: "no upstream resolvers configured"})
	case failed > 0:
		checks = append(checks, doctorCheck{Name: "upstreams", Status: "warn",
			Detail: fmt.Sprintf("%d of %d upstream resolvers unreachable", failed, len(upstreams))})
	default:
		checks = append(checks, doctorCheck{Name: "upstreams", Status: "ok",
			Detail: fmt.Sprintf("%d upstream resolvers reachable", len(upstreams))})
	}

	if runtime.GOOS != "windows" {
		checks = append(checks, resolverCheck(st.Bind))
	}

	deps := s.rt.DepsMissing()
	if len(deps) > 0 {
		keys := make([]string, 0, len(deps))
		for k := range deps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+": "+deps[k])
		}
		checks = append(checks, doctorCheck{Name: "projects", Status: "warn",
			Detail: "dependency issues — " + strings.Join(parts, "; ")})
	} else {
		checks = append(checks, doctorCheck{Name: "projects", Status: "ok", Detail: "all managed projects ready"})
	}

	checks = append(checks, s.commandsCheck())

	status := "ok"
	for _, c := range checks {
		if c.Status == "fail" {
			status = "fail"
			break
		}
		if c.Status == "warn" {
			status = "warn"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "checks": checks})
}

func (s *Server) commandsCheck() doctorCheck {
	dirs := s.rt.EffectivePATHDirs()
	var problems []string
	for _, p := range s.rt.Store().Projects() {
		for _, cmd := range projectCommands(p) {
			bin := runner.CommandBinary(cmd)
			if bin == "" {
				continue
			}
			if _, err := runner.ResolveCommandPath(dirs, bin); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %s not found in daemon PATH — install it or extend the daemon's PATH", p.Domain, bin))
			}
		}
	}
	if len(problems) == 0 {
		return doctorCheck{Name: "commands", Status: "ok", Detail: "all managed commands resolve in daemon PATH"}
	}
	sort.Strings(problems)
	return doctorCheck{Name: "commands", Status: "warn",
		Detail: strings.Join(problems, "; ") + " — see docs/troubleshooting.md#managed-command-path"}
}

func projectCommands(p config.Project) []string {
	var out []string
	if p.Run != nil && strings.TrimSpace(p.Run.Command) != "" {
		out = append(out, p.Run.Command)
	}
	dir := p.Path
	if dir == "" {
		return out
	}
	if abs, err := expandTilde(dir); err == nil {
		dir = abs
	}
	if doc, err := runner.ParseDotDnserDir(dir); err == nil && doc != nil {
		if doc.Command != "" && (p.Run == nil || strings.TrimSpace(p.Run.Command) == "") {
			out = append(out, doc.Command)
		}
		for _, svc := range doc.Services {
			if svc.Command != "" {
				out = append(out, svc.Command)
			}
		}
	}
	return out
}

func expandTilde(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path, err
		}
		return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/")), nil
	}
	return filepath.Abs(path)
}

func probePort(bind string, port int) (string, string) {
	addr := net.JoinHostPort(bindIfWildcard(bind), strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "ok", fmt.Sprintf("port %d in use (dnser is serving it)", port)
	}
	_ = ln.Close()
	return "fail", fmt.Sprintf("port %d is free but dnser is not serving on it — restart the daemon", port)
}

func bindIfWildcard(bind string) string {
	if bind == "" || bind == "0.0.0.0" || bind == "::" {
		return "127.0.0.1"
	}
	return bind
}

func dialUDP(upstream string) bool {
	host := upstream
	if h, _, err := net.SplitHostPort(upstream); err == nil {
		host = h
	}
	addr := net.JoinHostPort(host, "53")
	conn, err := net.DialTimeout("udp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func resolverCheck(bind string) doctorCheck {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return doctorCheck{Name: "resolver", Status: "warn", Detail: "cannot read /etc/resolv.conf"}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "nameserver") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == bind {
			return doctorCheck{Name: "resolver", Status: "ok", Detail: "system resolver points to dnser (" + bind + ")"}
		}
	}
	return doctorCheck{Name: "resolver", Status: "warn",
		Detail: fmt.Sprintf("system resolver does not point to dnser (%s) — run 'dnser setup' or add a nameserver entry", bind)}
}
