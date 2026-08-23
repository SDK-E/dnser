package config

import (
	"fmt"
	"net"
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
		if p.Port < 0 || p.Port > 65535 {
			return fmt.Errorf("projects[%d] (%s): port %d out of range", i, domain, p.Port)
		}
		aliasSeen := map[string]bool{}
		for j, a := range p.Aliases {
			alias, err := NormalizeDomain(a)
			if err != nil {
				return fmt.Errorf("projects[%d] (%s).aliases[%d]: %w", i, domain, j, err)
			}
			if aliasSeen[alias] || seen[alias] {
				return fmt.Errorf("projects[%d] (%s): duplicate alias %q", i, domain, alias)
			}
			aliasSeen[alias] = true
			seen[alias] = true
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
			if (r.Type == "CNAME" || r.Type == "NS") && normName == "@" && len(p.Records) > 1 {
				continue
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
	}
	return nil
}
