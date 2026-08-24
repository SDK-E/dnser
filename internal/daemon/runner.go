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
	rt.mergeDotDnser(&cfg)

	want := make(map[string]runner.Spec, len(cfg.Projects))
	exclude := managedPortExcludes(cfg, "")
	for _, p := range cfg.Projects {
		if p.Path == "" || p.Run == nil {
			delete(rt.depsMissing, p.Domain)
			continue
		}
		specs, ok := rt.resolveSpecs(p)
		if !ok {
			continue
		}

		pending := map[string]int{}
		switch {
		case specs[0].Port > 0 && !portFree(specs[0].Port):
			fresh, err := runner.AllocateFreePort(exclude)
			if err != nil {
				slog.Warn("runner: no free port to reallocate", "project", p.Domain, "err", err)
				continue
			}
			slog.Warn(fmt.Sprintf("project %s port %d is taken by another process; moving it to %d", p.Domain, specs[0].Port, fresh))
			pending[""] = fresh
			exclude[fresh] = true
			specs[0].Port = fresh
		case specs[0].Port == 0:
			fresh, err := runner.AllocateFreePort(exclude)
			if err != nil {
				slog.Warn("runner: port allocation failed", "project", p.Domain, "err", err)
				continue
			}
			pending[""] = fresh
			exclude[fresh] = true
			specs[0].Port = fresh
		default:
			exclude[specs[0].Port] = true
		}

		named := map[string]int{"": specs[0].Port}
		for i := 1; i < len(specs); i++ {
			svcName := strings.TrimPrefix(specs[i].Domain, p.Domain+"/")
			if specs[i].Port == 0 {
				fresh, err := runner.AllocateFreePort(exclude)
				if err != nil {
					slog.Warn("runner: service port allocation failed", "project", p.Domain, "service", svcName, "err", err)
					continue
				}
				specs[i].Port = fresh
				pending[svcName] = fresh
			}
			exclude[specs[i].Port] = true
			named[svcName] = specs[i].Port
		}
		if len(pending) > 0 {
			rt.persistPorts(p.Domain, pending)
		}
		for i := range specs {
			specs[i].NamedPorts = named
		}
		delete(rt.depsMissing, p.Domain)
		for _, spec := range specs {
			want[spec.Domain] = spec
		}
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

func (rt *Runtime) resolveSpecs(p config.Project) ([]runner.Spec, bool) {
	dir := expandPath(p.Path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		rt.depsMissing[p.Domain] = fmt.Sprintf("path not found: %s", dir)
		return nil, false
	}

	result, recipe, err := detect.DetectStack(dir)
	if err != nil {
		rt.depsMissing[p.Domain] = fmt.Sprintf("detect stack: %v", err)
		return nil, false
	}
	framework := result.Framework

	var doc *runner.DotDnser
	if parsed, err := runner.ParseDotDnserDir(dir); err == nil {
		doc = parsed
	}

	services := effectiveServices(p, doc)

	var primary runner.Spec
	resolved := false
	switch {
	case strings.TrimSpace(p.Run.Command) != "":
		primary = baseSpec(p, dir, framework, true, true)
		primary.Command = strings.Fields(p.Run.Command)
		resolved = true
	default:
		if doc != nil && doc.Command != "" {
			primary = baseSpec(p, dir, framework, true, true)
			primary.Command = strings.Fields(doc.Command)
			resolved = true
		} else if len(recipe.Command) > 0 {
			primary = baseSpec(p, dir, framework, false, recipe.PortEnv)
			primary.Command = recipe.Command
			resolved = true
		} else {
			rt.depsMissing[p.Domain] = fmt.Sprintf("no dev command known for %s — add command: to %s", framework, filepath.Join(dir, ".dnser.yaml"))
		}
	}

	specs := make([]runner.Spec, 0, 1+len(services))
	if resolved {
		specs = append(specs, primary)
	}
	for _, svc := range services {
		if !svc.Managed() {
			continue
		}
		spec := runner.Spec{
			Domain:    p.Domain + "/" + svc.Name,
			Dir:       dir,
			Framework: svc.Type,
			Port:      svc.Port,
			Command:   strings.Fields(svc.Command),
			UseShell:  true,
			PortEnv:   true,
			Env:       []string{"DNSER_SERVICE=" + svc.Name},
		}
		specs = append(specs, spec)
	}

	if !detect.DepsInstalled(dir, framework) {
		hint := detect.InstallHint(framework)
		if hint == "" {
			hint = "install project dependencies first"
		}
		rt.depsMissing[p.Domain] = hint
		return nil, false
	}
	if !resolved {
		return nil, false
	}
	return specs, true
}

func baseSpec(p config.Project, dir, framework string, useShell, portEnv bool) runner.Spec {
	return runner.Spec{
		Domain:    p.Domain,
		Dir:       dir,
		Framework: framework,
		Port:      p.Run.Port,
		UseShell:  useShell,
		PortEnv:   portEnv,
	}
}

func effectiveServices(p config.Project, doc *runner.DotDnser) []config.Service {
	out := append([]config.Service(nil), p.Services...)
	seen := map[string]bool{}
	for _, s := range out {
		seen[s.Name] = true
	}
	if doc != nil {
		for _, ds := range doc.Services {
			if seen[ds.Name] || (ds.Command == "" && ds.Host == "") {
				continue
			}
			svc := config.Service{
				Name:      ds.Name,
				Type:      ds.Type,
				Command:   ds.Command,
				Host:      ds.Host,
				Port:      ds.Port,
				Transport: ds.Transport,
			}
			if ds.DNS != nil {
				svc.DNS = *ds.DNS
			} else {
				svc.DNS = true
			}
			out = append(out, svc)
		}
	}
	return out
}

func (rt *Runtime) mergeDotDnser(cfg *config.Config) {
	type pendingProject struct {
		domain   string
		routes   []config.Route
		services []config.Service
		records  []config.Record
	}
	var pending []pendingProject

	tld := cfg.Settings.TLD
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.Path == "" || p.Run == nil {
			continue
		}
		dir := expandPath(p.Path)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		doc, err := runner.ParseDotDnserDir(dir)
		if err != nil || doc == nil {
			continue
		}

		entry := pendingProject{domain: p.Domain}
		knownHosts := map[string]bool{}
		for _, r := range p.Routes {
			knownHosts[r.Hostname(p.Domain, tld)] = true
		}
		for _, fr := range doc.Routes {
			host := fr.Host
			if host == "" {
				host = "@"
			}
			route := config.Route{
				Host:       host,
				Backends:   fr.Backends,
				TCP:        fr.TCP,
				UDP:        fr.UDP,
				Listen:     fr.Listen,
				HTTPS:      fr.HTTPS,
				ForceHTTPS: fr.ForceHTTPS,
				Paths:      fr.Paths,
			}
			if knownHosts[route.Hostname(p.Domain, tld)] {
				continue
			}
			knownHosts[route.Hostname(p.Domain, tld)] = true
			entry.routes = append(entry.routes, route)
		}

		storedSvc := map[string]bool{}
		for _, s := range p.Services {
			storedSvc[s.Name] = true
		}
		effective := effectiveServices(*p, doc)
		for _, svc := range effectiveServicesFromDoc(doc) {
			if storedSvc[svc.Name] {
				continue
			}
			entry.services = append(entry.services, svc)
		}
		for _, svc := range effective {
			rec, wanted := serviceRecord(svc)
			if !wanted {
				continue
			}
			if hasRecord(p.Records, rec) {
				continue
			}
			entry.records = append(entry.records, rec)
		}

		if len(entry.routes) > 0 || len(entry.services) > 0 || len(entry.records) > 0 {
			pending = append(pending, entry)
		}
	}
	if len(pending) == 0 {
		return
	}

	err := rt.store.Update(func(c *config.Config) {
		for _, entry := range pending {
			for j := range c.Projects {
				if c.Projects[j].Domain != entry.domain {
					continue
				}
				pj := &c.Projects[j]
				pj.Routes = append(pj.Routes, entry.routes...)
				pj.Services = append(pj.Services, entry.services...)
				for _, rec := range entry.records {
					if !hasRecord(pj.Records, rec) {
						pj.Records = append(pj.Records, rec)
					}
				}
			}
		}
	})
	if err != nil {
		slog.Warn("runner: importing .dnser.yaml failed", "err", err)
		return
	}
	for _, entry := range pending {
		for j := range cfg.Projects {
			if cfg.Projects[j].Domain != entry.domain {
				continue
			}
			pj := &cfg.Projects[j]
			pj.Routes = append(pj.Routes, entry.routes...)
			pj.Services = append(pj.Services, entry.services...)
			for _, rec := range entry.records {
				if !hasRecord(pj.Records, rec) {
					pj.Records = append(pj.Records, rec)
				}
			}
		}
	}
}

