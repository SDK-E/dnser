//go:build windows

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/SDK-E/dnser/internal/setup"
)

type elevRunner struct{}

func newElevatedRunner() setup.Runner {
	return elevRunner{}
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func (elevRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", name, err)
	}
	var inner strings.Builder
	inner.WriteString("& " + psQuote(bin))
	for _, a := range args {
		inner.WriteString(" " + psQuote(a))
	}
	tmp, err := os.CreateTemp("", "dnser-elevate-*.ps1")
	if err != nil {
		return nil, fmt.Errorf("create elevate script: %w", err)
	}
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := tmp.WriteString(inner.String()); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("write elevate script: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("write elevate script: %w", err)
	}
	wrapper := fmt.Sprintf(
		"$p = Start-Process powershell -Verb RunAs -Wait -PassThru -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',%s; exit $p.ExitCode",
		psQuote(path),
	)
	return exec.Command("powershell", "-NoProfile", "-Command", wrapper).CombinedOutput()
}

func elevateAvailable() bool {
	return true
}
