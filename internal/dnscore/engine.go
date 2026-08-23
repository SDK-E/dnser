package dnscore

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

type zone struct {
	name     string
	wildcard bool
	labels   map[string]bool
	records  map[string][]config.Record
}

type Match struct {
	Zone   *zone
	Name   string
	Rel    string
	Record []config.Record
}

type Engine struct {
	bindIP     string
	zones      map[string]*zone
	dashDomain string
}

func NewEngine(cfg config.Config) *Engine {
	e := &Engine{
		bindIP: cfg.Settings.Bind,
		zones:  make(map[string]*zone),
	}
	addZone := func(name string, z *zone) {
		if _, exists := e.zones[name]; !exists {
			e.zones[name] = z
		}
	}
	tld := cfg.Settings.TLD
	for _, p := range cfg.Projects {
		z := &zone{
			name:    p.Domain,
			labels:  make(map[string]bool),
			records: make(map[string][]config.Record),
		}
		for _, r := range p.Records {
			z.records[r.Name] = append(z.records[r.Name], r)
		}
		for _, route := range p.Routes {
			switch {
			case route.Host == "*" || route.Host == "*."+p.Domain:
				z.wildcard = true
			case route.Host == "@" || route.Host == "":
			case !strings.Contains(route.Host, "."):
				z.labels[route.Host] = true
			default:
				host := config.ResolveHost(route.Host, p.Domain, tld)
				if host != p.Domain && !strings.HasPrefix(host, "*.") {
					addZone(host, &zone{name: host, labels: map[string]bool{}, records: map[string][]config.Record{}})
				}
			}
		}
		addZone(p.Domain, z)
	}
	dash := config.DashboardDomain(cfg.Settings.TLD)
	e.zones[dash] = &zone{name: dash}
	e.dashDomain = dash
	return e
}

func (e *Engine) BindIP() string { return e.bindIP }

func (e *Engine) DashboardDomain() string { return e.dashDomain }

func (e *Engine) match(name string) *Match {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if name == "" {
		return nil
	}
	if z, ok := e.zones[name]; ok {
		return &Match{Zone: z, Name: name, Rel: "@", Record: z.records["@"]}
	}
	parts := strings.Split(name, ".")
	for i := 1; i < len(parts); i++ {
		z, ok := e.zones[strings.Join(parts[i:], ".")]
		if !ok {
			continue
		}
		rel := strings.Join(parts[:i], ".")
		if recs := z.records[rel]; len(recs) > 0 {
			return &Match{Zone: z, Name: name, Rel: rel, Record: recs}
		}
		if z.wildcard || z.labels[rel] {
			return &Match{Zone: z, Name: name, Rel: rel}
		}
		return nil
	}
	return nil
}

func (e *Engine) Resolve(qname string, qtype uint16) ([]dns.RR, bool) {
	m := e.match(qname)
	if m == nil {
		return nil, false
	}
	fqdn := dns.Fqdn(m.Name)
	var out []dns.RR
	for _, r := range m.Record {
		rrs, err := RenderRecord(fqdn, r)
		if err != nil {
			continue
		}
		out = append(out, rrs...)
	}
	if !hasAny(out, dns.TypeA, dns.TypeAAAA, dns.TypeCNAME) {
		if rr, err := dns.NewRR(fmt.Sprintf("%s %d IN A %s", fqdn, DefaultTTL, e.bindIP)); err == nil {
			out = append(out, rr)
		}
	}
	filtered := filterType(out, qtype)
	return filtered, true
}

func (e *Engine) Owns(name string) bool {
	return e.match(name) != nil
}

func hasAny(rrs []dns.RR, types ...uint16) bool {
	for _, rr := range rrs {
		for _, t := range types {
			if rr.Header().Rrtype == t {
				return true
			}
		}
	}
	return false
}

func filterType(rrs []dns.RR, qtype uint16) []dns.RR {
	if qtype == dns.TypeANY || qtype == dns.TypeNone {
		return rrs
	}
	var out []dns.RR
	for _, rr := range rrs {
		if rr.Header().Rrtype == qtype {
			out = append(out, rr)
		}
	}
	return out
}