func effectiveServicesFromDoc(doc *runner.DotDnser) []config.Service {
	out := make([]config.Service, 0, len(doc.Services))
	for _, ds := range doc.Services {
		if ds.Command == "" && ds.Host == "" {
			continue
		}
		svc := config.Service{
			Name:      ds.Name,
			Type:      ds.Type,
			Command:   ds.Command,
			Host:      ds.Host,
			Port:      ds.Port,
			Transport: ds.Transport,
		}
		if ds.DNS != nil {
			svc.DNS = *ds.DNS
		} else {
			svc.DNS = true
		}
		out = append(out, svc)
	}
	return out
}

func serviceRecord(svc config.Service) (config.Record, bool) {
	if !svc.DNS {
		return config.Record{}, false
	}
	rec := config.Record{Name: svc.Name, TTL: 120}
	switch {
	case svc.Managed():
		rec.Type = "A"
		rec.Value = "127.0.0.1"
	case net.ParseIP(svc.Host) != nil:
		rec.Type = "A"
		rec.Value = svc.Host
	case svc.Host != "":
		rec.Type = "CNAME"
		rec.Value = svc.Host
	default:
		return config.Record{}, false
	}
	return rec, true
}

func hasRecord(records []config.Record, rec config.Record) bool {
	for _, existing := range records {
		if strings.EqualFold(existing.Name, rec.Name) &&
			strings.EqualFold(existing.Type, rec.Type) &&
			strings.EqualFold(existing.Value, rec.Value) {
			return true
		}
	}
	return false
}

