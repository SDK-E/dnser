package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"go.yaml.in/yaml/v4"
)

const (
	AvailabilityAlways    = "always"
	AvailabilityOnRequest = "on_request"
	AvailabilityManual    = "manual"
	defaultIdleStop       = 30 * time.Minute
	defaultMinUptime      = 2 * time.Minute
	DefaultTLD            = "test"
	dnsPortFallback1      = 5353
	dnsPortFallback2      = 35353
)

type Manifest struct {
	Type         string             `yaml:"type,omitempty" json:"type,omitempty"`
	Domain       string             `yaml:"domain,omitempty" json:"domain,omitempty"`
	Domains      []string           `yaml:"domains,omitempty" json:"domains,omitempty"`
	Aliases      []string           `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Port         *int               `yaml:"port,omitempty" json:"port,omitempty"`
	Command      string             `yaml:"command,omitempty" json:"command,omitempty"`
	Shell        Shell              `yaml:"shell,omitempty" json:"shell,omitempty"`
	Cwd          string             `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	HTTPS        HTTPSSetting       `yaml:"https,omitempty" json:"https,omitempty"`
	ForceHTTPS   bool               `yaml:"force_https,omitempty" json:"force_https,omitempty"`
	Env          map[string]string  `yaml:"env,omitempty" json:"env,omitempty"`
	EnvFile      StringList         `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	Services     map[string]Service `yaml:"services,omitempty" json:"services,omitempty"`
	Routes       []Route            `yaml:"routes,omitempty" json:"routes,omitempty"`
	Records      []Record           `yaml:"records,omitempty" json:"records,omitempty"`
	Forward      []Forward          `yaml:"forward,omitempty" json:"forward,omitempty"`
	Process      *RawMap            `yaml:"process,omitempty" json:"process,omitempty"`
	Caddy        *RawMap            `yaml:"caddy,omitempty" json:"caddy,omitempty"`
	Availability string             `yaml:"availability,omitempty" json:"availability,omitempty"`
	IdleStop     Duration           `yaml:"idle_stop,omitempty" json:"idle_stop,omitempty"`
	MinUptime    Duration           `yaml:"min_uptime,omitempty" json:"min_uptime,omitempty"`
}

type Service struct {
	ImageHint string `yaml:"image_hint,omitempty" json:"image_hint,omitempty"`
	Type      string `yaml:"type,omitempty" json:"type,omitempty"`
	Command   string `yaml:"command,omitempty" json:"command,omitempty"`
	Readiness string `yaml:"readiness,omitempty" json:"readiness,omitempty"`
	Host      string `yaml:"host,omitempty" json:"host,omitempty"`
	Port      *int   `yaml:"port,omitempty" json:"port,omitempty"`
	DNS       bool   `yaml:"dns,omitempty" json:"dns,omitempty"`
}

type Route struct {
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	Host    string `yaml:"host,omitempty" json:"host,omitempty"`
	Port    *int   `yaml:"port,omitempty" json:"port,omitempty"`
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
}

type Record struct {
	Name  string `yaml:"name" json:"name"`
	Type  string `yaml:"type" json:"type"`
	Value string `yaml:"value" json:"value"`
}

type Forward struct {
	Proto  string `yaml:"proto" json:"proto"`
	Listen int    `yaml:"listen" json:"listen"`
	To     int    `yaml:"to" json:"to"`
}

type RawMap struct {
	Value map[string]any
}

func (r *RawMap) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping at line %d", node.Line)
	}
	return node.Decode(&r.Value)
}

func (r RawMap) MarshalYAML() (any, error) {
	if r.Value == nil {
		return map[string]any{}, nil
	}
	return r.Value, nil
}

func (r RawMap) MarshalJSON() ([]byte, error) {
	if r.Value == nil {
		return json.Marshal(map[string]any{})
	}
	return json.Marshal(r.Value)
}

func (RawMap) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}

type Shell struct {
	Enabled bool
	Path    string
	Set     bool
}

func (s *Shell) UnmarshalYAML(node *yaml.Node) error {
	s.Set = true
	switch node.Kind {
	case yaml.ScalarNode:
		var raw string
		if err := node.Decode(&raw); err != nil {
			return err
		}
		switch strings.ToLower(raw) {
		case "true":
			s.Enabled, s.Path = true, ""
		case "false":
			s.Enabled, s.Path = false, ""
		default:
			s.Enabled, s.Path = true, raw
		}
	case yaml.MappingNode:
		return fmt.Errorf("shell must be a bool or shell path at line %d", node.Line)
	default:
		var b bool
		if err := node.Decode(&b); err != nil {
			return err
		}
		s.Enabled = b
	}
	return nil
}

func (s Shell) MarshalYAML() (any, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	return s.Enabled, nil
}

func (s Shell) JSONSchema() *jsonschema.Schema {
	one := uint64(1)
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{Type: "string", MinLength: &one},
		},
	}
}

type HTTPSSetting struct {
	Enabled bool
	PerName map[string]bool
	Set     bool
}

func (h *HTTPSSetting) UnmarshalYAML(node *yaml.Node) error {
	h.Set = true
	switch node.Kind {
	case yaml.ScalarNode:
		var b bool
		if err := node.Decode(&b); err != nil {
			return err
		}
		h.Enabled = b
	case yaml.MappingNode:
		h.PerName = map[string]bool{}
		if err := node.Decode(&h.PerName); err != nil {
			return fmt.Errorf("invalid https map at line %d: %w", node.Line, err)
		}
	default:
		return fmt.Errorf("https must be a bool or name→bool map at line %d", node.Line)
	}
	return nil
}

func (h HTTPSSetting) MarshalYAML() (any, error) {
	if h.PerName != nil {
		return h.PerName, nil
	}
	return true, nil
}

func (HTTPSSetting) JSONSchema() *jsonschema.Schema {
	boolValues := &jsonschema.Schema{Type: "boolean"}
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{
				Type:                 "object",
				AdditionalProperties: boolValues,
				PropertyNames:        &jsonschema.Schema{Type: "string"},
			},
		},
	}
}

type StringList []string

func (s *StringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var out []string
		if err := node.Decode(&out); err != nil {
			return err
		}
		*s = out
	case yaml.ScalarNode:
		var single string
		if err := node.Decode(&single); err != nil {
			return err
		}
		*s = StringList{single}
	default:
		return fmt.Errorf("expected string or list at line %d", node.Line)
	}
	return nil
}

type Duration struct {
	Value time.Duration
	Set   bool
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q at line %d: %w", raw, node.Line, err)
	}
	d.Value, d.Set = parsed, true
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.Value.String(), nil
}

func (Duration) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Description: "Go duration string, e.g. 30m or 2m",
		Pattern:     `^[0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h)+$`,
	}
}

func (m *Manifest) EffectiveIdleStop() time.Duration {
	if m.IdleStop.Set {
		return m.IdleStop.Value
	}
	return defaultIdleStop
}

func (m *Manifest) EffectiveMinUptime() time.Duration {
	if m.MinUptime.Set {
		return m.MinUptime.Value
	}
	return defaultMinUptime
}

func (m *Manifest) PrimaryDomain(dirname string) string {
	switch {
	case m.Domain != "":
		return m.Domain
	case len(m.Domains) > 0:
		return m.Domains[0]
	default:
		return fmt.Sprintf("%s.%s", sanitizeLabel(dirname), DefaultTLD)
	}
}

func (m *Manifest) EffectiveNames() []string {
	var names []string
	if m.Domain != "" {
		names = append(names, m.Domain)
	}
	names = append(names, m.Domains...)
	names = append(names, m.Aliases...)
	return dedupe(names)
}

func (m *Manifest) Validate() error {
	switch m.Availability {
	case "", AvailabilityAlways, AvailabilityOnRequest, AvailabilityManual:
	default:
		return fmt.Errorf("availability must be one of always|on_request|manual, got %q", m.Availability)
	}
	for i, f := range m.Forward {
		switch strings.ToLower(f.Proto) {
		case "tcp", "udp":
		default:
			return fmt.Errorf("forward[%d].proto must be tcp|udp, got %q", i, f.Proto)
		}
		if f.Listen < 1 || f.Listen > 65535 || f.To < 1 || f.To > 65535 {
			return fmt.Errorf("forward[%d] ports must be in 1-65535", i)
		}
		if m.Availability == AvailabilityOnRequest {
			return fmt.Errorf("forward[%d]: push-protocol frontends require availability: always (on_request is valid only for HTTP)", i)
		}
	}
	for name, s := range m.Services {
		if s.Port != nil && (*s.Port < 1 || *s.Port > 65535) {
			return fmt.Errorf("service %q port must be in 1-65535", name)
		}
	}
	for _, r := range m.Records {
		switch strings.ToUpper(r.Type) {
		case "A", "AAAA", "TXT", "CNAME", "SRV", "MX":
		default:
			return fmt.Errorf("record %q has unsupported type %q", r.Name, r.Type)
		}
	}
	if m.Port != nil && (*m.Port < 1 || *m.Port > 65535) {
		return fmt.Errorf("port must be in 1-65535")
	}
	for name := range m.Services {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("service names must be non-empty")
		}
	}
	return nil
}

func sanitizeLabel(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	return out
}
