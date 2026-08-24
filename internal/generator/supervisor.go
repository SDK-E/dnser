package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SDK-E/dnser/internal/config"
	"go.yaml.in/yaml/v4"
)

type pcProcess struct {
	Command        string          `yaml:"command,omitempty"`
	Disabled       bool            `yaml:"disabled,omitempty"`
	Environment    []string        `yaml:"environment,omitempty"`
	WorkingDir     string          `yaml:"working_dir,omitempty"`
	LogLocation    string          `yaml:"log_location,omitempty"`
	Availability   *pcAvailability `yaml:"availability,omitempty"`
	Shutdown       *pcShutdown     `yaml:"shutdown,omitempty"`
	ReadinessProbe *pcProbe        `yaml:"readiness_probe,omitempty"`
	DependsOn      map[string]any  `yaml:"depends_on,omitempty"`
}

type pcAvailability struct {
	Restart        string `yaml:"restart,omitempty"`
	BackoffSeconds int    `yaml:"backoff_seconds,omitempty"`
	MaxRestarts    int    `yaml:"max_restarts,omitempty"`
}

type pcShutdown struct {
	Signal         int `yaml:"signal,omitempty"`
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty"`
}

type pcProbe struct {
	Exec          *pcExecProbe `yaml:"exec,omitempty"`
	HttpGet       *pcHTTPProbe `yaml:"http_get,omitempty"`
	PeriodSeconds int          `yaml:"period_seconds,omitempty"`
}

type pcExecProbe struct {
	Command string `yaml:"command"`
}

type pcHTTPProbe struct {
	Scheme  string `yaml:"scheme,omitempty"`
	Host    string `yaml:"host,omitempty"`
	NumPort int    `yaml:"num_port,omitempty"`
	Path    string `yaml:"path,omitempty"`
}

const (
	defaultShutdownSignal   = 15
	defaultShutdownTimeout  = 10
	defaultRestartOnFailure = "on_failure"
	restartNo               = "no"
)

func renderSupervisorYAML(in Input, eff *config.EffectiveConfig) ([]byte, error) {
	if eff.Command.Value == "" && len(eff.Services) == 0 {
		return nil, nil
	}
	ctx := placeholderCtx(in)
	processes := map[string]any{}
	project := map[string]any{
		"version":   "0.5",
		"log_level": "info",
		"processes": processes,
	}

	if eff.Command.Value != "" {
		substituted, err := config.SubstituteStrings(eff.Command.Value, ctx)
		if err != nil {
			return nil, fmt.Errorf("command placeholders: %w", err)
		}
		proc := pcProcess{
			Command:     substituted.(string),
			WorkingDir:  workingDir(in),
			Environment: envList(eff),
			Availability: &pcAvailability{
				Restart: restartForTier(eff.Availability.Value),
			},
			Shutdown: &pcShutdown{
				Signal:         defaultShutdownSignal,
				TimeoutSeconds: defaultShutdownTimeout,
			},
			LogLocation: fmt.Sprintf("%s/%s.log", strings.TrimSuffix(in.LogsDir, "/"), in.Project),
		}
		if eff.Availability.Value == config.AvailabilityManual || eff.Availability.Value == config.AvailabilityOnRequest {
			proc.Disabled = true
		}
		data, err := applyProcessRaw(in.Manifest, proc)
		if err != nil {
			return nil, err
		}
		var procMap map[string]any
		if err := yaml.Unmarshal(data, &procMap); err != nil {
			return nil, fmt.Errorf("process %s: %w", in.Project, err)
		}
		processes[in.Project] = procMap
	}

	for _, svcName := range sortedKeys(eff.Services) {
		svc := eff.Services[svcName]
		if svc.Command == "" || svc.Host != "" {
			continue
		}
		cmd, err := config.SubstituteStrings(svc.Command, ctx)
		if err != nil {
			return nil, fmt.Errorf("service %s command: %w", svcName, err)
		}
		proc := pcProcess{
			Command:      cmd.(string),
			WorkingDir:   workingDir(in),
			Environment:  []string{},
			Availability: &pcAvailability{Restart: defaultRestartOnFailure},
			Shutdown: &pcShutdown{
				Signal:         defaultShutdownSignal,
				TimeoutSeconds: defaultShutdownTimeout,
			},
			LogLocation: fmt.Sprintf("%s/%s-%s.log", strings.TrimSuffix(in.LogsDir, "/"), in.Project, svcName),
		}
		if probe := readinessProbe(svc.Readiness, ctx); probe != nil {
			proc.ReadinessProbe = probe
		}
		data, err := applyServiceRaw(in.Manifest, svcName, proc)
		if err != nil {
			return nil, err
		}
		var procMap map[string]any
		if err := yaml.Unmarshal(data, &procMap); err != nil {
			return nil, fmt.Errorf("service %s: %w", svcName, err)
		}
		processes[keyFor(svcName)] = procMap
	}

	out, err := yaml.Marshal(project)
	if err != nil {
		return nil, fmt.Errorf("marshal process-compose project: %w", err)
	}
	return out, nil
}

