//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	label       = "enterprises.sdk.dnser"
	plistName   = label + ".plist"
	logFileName = "dnser.log"
)

type launchd struct{}

func NewManager() Manager { return launchd{} }

func (launchd) Name() string { return "launchd" }

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistName)
}

func logPath() string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".dnser", logFileName)
}

func renderPlist(binaryPath string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>start</string>
    <string>--foreground</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
  <key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, label, binaryPath, logPath(), logPath()))
}

func (l launchd) Install(binaryPath string) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary %s: %w", binaryPath, err)
	}
	path := plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := l.Stop(); err != nil {
		return err
	}
	if err := os.WriteFile(path, renderPlist(binaryPath), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	out, err := exec.Command("launchctl", "load", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %w\n%s", err, out)
	}
	return nil
}

func (l launchd) Uninstall() error {
	path := plistPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := l.Stop(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove plist: %w", err)
	}
	return nil
}

func (launchd) Stop() error {
	path := plistPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	out, err := exec.Command("launchctl", "unload", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl unload: %w\n%s", err, out)
	}
	return nil
}

func (l launchd) Start() error {
	return l.Install(executablePath())
}

func (launchd) IsRunning() (bool, error) {
	out, err := exec.Command("launchctl", "list").CombinedOutput()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, label) {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "-" {
				continue
			}
			return true, nil
		}
	}
	return false, nil
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "dnser"
	}
	return exe
}
