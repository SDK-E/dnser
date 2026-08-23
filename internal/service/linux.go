//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitName = "dnser.service"

type systemd struct{}

func NewManager() Manager { return systemd{} }

func (systemd) Name() string { return "systemd-user" }

func unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", unitName)
}

func renderUnit(binaryPath string) []byte {
	return []byte(fmt.Sprintf(`[Unit]
Description=DNSer local DNS + proxy
After=network-online.target

[Service]
ExecStart=%s start --foreground
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, binaryPath))
}

func (s systemd) Install(binaryPath string) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary %s: %w", binaryPath, err)
	}
	path := unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	if err := os.WriteFile(path, renderUnit(binaryPath), 0o644); err != nil {
		return fmt.Errorf("write unit: %w", err)
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "--now", unitName},
	} {
		out, err := exec.Command("systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %v: %w\n%s", args, err, out)
		}
	}
	return nil
}

func (s systemd) Uninstall() error {
	if _, err := os.Stat(unitPath()); os.IsNotExist(err) {
		return nil
	}
	out, err := exec.Command("systemctl", "--user", "disable", "--now", unitName).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "does not exist") {
		return fmt.Errorf("disable service: %w\n%s", err, out)
	}
	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit: %w", err)
	}
	_, _ = exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput()
	return nil
}

func (systemd) Stop() error {
	out, err := exec.Command("systemctl", "--user", "stop", unitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop service: %w\n%s", err, out)
	}
	return nil
}

func (s systemd) Start() error {
	return s.Install(executablePath())
}

func (systemd) IsRunning() (bool, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", unitName).CombinedOutput()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "active", nil
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "dnser"
	}
	return exe
}
