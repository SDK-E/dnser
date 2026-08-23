package config

import (
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
	TLD       string   `json:"tld"`
	Bind      string   `json:"bind"`
	Upstreams []string `json:"upstreams"`
	Autostart bool     `json:"autostart"`
	Ports     Ports    `json:"ports"`
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
	Listen     int      `json:"listen,omitempty"`
	HTTPS      bool     `json:"https,omitempty"`
	ForceHTTPS bool     `json:"force_https,omitempty"`
}

type RunConfig struct {
	Command string `json:"command,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Port    int    `json:"port,omitempty"`
}

type Project struct {
	Domain    string     `json:"domain"`
	Path      string     `json:"path,omitempty"`
	Run       *RunConfig `json:"run,omitempty"`
	Routes    []Route    `json:"routes,omitempty"`
	Records   []Record   `json:"records,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
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

func DashboardDomain(tld string) string {
	return DashboardSubdomain + "." + strings.Trim(strings.TrimSpace(tld), ".")
}
