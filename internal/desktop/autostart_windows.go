package desktop

import (
	"fmt"
	"os/exec"
	"strings"
)

const windowsRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const windowsRunValue = "DNSer"

type cmdRunner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

func setAutostart(exe string, enabled bool) error {
	return setAutostartWindows(nil, exe, enabled)
}

func autostartActive() bool {
	return autostartActiveWindows(nil)
}

func setAutostartWindows(r cmdRunner, exe string, enabled bool) error {
	if r == nil {
		r = realRunner{}
	}
	if !enabled {
		out, err := r.CombinedOutput("reg", "delete", windowsRunKey, "/v", windowsRunValue, "/f")
		if err == nil {
			return nil
		}
		if strings.Contains(strings.ToLower(string(out)), "unable to find") {
			return nil
		}
		return fmt.Errorf("remove autostart entry: %w", err)
	}
	quoted := `"` + exe + `"`
	_, err := r.CombinedOutput("reg", "add", windowsRunKey, "/v", windowsRunValue, "/t", "REG_SZ", "/d", quoted, "/f")
	if err != nil {
		return fmt.Errorf("add autostart entry: %w", err)
	}
	return nil
}

func autostartActiveWindows(r cmdRunner) bool {
	if r == nil {
		r = realRunner{}
	}
	out, err := r.CombinedOutput("reg", "query", windowsRunKey, "/v", windowsRunValue)
	return err == nil && strings.Contains(strings.ToLower(string(out)), strings.ToLower(windowsRunValue))
}

type realRunner struct{}

func (realRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}
