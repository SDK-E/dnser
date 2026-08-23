package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

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
				h, portStr, err := net.SplitHostPort(strings.TrimSpace(b))
				if err != nil {
					return fmt.Errorf("projects[%d] (%s).routes[%d].backends[%d]: %q must be host:port", i, domain, j, k, b)
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
			if route.TCP {
				if route.Listen < 1 || route.Listen > 65535 {
					return fmt.Errorf("projects[%d] (%s).routes[%d] (tcp): listen port required (1-65535)", i, domain, j)
				}
				if owner, clash := tcpListens[route.Listen]; clash {
					return fmt.Errorf("projects[%d] (%s).routes[%d]: tcp listen %d already used by %s", i, domain, j, route.Listen, owner)
				}
				tcpListens[route.Listen] = fmt.Sprintf("projects[%d] (%s).routes[%d]", i, domain, j)
			} else if route.Listen != 0 {
				return fmt.Errorf("projects[%d] (%s).routes[%d]: listen is only valid for tcp routes", i, domain, j)
			}
			if route.ForceHTTPS && !route.HTTPS {
				return fmt.Errorf("projects[%d] (%s).routes[%d] (%s): force_https requires https", i, domain, j, resolved)
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
			switch p.Run.Mode {
			case "", "dev":
			default:
				return fmt.Errorf("projects[%d] (%s).run.mode: %q not supported (use \"dev\")", i, domain, p.Run.Mode)
			}
		}
	}
	return nil
}
