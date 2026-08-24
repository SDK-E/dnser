package config

import (
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const CurrentVersion = 2

const (
	DefaultTLD         = "test"
	DashboardSubdomain = "dnser"
)

type Ports struct {
	DNS   int `json:"dns"`
	HTTP  int `json:"http"`
	HTTPS int `json:"https"`
	UI    int `json:"ui"`
}

type Settings struct {
	TLD             string   `json:"tld"`
	Bind            string   `json:"bind"`
	Upstreams       []string `json:"upstreams"`
	Autostart       bool     `json:"autostart"`
	Ports           Ports    `json:"ports"`
	ForceHTTPS      bool     `json:"force_https,omitempty"`
	PathRefreshMins int      `json:"path_refresh_minutes,omitempty"`
}

const DefaultPathRefreshMins = 1440

func (s Settings) PathRefresh() int {
	if s.PathRefreshMins > 0 {
		return s.PathRefreshMins
	}
	return DefaultPathRefreshMins
}

type Record struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      uint32 `json:"ttl,omitempty"`
	Priority uint16 `json:"priority,omitempty"`
	Weight   uint16 `json:"weight,omitempty"`
	Port     uint16 `json:"port,omitempty"`
}

type Route struct {
	Host       string   `json:"host"`
	Backends   []string `json:"backends"`
	TCP        bool     `json:"tcp,omitempty"`
	UDP        bool     `json:"udp,omitempty"`
	Listen     int      `json:"listen,omitempty"`
	HTTPS      bool     `json:"https,omitempty"`
	ForceHTTPS bool     `json:"force_https,omitempty"`
	Paths      []string `json:"paths,omitempty"`
}

type Service struct {
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Command   string `json:"command,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	Transport string `json:"transport,omitempty"`
	DNS       bool   `json:"dns,omitempty"`
}

func (s Service) Managed() bool { return strings.TrimSpace(s.Command) != "" }

func (s Service) Endpoint() string {
	host := s.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(s.Port))
}

func (s Service) IsUDP() bool { return s.Transport == "udp" }

type RunConfig struct {
	Command string `json:"command,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Port    int    `json:"port,omitempty"`
}

type Project struct {
	Domain    string     `json:"domain"`
	Path      string     `json:"path,omitempty"`
	Run       *RunConfig `json:"run,omitempty"`
	Services  []Service  `json:"services,omitempty"`
	Routes    []Route    `json:"routes,omitempty"`
	Records   []Record   `json:"records,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (p Project) ServicePortMap() map[string]int {
	ports := map[string]int{}
	if p.Run != nil && p.Run.Port > 0 {
		ports[""] = p.Run.Port
	}
	for _, svc := range p.Services {
		if svc.Port > 0 {
			ports[svc.Name] = svc.Port
		}
	}
	return ports
}

type Config struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Projects []Project `json:"projects"`
}

func ResolveHost(host, domain, tld string) string {
	switch host {
	case "@", "":
		return domain
	case "*":
		return "*." + domain
	}
	host = strings.ToLower(host)
	if strings.Contains(host, ".") && tld != "" && !strings.HasSuffix(domain, "."+host) {
		if host == tld || strings.HasSuffix(host, "."+tld) {
			return strings.TrimSuffix(host, ".")
		}
	}
	return host + "." + domain
}

func (r Route) Hostname(domain, tld string) string {
	return ResolveHost(r.Host, domain, tld)
}

func (r Route) EffectiveForceHTTPS(global bool) bool {
	return r.ForceHTTPS || (global && r.HTTPS)
}

func (r Route) Forwarded() bool { return r.TCP || r.UDP }

func DashboardDomain(tld string) string {
	return DashboardSubdomain + "." + strings.Trim(strings.TrimSpace(tld), ".")
}

func NormalizePathPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p == "/" {
		return ""
	}
	return p
}

var portPlaceholderRe = regexp.MustCompile(`^\{port(?::[A-Za-z0-9_-]+)?\}$`)

func IsPortPlaceholder(s string) bool { return portPlaceholderRe.MatchString(s) }

var plainPortRe = regexp.MustCompile(`^\d+$`)

func splitBackend(b string) (string, string, error) {
	if idx := strings.LastIndex(b, ":{"); idx >= 0 {
		port := b[idx+1:]
		if IsPortPlaceholder(port) {
			return b[:idx], port, nil
		}
	}
	if i := strings.LastIndexByte(b, ':'); i > 0 {
		port := b[i+1:]
		if plainPortRe.MatchString(port) || IsPortPlaceholder(port) {
			return b[:i], port, nil
		}
	}
	return net.SplitHostPort(b)
}
