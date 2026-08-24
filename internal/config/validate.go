package config

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var typeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func Validate(cfg Config) error {
	if cfg.Version > CurrentVersion {
		return fmt.Errorf("config version %d is newer than this build supports (max %d); upgrade dnser", cfg.Version, CurrentVersion)
	}
	if cfg.Version < 1 {
		return fmt.Errorf("config missing version field")
	}
	s := cfg.Settings
	tld, err := NormalizeDomain(s.TLD)
	if err != nil {
		return fmt.Errorf("settings.tld: %w", err)
	}
	if strings.Contains(tld, "*") {
		return fmt.Errorf("settings.tld: wildcard not allowed")
	}
	ip := net.ParseIP(s.Bind)
	if ip == nil {
		return fmt.Errorf("settings.bind: %q is not an IP address", s.Bind)
	}
	if len(s.Upstreams) == 0 {
		return fmt.Errorf("settings.upstreams: at least one upstream resolver required")
	}
	for _, u := range s.Upstreams {
		host := u
		if h, _, err := net.SplitHostPort(u); err == nil {
			host = h
		}
		if net.ParseIP(host) == nil {
			if _, err := NormalizeDomain(host); err != nil {
				return fmt.Errorf("settings.upstreams: invalid upstream %q", u)
			}
		}
	}
	for name, port := range map[string]int{"dns": s.Ports.DNS, "http": s.Ports.HTTP, "https": s.Ports.HTTPS, "ui": s.Ports.UI} {
		if port < 1 || port > 65535 {
			return fmt.Errorf("settings.ports.%s: %d out of range 1-65535", name, port)
		}
	}
	if s.PathRefreshMins < 0 || s.PathRefreshMins > 525600 {
		return fmt.Errorf("settings.path_refresh_minutes: %d out of range 0-525600", s.PathRefreshMins)
	}

	corePorts := map[int]string{
		s.Ports.DNS: "dns", s.Ports.HTTP: "http", s.Ports.HTTPS: "https", s.Ports.UI: "ui",
	}
	tcpListens := map[int]string{}
	for port, what := range corePorts {
		tcpListens[port] = "settings.ports." + what
	}

	seen := map[string]bool{DashboardDomain(tld): true}
	for i, p := range cfg.Projects {
		domain, err := NormalizeDomain(p.Domain)
		if err != nil {
			return fmt.Errorf("projects[%d].domain: %w", i, err)
		}
		if IsWildcard(domain) {
			return fmt.Errorf("projects[%d]: domain %q must be a zone, not a wildcard", i, p.Domain)
		}
		if seen[domain] {
			return fmt.Errorf("projects[%d]: duplicate domain %q", i, domain)
		}
		seen[domain] = true

		hostSeen := map[string]bool{}
		for j, route := range p.Routes {
			host, err := NormalizeLabel(route.Host)
			if err != nil {
				return fmt.Errorf("projects[%d] (%s).routes[%d].host: %w", i, domain, j, err)
			}
			resolved := ResolveHost(host, domain, tld)
			if hostSeen[resolved] {
				return fmt.Errorf("projects[%d] (%s): duplicate route host %q", i, domain, resolved)
			}
			hostSeen[resolved] = true
			if resolved != domain && seen[resolved] {
				return fmt.Errorf("projects[%d] (%s): duplicate route host %q", i, domain, resolved)
			}
			seen[resolved] = true
			if len(route.Backends) == 0 {
				return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): at least one backend required", i, domain, j, resolved)
			}
			for k, b := range route.Backends {
				h, portStr, err := splitBackend(strings.TrimSpace(b))
				if err != nil {
					return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: %q must be host:port", i, domain, j, k, b)
				}
				if IsPortPlaceholder(portStr) {
					if h == "" {
						return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: empty backend host in %q", i, domain, j, k, b)
					}
					continue
				}
				port, perr := strconv.Atoi(portStr)
				if perr != nil {
					return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: %q has non-numeric port", i, domain, j, k, b)
				}
				if h == "" {
					return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: empty backend host in %q", i, domain, j, k, b)
				}
				if net.ParseIP(h) == nil {
					if _, err := NormalizeDomain(h); err != nil {
						return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: invalid backend host %q", i, domain, j, k, h)
					}
				}
				if port < 1 || port > 65535 {
					return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: port %d out of range", i, domain, j, k, port)
				}
			}
			if (route.TCP || route.UDP) && route.TCP == route.UDP {
				return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): tcp and udp are mutually exclusive", i, domain, j, resolved)
			}
			if route.Forwarded() {
				if route.Listen < 1 || route.Listen > 65535 {
					kind := "tcp"
					if route.UDP {
						kind = "udp"
					}
					return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): %s listen port required (1-65535)", i, domain, j, resolved, kind)
				}
				if owner, clash := tcpListens[route.Listen]; clash {
					return fmt.Errorf("projects[%d] (%s).routes[%d]: listen %d already used by %s", i, domain, j, route.Listen, owner)
				}
				tcpListens[route.Listen] = fmt.Sprintf("projects[%d] (%s).routes[%d]", i, domain, j)
			} else if route.Listen != 0 {
				return fmt.Errorf("projects[%d] (%s).routes[%d]: listen is only valid for tcp/udp routes", i, domain, j)
			}
			if route.HTTPS && (route.TCP || route.UDP) {
				return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): https does not apply to tcp/udp routes", i, domain, j, resolved)
			}
			pathSeen := map[string]bool{}
			for k, raw := range route.Paths {
				p := NormalizePathPrefix(raw)
				if p == "" || p == "/" {
					return fmt.Errorf("projects[%d] (%s).routes[%d].paths[%d]: %q must be a path prefix like /api", i, domain, j, k, raw)
				}
				if strings.ContainsAny(p, " \t\"'") {
					return fmt.Errorf("projects[%d] (%s).routes[%d].paths[%d]: %q contains invalid characters", i, domain, j, k, raw)
				}
				if pathSeen[p] {
					return fmt.Errorf("projects[%d] (%s).routes[%d].paths[%d]: duplicate path %q", i, domain, j, k, p)
				}
				pathSeen[p] = true
			}
			route.Paths = nil
			for pref := range pathSeen {
				route.Paths = append(route.Paths, pref)
			}
			sort.Strings(route.Paths)
			p.Routes[j].Paths = route.Paths
			if route.ForceHTTPS && !route.HTTPS {
				return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): force_https requires https", i, domain, j, resolved)
			}
		}

		svcSeen := map[string]bool{}
		for j, svc := range p.Services {
			name, err := NormalizeLabel(svc.Name)
			if err != nil {
				return fmt.Errorf("projects[%d] (%s).services[%d].name: %w", i, domain, j, err)
			}
			if svcSeen[name] {
				return fmt.Errorf("projects[%d] (%s): duplicate service %q", i, domain, name)
			}
			svcSeen[name] = true
			p.Services[j].Name = name
			switch svc.Transport {
			case "", "tcp", "udp":
				p.Services[j].Transport = svc.Transport
			default:
				return fmt.Errorf("projects[%d] (%s).services[%d].transport: %q not supported (use \"tcp\" or \"udp\")", i, domain, j, svc.Transport)
			}
			svcType := strings.ToLower(strings.TrimSpace(svc.Type))
			if svcType != "" && !typeRe.MatchString(svcType) {
				return fmt.Errorf("projects[%d] (%s).services[%d].type: %q is not a valid service label", i, domain, j, svc.Type)
			}
			p.Services[j].Type = svcType
			managed := strings.TrimSpace(svc.Command) != ""
			switch {
			case managed && svc.Host != "":
				return fmt.Errorf("projects[%d] (%s).services[%d] (%s): command and host are mutually exclusive (managed or external, not both)", i, domain, j, name)
			case managed:
				if svc.Port < 0 || svc.Port > 65535 {
					return fmt.Errorf("projects[%d] (%s).services[%d] (%s).port: %d out of range", i, domain, j, name, svc.Port)
				}
			case svc.Host != "":
				if net.ParseIP(svc.Host) == nil {
					if _, err := NormalizeDomain(svc.Host); err != nil {
						return fmt.Errorf("projects[%d] (%s).services[%d] (%s).host: invalid endpoint host %q", i, domain, j, name, svc.Host)
					}
				}
				if svc.Port < 1 || svc.Port > 65535 {
					return fmt.Errorf("projects[%d] (%s).services[%d] (%s).port: %d out of range (external services need 1-65535)", i, domain, j, name, svc.Port)
				}
			default:
				return fmt.Errorf("projects[%d] (%s).services[%d] (%s): needs command: (managed) or host: (external)", i, domain, j, name)
			}
		}

		nameCount := map[string]int{}
		for j, r := range p.Records {
			normName, err := NormalizeLabel(r.Name)
			if err != nil {
				return fmt.Errorf("projects[%d] (%s).records[%d]: %w", i, domain, j, err)
			}
			nameCount[normName]++
			if err := ValidateRecord(r); err != nil {
				return fmt.Errorf("projects[%d] (%s).records[%d]: %w", i, domain, j, err)
			}
		}
		for name, n := range nameCount {
			hasCNAME := false
			for _, r := range p.Records {
				norm, _ := NormalizeLabel(r.Name)
				if norm == name && r.Type == "CNAME" {
					hasCNAME = true
				}
			}
			if hasCNAME && n > 1 {
				return fmt.Errorf("projects[%d] (%s): record %q has CNAME conflicting with other records", i, domain, name)
			}
		}
		if p.Run != nil {
			if p.Run.Port < 0 || p.Run.Port > 65535 {
				return fmt.Errorf("projects[%d] (%s).run.port: %d out of range", i, domain, p.Run.Port)
			}
			switch p.Run.Mode {
			case "", "dev":
			default:
				return fmt.Errorf("projects[%d] (%s).run.mode: %q not supported (use \"dev\")", i, domain, p.Run.Mode)
			}
		}
	}
	return nil
}
