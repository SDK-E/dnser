//go:build darwin

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
	script := setup.ShellJoin(append([]string{bin}, args...)...)
	return exec.Command("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", script)).CombinedOutput()
}

func elevateAvailable() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}