func (rt *Runtime) persistPorts(domain string, ports map[string]int) {
	oldPrimary, newPrimary := ports[""], ports[""]
	err := rt.store.Update(func(c *config.Config) {
		for i := range c.Projects {
			p := &c.Projects[i]
			if p.Domain != domain || p.Run == nil {
				continue
			}
			if oldPrimary > 0 && newPrimary > 0 && p.Run.Port != newPrimary {
				oldPort := p.Run.Port
				p.Run.Port = newPrimary
				rewriteBackends(p, oldPort, newPrimary)
			}
			for name, port := range ports {
				if name == "" {
					continue
				}
				for j := range p.Services {
					if p.Services[j].Name == name && p.Services[j].Port != port {
						p.Services[j].Port = port
					}
				}
			}
		}
	})
	if err != nil {
		slog.Warn("runner: persisting ports failed", "project", domain, "err", err)
	}
}

func rewriteBackends(p *config.Project, oldPort, newPort int) {
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
		for _, svc := range p.Services {
			if svc.Managed() && svc.Port > 0 {
				exclude[svc.Port] = true
			}
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
	if config.IsPortPlaceholder(portStr) {
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

func (rt *Runtime) SyncRunner() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.syncRunner(rt.store.Get())
}

func (rt *Runtime) DepsMissing() map[string]string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make(map[string]string, len(rt.depsMissing))
	for k, v := range rt.depsMissing {
		out[k] = v
	}
	return out
}

func (rt *Runtime) EffectivePATHDirs() []string {
	if rt.runner == nil {
		return nil
	}
	return rt.runner.EffectivePATHDirs()
}
