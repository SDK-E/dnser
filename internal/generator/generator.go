package generator

import (
	"fmt"
	"path/filepath"

	"github.com/SDK-E/dnser/internal/config"
)

type Input struct {
	Project      string
	Root         string
	Dir          string
	Manifest     *config.Manifest
	Port         int
	ServicePorts map[string]int
	LogsDir      string
	DNSPort      int
}

type Output struct {
	Caddyfile  []byte
	Supervisor []byte
	Resolvers  []ResolverReg
	Answers    []Answer
}

func placeholderCtx(in Input) config.PlaceholderCtx {
	svc := map[string]int{}
	for k, v := range in.ServicePorts {
		svc[k] = v
	}
	m := in.Manifest
	return config.PlaceholderCtx{
		Domain:   m.PrimaryDomain(in.Project),
		Port:     in.Port,
		Services: svc,
		LogsDir:  in.LogsDir,
	}
}

func Generate(in Input) (*Output, error) {
	if in.Manifest == nil {
		return nil, fmt.Errorf("manifest required")
	}
	if in.Project == "" || in.Root == "" || in.LogsDir == "" {
		return nil, fmt.Errorf("project, root and logs_dir are required")
	}
	m := in.Manifest
	primary := m.PrimaryDomain(in.Project)
	names := m.EffectiveNames()
	if len(names) == 0 || names[0] != primary {
		names = append([]string{primary}, names...)
	}
	names = dedupeNames(names)

	if err := validateForGenerate(m, names, primary); err != nil {
		return nil, err
	}
	ctx := placeholderCtx(in)

	staticRoot := ""
	if m.ServesStaticFiles() && in.Port == 0 && !m.HasExplicitUpstream() {
		staticRoot = staticServeRoot(in.Dir)
	}
	caddyfile, err := renderCaddyfile(m, names, primary, in.Port, ctx, staticRoot)
	if err != nil {
		return nil, err
	}
	eff, err := effectiveForGenerate(m, in.dirOrRoot(), in.Project)
	if err != nil {
		return nil, err
	}
	if in.Port > 0 {
		eff.Port.Value = in.Port
		if eff.Port.Source == config.SourceDefault {
			eff.Port.Source = config.SourceDetected
		}
	}
	for svcName, sp := range in.ServicePorts {
		svc, ok := eff.Services[svcName]
		if !ok || svc.Port != nil {
			continue
		}
		portCopy := sp
		svc.Port = &portCopy
		eff.Services[svcName] = svc
	}
	eff.InjectRuntimeEnv(in.Port)
	supervisor, err := renderSupervisorYAML(in, eff)
	if err != nil {
		return nil, err
	}
	resolvers := resolverRegistrations(names, in.DNSPort)
	answers, err := answerTable(m, names, ctx.Domain)
	if err != nil {
		return nil, err
	}
	return &Output{
		Caddyfile:  caddyfile,
		Supervisor: supervisor,
		Resolvers:  resolvers,
		Answers:    answers,
	}, nil
}

func effectiveForGenerate(m *config.Manifest, root, project string) (*config.EffectiveConfig, error) {
	eff, err := config.ResolveEffective(m, nil, config.FlagOverrides{})
	if eff != nil && project != "" {
		eff.Domain = config.ResolvedValue[string]{Value: m.PrimaryDomain(project), Source: config.SourceManifest}
	}
	if err != nil {
		return nil, err
	}
	var envPaths []string
	for _, p := range m.EnvFile {
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			envPaths = append(envPaths, p)
			continue
		}
		envPaths = append(envPaths, filepath.Join(root, p))
	}
	envFiles, err := config.LoadEnvFiles(envPaths)
	if err != nil {
		return nil, err
	}
	values, sources := config.MergeEnv(envFiles, m.Env)
	for k, v := range values {
		if _, exists := eff.EnvValues[k]; !exists {
			eff.EnvValues[k] = v
			eff.EnvSources[k] = sources[k]
		}
	}
	return eff, nil
}

func dedupeNames(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, n := range in {
		key := normalizeName(n)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, n)
	}
	return out
}

func (in Input) dirOrRoot() string {
	if in.Dir != "" {
		return in.Dir
	}
	return in.Root
}
