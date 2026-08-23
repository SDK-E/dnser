package daemon

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/detect"
	"github.com/SDK-E/dnser/internal/runner"
)

func (rt *Runtime) syncRunner(cfg config.Config) {
	if rt.runner == nil {
		return
	}
	want := make(map[string]runner.Spec, len(cfg.Projects))
	for _, p := range cfg.Projects {
		if p.Path == "" || p.Run == nil {
			delete(rt.depsMissing, p.Domain)
			continue
		}
		spec, _, ok := rt.resolveSpec(p)
		if !ok {
			continue
		}
		port := p.Run.Port
		switch {
		case port > 0 && !portFree(port):
			exclude := managedPortExcludes(cfg, p.Domain)
			fresh, err := runner.AllocateFreePort(exclude)
			if err != nil {
				slog.Warn("runner: no free port to reallocate", "project", p.Domain, "err", err)
				continue
			}
			slog.Warn(fmt.Sprintf("project %s port %d is taken by another process; moving it to %d", p.Domain, port, fresh))
			rt.persistPort(cfg, p.Domain, port, fresh)
			spec.Port = fresh
		case port == 0:
			fresh, err := runner.AllocateFreePort(managedPortExcludes(cfg, p.Domain))
			if err != nil {
				slog.Warn("runner: port allocation failed", "project", p.Domain, "err", err)
				continue
			}
			rt.persistPort(cfg, p.Domain, 0, fresh)
			spec.Port = fresh
		default:
			spec.Port = port
		}
		delete(rt.depsMissing, p.Domain)
		want[p.Domain] = spec
	}

	for domain := range rt.runner.Info() {
		if _, keep := want[domain]; !keep {
			rt.runner.Remove(domain)
		}
	}
	for domain, spec := range want {
		current, running := rt.runner.Get(domain)
		if !running || !sameSpec(current, spec) {
			if err := rt.runner.Start(spec); err != nil {
				slog.Warn("runner: start failed", "project", domain, "err", err)
			}
		}
	}
}

func (rt *Runtime) resolveSpec(p config.Project) (runner.Spec, string, bool) {
	dir := expandPath(p.Path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		rt.depsMissing[p.Domain] = fmt.Sprintf("path not found: %s", dir)
		return runner.Spec{}, "", false
	}

	result, recipe, err := detect.DetectStack(dir)
	if err != nil {
		rt.depsMissing[p.Domain] = fmt.Sprintf("detect stack: %v", err)
		return runner.Spec{}, "", false
	}
	framework := result.Framework
	spec := runner.Spec{Domain: p.Domain, Dir: dir, Framework: framework, PortEnv: true, UseShell: true}

	switch {
	case strings.TrimSpace(p.Run.Command) != "":
		spec.Command = strings.Fields(p.Run.Command)
	default:
		if override, ok := runner.ReadLinkOverride(dir); ok {
			spec.Command = strings.Fields(override.Command)
		} else if len(recipe.Command) > 0 {
			spec.Command = recipe.Command
			spec.UseShell = false
			spec.PortEnv = recipe.PortEnv
		} else {
			rt.depsMissing[p.Domain] = fmt.Sprintf("no dev command known for %s — add command: to %s", framework, filepath.Join(dir, ".dnser.yaml"))
			return runner.Spec{}, "", false
		}
	}

	if !detect.DepsInstalled(dir, framework) {
		hint := detect.InstallHint(framework)
		if hint == "" {
			hint = "install project dependencies first"
		}
		rt.depsMissing[p.Domain] = hint
		return runner.Spec{}, hint, false
	}
	return spec, "", true
}

func (rt *Runtime) persistPort(cfg config.Config, domain string, oldPort, newPort int) {
	err := rt.store.Update(func(c *config.Config) {
		for i := range c.Projects {
			p := &c.Projects[i]
			if p.Domain != domain || p.Run == nil {
				continue
			}
			p.Run.Port = newPort
			if oldPort <= 0 {
				continue
			}
			oldSuffix := ":" + strconv.Itoa(oldPort)
			newSuffix := ":" + strconv.Itoa(newPort)
			for j := range p.Routes {
				for k, b := range p.Routes[j].Backends {
					if strings.HasSuffix(b, oldSuffix) {
						p.Routes[j].Backends[k] = strings.TrimSuffix(b, oldSuffix) + newSuffix
					}
				}
			}
		}
	})
	if err != nil {
		slog.Warn("runner: persisting port failed", "project", domain, "err", err)
	}
}

func sameSpec(info runner.AppInfo, spec runner.Spec) bool {
	return info.Path == spec.Dir &&
		info.Port == spec.Port &&
		info.Framework == spec.Framework &&
		strings.Join(info.Command, " ") == strings.Join(spec.Command, " ")
}

func managedPortExcludes(cfg config.Config, skipDomain string) map[int]bool {
	exclude := map[int]bool{
		cfg.Settings.Ports.DNS:   true,
		cfg.Settings.Ports.HTTP:  true,
		cfg.Settings.Ports.HTTPS: true,
		cfg.Settings.Ports.UI:    true,
	}
	for _, p := range cfg.Projects {
		if p.Domain == skipDomain {
			continue
		}
		if p.Run != nil && p.Run.Port > 0 {
			exclude[p.Run.Port] = true
		}
		for _, r := range p.Routes {
			if r.Listen > 0 {
				exclude[r.Listen] = true
			}
			for _, b := range r.Backends {
				if port, ok := backendPort(b); ok {
					exclude[port] = true
				}
			}
		}
	}
	return exclude
}

func backendPort(backend string) (int, bool) {
	_, portStr, err := net.SplitHostPort(backend)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func expandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}

func (rt *Runtime) Runner() *runner.Supervisor { return rt.runner }

func (rt *Runtime) DepsMissing() map[string]string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make(map[string]string, len(rt.depsMissing))
	for k, v := range rt.depsMissing {
		out[k] = v
	}
	return out
}
