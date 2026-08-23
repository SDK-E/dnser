package dnscore

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

const DefaultTTL = 120

func ttlOr(r config.Record, fallback uint32) uint32 {
	if r.TTL > 0 {
		return r.TTL
	}
	return fallback
}

func RenderRecord(fqdn string, r config.Record) ([]dns.RR, error) {
	switch r.Type {
	case "A", "AAAA", "CNAME", "NS":
		rr, err := dns.NewRR(fmt.Sprintf("%s %d IN %s %s", fqdn, ttlOr(r, DefaultTTL), r.Type, r.Value))
		if err != nil {
			return nil, fmt.Errorf("render %s record: %w", r.Type, err)
		}
		return []dns.RR{rr}, nil
	case "TXT":
		var out []dns.RR
		for _, chunk := range splitTXT(r.Value, 255) {
			rr, err := dns.NewRR(fmt.Sprintf("%s %d IN TXT %q", fqdn, ttlOr(r, DefaultTTL), chunk))
			if err != nil {
				return nil, fmt.Errorf("render TXT record: %w", err)
			}
			out = append(out, rr)
		}
		return out, nil
	case "MX":
		rr, err := dns.NewRR(fmt.Sprintf("%s %d IN MX %d %s", fqdn, ttlOr(r, DefaultTTL), r.Priority, r.Value))
		if err != nil {
			return nil, fmt.Errorf("render MX record: %w", err)
		}
		return []dns.RR{rr}, nil
	case "SRV":
		rr, err := dns.NewRR(fmt.Sprintf("%s %d IN SRV %d %d %d %s",
			fqdn, ttlOr(r, DefaultTTL), r.Priority, r.Weight, r.Port, r.Value))
		if err != nil {
			return nil, fmt.Errorf("render SRV record: %w", err)
		}
		return []dns.RR{rr}, nil
	default:
		return nil, fmt.Errorf("render: unsupported record type %q", r.Type)
	}
}

func splitTXT(s string, limit int) []string {
	if len(s) <= limit {
		return []string{s}
	}
	var out []string
	for len(s) > limit {
		out = append(out, s[:limit])
		s = s[limit:]
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func AnswerSummary(resp *dns.Msg) string {
	if resp == nil || len(resp.Answer) == 0 {
		if resp != nil && resp.Rcode == dns.RcodeNameError {
			return "NXDOMAIN"
		}
		return "NODATA"
	}
	parts := make([]string, 0, 3)
	for i, rr := range resp.Answer {
		if i >= 3 {
			parts = append(parts, fmt.Sprintf("+%d more", len(resp.Answer)-3))
			break
		}
		parts = append(parts, compactRR(rr))
	}
	return strings.Join(parts, ", ")
}

func compactRR(rr dns.RR) string {
	s := rr.String()
	if i := strings.Index(s, "\tIN\t"); i >= 0 {
		s = strings.TrimPrefix(s[i:], "\tIN\t")
	}
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		return fields[0] + " " + fields[1]
	}
	return s
}
