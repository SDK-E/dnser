//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const taskName = "DNSer"

type schtasks struct{}

func NewManager() Manager { return schtasks{} }

func (schtasks) Name() string { return "scheduled-task" }

func (s schtasks) Install(binaryPath string) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary %s: %w", binaryPath, err)
	}
	args := fmt.Sprintf(`"%s" start --foreground`, binaryPath)
	out, err := exec.Command("schtasks", "/Create", "/F", "/TN", taskName,
		"/TR", args, "/SC", "ONLOGON").CombinedOutput()
	if err != nil {
		return fmt.Errorf("create scheduled task: %w\n%s", err, out)
	}
	return nil
}

func (schtasks) Uninstall() error {
	out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "does not exist") {
		return fmt.Errorf("delete scheduled task: %w\n%s", err, out)
	}
	return nil
}

func (schtasks) Stop() error {
	out, err := exec.Command("taskkill", "/IM", "dnser.exe", "/F").CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(out)), "not found") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stop process: %w\n%s", err, out)
	}
	return nil
}

func (s schtasks) Start() error {
	exe, err := os.Executable()
	if err != nil {
		exe = "dnser.exe"
	}
	return s.Install(exe)
}

func (schtasks) IsRunning() (bool, error) {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", "dnser.exe")).CombinedOutput()
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(string(out)), "dnser.exe"), nil
}
