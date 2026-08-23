package config

import (
	"strings"
	"time"
)

const CurrentVersion = 1

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

type Project struct {
	Domain    string    `json:"domain"`
	Port      int       `json:"port"`
	Wildcard  bool      `json:"wildcard"`
	HTTPS     bool      `json:"https"`
	Aliases   []string  `json:"aliases,omitempty"`
	Records   []Record  `json:"records,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Config struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Projects []Project `json:"projects"`
}

func (p Project) AllHostnames() []string {
	out := make([]string, 0, 1+len(p.Aliases))
	out = append(out, p.Domain)
	out = append(out, p.Aliases...)
	return out
}

func DashboardDomain(tld string) string {
	return DashboardSubdomain + "." + strings.Trim(strings.TrimSpace(tld), ".")
}