func keyFor(service string) string {
	return service
}

func workingDir(in Input) string {
	cwd := in.Root
	if in.Manifest.Cwd != "" {
		cwd = in.Root + "/" + in.Manifest.Cwd
	}
	return cwd
}

func envList(eff *config.EffectiveConfig) []string {
	values := make(map[string]string, len(eff.EnvValues))
	for k, v := range eff.EnvValues {
		values[k] = v
	}
	keys := sortedKeys(values)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+values[k])
	}
	return out
}

func restartForTier(tier string) string {
	switch tier {
	case config.AvailabilityManual, config.AvailabilityOnRequest:
		return restartNo
	default:
		return defaultRestartOnFailure
	}
}

func readinessProbe(spec string, ctx config.PlaceholderCtx) *pcProbe {
	if spec == "" {
		return nil
	}
	substituted, err := config.SubstituteStrings(spec, ctx)
	if err != nil {
		substituted = spec
	}
	s := substituted.(string)
	switch {
	case strings.HasPrefix(s, "tcp://"):
		addr := strings.TrimPrefix(s, "tcp://")
		host, port, ok := splitHostPort(addr)
		if !ok {
			return &pcProbe{Exec: &pcExecProbe{Command: "nc -z " + addr}}
		}
		return &pcProbe{Exec: &pcExecProbe{Command: fmt.Sprintf("nc -z %s %s", host, port)}}
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"):
		raw := s
		scheme := "http"
		if strings.HasPrefix(raw, "https://") {
			scheme = "https"
			raw = strings.TrimPrefix(raw, "https://")
		} else {
			raw = strings.TrimPrefix(raw, "http://")
		}
		authority, path := raw, "/"
		if i := strings.Index(raw, "/"); i >= 0 {
			authority, path = raw[:i], raw[i:]
		}
		host, portStr, ok := splitHostPort(authority)
		if !ok || !portIsNumeric(portStr) {
			return nil
		}
		numPort, _ := strconv.Atoi(portStr)
		return &pcProbe{
			HttpGet: &pcHTTPProbe{
				Scheme:  scheme,
				Host:    host,
				NumPort: numPort,
				Path:    path,
			},
		}
	default:
		return &pcProbe{Exec: &pcExecProbe{Command: s}}
	}
}

func portIsNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitHostPort(addr string) (string, string, bool) {
	if addr == "" {
		return "", "", false
	}
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", "", false
	}
	host, port := addr[:i], addr[i+1:]
	if host == "" || port == "" || strings.ContainsAny(host, "{}") || !portIsNumeric(port) {
		return "", "", false
	}
	return host, port, true
}

func applyProcessRaw(m *config.Manifest, proc pcProcess) ([]byte, error) {
	base, err := yaml.Marshal(proc)
	if err != nil {
		return nil, err
	}
	var baseMap map[string]any
	if err := yaml.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if m.Process != nil && len(m.Process.Value) > 0 {
		raw = m.Process.Value
	}
	if len(raw) == 0 {
		return base, nil
	}
	merged, err := DeepMerge(baseMap, raw)
	if err != nil {
		return nil, fmt.Errorf("process raw merge: %w", err)
	}
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func applyServiceRaw(m *config.Manifest, service string, proc pcProcess) ([]byte, error) {
	base, err := yaml.Marshal(proc)
	if err != nil {
		return nil, err
	}
	if m.Process == nil {
		return base, nil
	}
	svcKey := "service_" + service
	rawAny, ok := m.Process.Value[svcKey]
	if !ok {
		return base, nil
	}
	rawMap, ok := rawAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("process.%s must be a mapping", svcKey)
	}
	var baseMap map[string]any
	if err := yaml.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}
	merged, err := DeepMerge(baseMap, rawMap)
	if err != nil {
		return nil, fmt.Errorf("process.%s merge: %w", svcKey, err)
	}
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}
	return out, nil
}
