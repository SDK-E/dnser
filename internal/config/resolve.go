package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	EnvPort          = "PORT"
	EnvDomain        = "DNSER_DOMAIN"
	envServicePrefix = "DNSER_SERVICES_"
)

type Source int

const (
	SourceDefault Source = iota
	SourceTemplate
	SourceDetected
	SourceManifest
	SourceFlag
)

func (s Source) String() string {
	switch s {
	case SourceFlag:
		return "flag"
	case SourceManifest:
		return "manifest"
	case SourceDetected:
		return "detected"
	case SourceTemplate:
		return "template"
	default:
		return "default"
	}
}

type ResolvedValue[T any] struct {
	Value  T
	Source Source
}

type FlagOverrides struct {
	Port    *int
	Domain  string
	Command string
	Type    string
	Redact  *bool
}

func ResolveEffective(m *Manifest, tmpl *Template, flags FlagOverrides) (*EffectiveConfig, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest required")
	}
	eff := &EffectiveConfig{
		Sources: map[string]string{},
	}

	if flags.Type != "" {
		eff.Type = ResolvedValue[string]{Value: flags.Type, Source: SourceFlag}
	} else if m.Type != "" {
		eff.Type = ResolvedValue[string]{Value: m.Type, Source: SourceManifest}
	} else if tmpl != nil && tmpl.Name != "" {
		eff.Type = ResolvedValue[string]{Value: tmpl.Name, Source: SourceTemplate}
	}

	if flags.Domain != "" {
		eff.Domain = ResolvedValue[string]{Value: flags.Domain, Source: SourceFlag}
	} else {
		eff.Domain = ResolvedValue[string]{Value: m.PrimaryDomain(""), Source: SourceManifest}
	}

	names := m.EffectiveNames()
	if len(names) == 0 || names[0] != eff.Domain.Value {
		names = append([]string{eff.Domain.Value}, names...)
	}
	eff.Names = dedupe(names)

	if flags.Port != nil {
		eff.Port = ResolvedValue[int]{Value: *flags.Port, Source: SourceFlag}
	} else if m.Port != nil {
		eff.Port = ResolvedValue[int]{Value: *m.Port, Source: SourceManifest}
	}

	if flags.Command != "" {
		eff.Command = ResolvedValue[string]{Value: flags.Command, Source: SourceFlag}
	} else if m.Command != "" {
		eff.Command = ResolvedValue[string]{Value: m.Command, Source: SourceManifest}
	} else if tmpl != nil && tmpl.Command != "" {
		eff.Command = ResolvedValue[string]{Value: tmpl.Command, Source: SourceTemplate}
	}

	switch {
	case m.Shell.Set:
		eff.Shell = m.Shell
	case tmpl != nil && tmpl.Shell.Set:
		eff.Shell = tmpl.Shell
	default:
		eff.Shell = Shell{Enabled: true}
	}

	eff.ForceHTTPS = m.ForceHTTPS

	switch {
	case m.Availability != "":
		eff.Availability = ResolvedValue[string]{Value: m.Availability, Source: SourceManifest}
	case tmpl != nil && tmpl.Availability != "":
		eff.Availability = ResolvedValue[string]{Value: tmpl.Availability, Source: SourceTemplate}
	default:
		eff.Availability = ResolvedValue[string]{Value: AvailabilityAlways, Source: SourceDefault}
	}

	envTuning := map[string]string{}
	tuningSources := map[string]string{}
	if tmpl != nil {
		for k, v := range tmpl.Env {
			envTuning[k] = v
			tuningSources[k] = SourceTemplate.String()
		}
	}
	for k, v := range m.Env {
		envTuning[k] = v
		tuningSources[k] = SourceManifest.String()
	}
	eff.EnvValues = envTuning
	eff.EnvSources = tuningSources

	eff.Services = map[string]Service{}
	for name, s := range m.Services {
		eff.Services[name] = s
	}
	return eff, nil
}

func (e *EffectiveConfig) ApplyPlaceholders(ctx PlaceholderCtx) error {
	sub, err := SubstituteStrings(e.Command.Value, ctx)
	if err != nil {
		return fmt.Errorf("command: %w", err)
	}
	e.Command.Value = sub.(string)
	for name, s := range e.Services {
		cmd, err := SubstituteStrings(s.Command, ctx)
		if err != nil {
			return fmt.Errorf("service %s command: %w", name, err)
		}
		s.Command = cmd.(string)
		rdy, err := SubstituteStrings(s.Readiness, ctx)
		if err != nil {
			return fmt.Errorf("service %s readiness: %w", name, err)
		}
		s.Readiness = rdy.(string)
		e.Services[name] = s
	}
	return nil
}

type EffectiveConfig struct {
	Type         ResolvedValue[string]
	Domain       ResolvedValue[string]
	Names        []string
	Port         ResolvedValue[int]
	Command      ResolvedValue[string]
	Shell        Shell
	ForceHTTPS   bool
	Availability ResolvedValue[string]
	EnvValues    map[string]string
	EnvSources   map[string]string
	Services     map[string]Service
	Sources      map[string]string
}

func (e *EffectiveConfig) InjectRuntimeEnv(port int) {
	if e.EnvValues == nil {
		e.EnvValues = map[string]string{}
	}
	if e.EnvSources == nil {
		e.EnvSources = map[string]string{}
	}
	if _, userSetPort := e.EnvValues[EnvPort]; !userSetPort && port != 0 {
		e.EnvValues[EnvPort] = strconv.Itoa(port)
		e.EnvSources[EnvPort] = SourceDefault.String()
	}
	if _, ok := e.EnvValues[EnvDomain]; !ok && e.Domain.Value != "" {
		e.EnvValues[EnvDomain] = e.Domain.Value
		e.EnvSources[EnvDomain] = SourceDefault.String()
	}
	for name := range e.Services {
		key := envServicePrefix + strings.ToUpper(name)
		if _, ok := e.EnvValues[key]; !ok {
			e.EnvValues[key] = serviceAddr(e.Services[name])
			e.EnvSources[key] = SourceDefault.String()
		}
	}
}

func serviceAddr(s Service) string {
	host := s.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if s.Port != nil {
		return fmt.Sprintf("%s:%d", host, *s.Port)
	}
	return host
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func ObservePortFromEnv() (int, bool) {
	v := os.Getenv(EnvPort)
	if v == "" {
		return 0, false
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}
