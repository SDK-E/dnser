package config

import (
	"fmt"
	"net"
	"strings"
)

var AllowedRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "TXT": true,
	"MX": true, "SRV": true, "NS": true,
}

func NormalizeDomain(s string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ".")))
	if d == "" {
		return "", fmt.Errorf("empty domain")
	}
	if strings.HasPrefix(d, "*.") {
		base, err := NormalizeDomain(d[2:])
		if err != nil {
			return "", err
		}
		return "*." + base, nil
	}
	if len(d) > 253 {
		return "", fmt.Errorf("domain %q exceeds 253 characters", d)
	}
	labels := strings.Split(d, ".")
	for _, l := range labels {
		if l == "" {
			return "", fmt.Errorf("domain %q has empty label", d)
		}
		if len(l) > 63 {
			return "", fmt.Errorf("label %q in %q exceeds 63 characters", l, d)
		}
		for _, r := range l {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-' || r == '_':
			default:
				return "", fmt.Errorf("invalid character %q in domain %q", string(r), d)
			}
		}
		if strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return "", fmt.Errorf("label %q in %q may not start or end with hyphen", l, d)
		}
	}
	return d, nil
}

func IsWildcard(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

func WildcardBase(domain string) string {
	if IsWildcard(domain) {
		return domain[2:]
	}
	return domain
}

func EnsureTLD(domain, tld string) (string, error) {
	norm, err := NormalizeDomain(domain)
	if err != nil {
		return "", err
	}
	t := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(tld), "."))
	if t == "" || strings.HasSuffix(norm, "."+t) || norm == t {
		return norm, nil
	}
	if IsWildcard(norm) {
		base := WildcardBase(norm)
		if strings.HasSuffix(base, "."+t) || base == t {
			return norm, nil
		}
		return "*." + base + "." + t, nil
	}
	return norm + "." + t, nil
}

func NormalizeLabel(name string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(name))
	if l == "" || l == "@" {
		return "@", nil
	}
	if l == "*" {
		return "*", nil
	}
	if strings.HasPrefix(l, "*.") {
		rest, err := NormalizeLabel(l[2:])
		if err != nil || rest == "@" {
			if err != nil {
				return "", err
			}
			return "*", nil
		}
		return "*." + rest, nil
	}
	labels := strings.Split(l, ".")
	for _, part := range labels {
		if part == "" {
			return "", fmt.Errorf("record name %q has empty label", name)
		}
		for _, r := range part {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-' || r == '_':
			default:
				return "", fmt.Errorf("invalid character %q in record name %q", string(r), name)
			}
		}
	}
	return l, nil
}

func ValidateRecord(r Record) error {
	if !AllowedRecordTypes[r.Type] {
		return fmt.Errorf("record type %q not supported (allowed: A AAAA CNAME TXT MX SRV NS)", r.Type)
	}
	name, err := NormalizeLabel(r.Name)
	if err != nil {
		return err
	}
	r.Name = name
	value := strings.TrimSpace(r.Value)
	switch r.Type {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("%s record %s: %q is not an IPv4 address", r.Type, name, value)
		}
	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("%s record %s: %q is not an IPv6 address", r.Type, name, value)
		}
	case "CNAME", "NS":
		target, err := NormalizeDomain(value)
		if err != nil {
			return fmt.Errorf("%s record %s: invalid target: %w", r.Type, name, err)
		}
		if IsWildcard(target) {
			return fmt.Errorf("%s record %s: wildcard target %q not allowed", r.Type, name, target)
		}
	case "TXT":
		if value == "" {
			return fmt.Errorf("TXT record %s: empty value", name)
		}
		if len(value) > 255*8 {
			return fmt.Errorf("TXT record %s: value too long", name)
		}
	case "MX":
		if _, err := NormalizeDomain(value); err != nil {
			return fmt.Errorf("MX record %s: invalid exchange host: %w", name, err)
		}
	case "SRV":
		if _, err := NormalizeDomain(value); err != nil {
			return fmt.Errorf("SRV record %s: invalid target: %w", name, err)
		}
		if r.Port == 0 {
			return fmt.Errorf("SRV record %s: port required", name)
		}
	}
	return nil
}

func RecordFQDN(zone, label string) string {
	if label == "@" {
		return zone
	}
	if strings.HasPrefix(label, "*.") {
		return "*." + zone
	}
	return label + "." + zone
}
