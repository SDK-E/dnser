package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SDK-E/dnser/internal/journal"
)

const (
	ResolverDirSuffix = "etc/resolver"
	HealthName        = "probe.dnser.internal"
)

type PlanRequest struct {
	RootDir         string
	ShimPort        int
	ListenerPort    int
	Suffixes        []string
	CAPath          string
	ServiceDef      string
	ServiceName     string
	ServiceTargetFn func() string
}

func resolverPath(rootDir, suffix string) string {
	return filepath.Join(rootDir, ResolverDirSuffix, suffix)
}

func BuildPlan(req PlanRequest) (*journal.Plan, error) {
	if req.ListenerPort == 0 && req.ShimPort == 0 {
		return nil, fmt.Errorf("plan needs a live listener port")
	}
	if len(req.Suffixes) == 0 && req.CAPath == "" && req.ServiceDef == "" {
		return nil, fmt.Errorf("empty elevation plan: nothing to apply")
	}
	p := journal.NewPlan("elevate")
	for _, sfx := range req.Suffixes {
		sfx = strings.TrimPrefix(strings.ToLower(sfx), ".")
		if sfx == "" || strings.Contains(sfx, "/") || strings.Contains(sfx, "..") {
			return nil, fmt.Errorf("unsafe suffix %q", sfx)
		}
		p.Steps = append(p.Steps, &journal.Step{
			ID:   "resolver-" + sfx,
			Kind: journal.KindFileWrite,
			Params: map[string]any{
				"path":    resolverPath(req.RootDir, sfx),
				"content": resolverContent(req.ListenerPort),
				"mode":    0o644,
			},
			Status: journal.StatusPending,
		})
	}
	if req.CAPath != "" {
		p.Steps = append(p.Steps, &journal.Step{
			ID:   "ca-trust",
			Kind: journal.KindCATrust,
			Params: map[string]any{
				"cert": req.CAPath,
			},
			Status: journal.StatusPending,
		})
	}
	if req.ServiceDef != "" {
		target := ""
		if req.ServiceTargetFn != nil {
			target = req.ServiceTargetFn()
		}
		if target == "" {
			return nil, fmt.Errorf("service def set but no install target")
		}
		p.Steps = append(p.Steps, &journal.Step{
			ID:   "service-install",
			Kind: journal.KindServiceInstal,
			Params: map[string]any{
				"name":   req.ServiceName,
				"def":    req.ServiceDef,
				"target": target,
			},
			Status: journal.StatusPending,
		})
	}
	return p, nil
}

func resolverContent(port int) string {
	return fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", port)
}

func ServiceInstallTarget() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "LaunchAgents", "enterprises.sdk.dnser.plist"), nil
	case "linux":
		return filepath.Join("/etc/systemd/user/dnser.service"), nil
	default:
		return "", fmt.Errorf("service install unsupported on %s", runtime.GOOS)
	}
}
