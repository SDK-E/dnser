//go:build linux

package desktop

import (
	"fmt"
	"os/exec"

	"github.com/SDK-E/dnser/internal/setup"
)

type elevRunner struct{}

func newElevatedRunner() setup.Runner {
	return elevRunner{}
}

func (elevRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", name, err)
	}
	return exec.Command("pkexec", append([]string{bin}, args...)...).CombinedOutput()
}

func elevateAvailable() bool {
	_, err := exec.LookPath("pkexec")
	return err == nil
}
