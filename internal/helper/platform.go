package helper

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/SDK-E/dnser/internal/journal"
)

func RealRunner() journal.CommandRunner {
	return journal.ExecRunner{}
}

type securityTrust struct{}

func (securityTrust) IsTrusted(ctx context.Context, certPath string) (bool, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false, fmt.Errorf("read cert %s: %w", certPath, err)
	}
	out, err := exec.CommandContext(ctx, "security", "verify-cert", "-c", certPath).Output()
	if err != nil && !bytes.Contains(out, []byte("certificate verification successful")) {
		return false, nil
	}
	_ = data
	return bytes.Contains(out, []byte("successful")), nil
}

func (securityTrust) Trust(ctx context.Context, certPath string) error {
	cmd := exec.CommandContext(ctx, "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trust %s: %w (%s)", certPath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (securityTrust) Untrust(ctx context.Context, certPath string) error {
	cmd := exec.CommandContext(ctx, "security", "remove-trusted-cert", "-d", certPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrText := stderr.String()
		if strings.Contains(stderrText, "not found") || strings.Contains(stderrText, "SecKeychainSearch") {
			return nil
		}
		return fmt.Errorf("untrust %s: %w (%s)", certPath, err, strings.TrimSpace(stderrText))
	}
	return nil
}

func CATrustOps() journal.TrustOperator {
	switch runtime.GOOS {
	case "darwin":
		return securityTrust{}
	case "linux":
		return cpTrust{}
	default:
		return nil
	}
}

type cpTrust struct{}

const linuxCALocalDir = "/usr/local/share/ca-certificates"

func (cpTrust) IsTrusted(ctx context.Context, certPath string) (bool, error) {
	name := filepathBase64Safe(certPath)
	_, err := os.Stat(installedCertPath(name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat installed cert: %w", err)
	}
	return true, nil
}

func (cpTrust) Trust(ctx context.Context, certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read cert %s: %w", certPath, err)
	}
	target := installedCertPath(filepathBase64Safe(certPath))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("stage cert at %s: %w", target, err)
	}
	r := journal.ExecRunner{}
	if _, err := r.Run(ctx, "update-ca-certificates"); err != nil {
		return err
	}
	return nil
}

func (cpTrust) Untrust(ctx context.Context, certPath string) error {
	target := installedCertPath(filepathBase64Safe(certPath))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove staged cert %s: %w", target, err)
	}
	r := journal.ExecRunner{}
	if _, err := r.Run(ctx, "update-ca-certificates"); err != nil {
		return err
	}
	return nil
}

func installedCertPath(name string) string {
	return linuxCALocalDir + "/" + name
}

func filepathBase64Safe(p string) string {
	base := p
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			base = p[i+1:]
			break
		}
	}
	return "dnser-" + strings.ReplaceAll(strings.TrimSuffix(base, ".pem"), ".", "_") + ".crt"
}

type launchdService struct{}

func (launchdService) labelFor(defPath string) string {
	return "enterprises.sdk.dnser"
}

func (launchdService) IsLoaded(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "launchctl", "print", "gui/"+uidStr()+"/"+name)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if strings.Contains(stderr.String(), "Could not find service") {
		return false, nil
	}
	return false, fmt.Errorf("launchctl print %s: %w", name, err)
}

func uidStr() string {
	return fmt.Sprint(os.Getuid())
}

func (launchdService) Load(ctx context.Context, defPath string) error {
	self := launchdService{}
	label := self.labelFor(defPath)
	cmd := exec.CommandContext(ctx, "launchctl", "bootstrap", "gui/"+uidStr(), defPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if loaded, lerr := self.IsLoaded(ctx, label); lerr == nil && loaded {
			return nil
		}
		return fmt.Errorf("launchctl bootstrap %s: %w (%s)", defPath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (launchdService) Unload(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "launchctl", "bootout", "gui/"+uidStr()+"/"+name)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "Could not find service") {
			return nil
		}
		return fmt.Errorf("launchctl bootout %s: %w (%s)", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type systemdUserService struct{}

func (systemdUserService) IsLoaded(ctx context.Context, name string) (bool, error) {
	r := journal.ExecRunner{}
	_, err := r.Run(ctx, "systemctl", "--user", "is-enabled", name)
	return err == nil, nil
}

func (systemdUserService) Load(ctx context.Context, defPath string) error {
	r := journal.ExecRunner{}
	if _, err := r.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if _, err := r.Run(ctx, "systemctl", "--user", "enable", "--now", baseName(defPath)); err != nil {
		return err
	}
	return nil
}

func (systemdUserService) Unload(ctx context.Context, name string) error {
	r := journal.ExecRunner{}
	_, _ = r.Run(ctx, "systemctl", "--user", "disable", "--now", name)
	return nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func ServiceOps() journal.ServiceOperator {
	switch runtime.GOOS {
	case "darwin":
		return launchdService{}
	case "linux":
		return systemdUserService{}
	default:
		return nil
	}
}

func PromptRefusalHint(w *bufio.Writer) {
	fmt.Fprintln(w, "elevation refused: dnser will run in fallback mode (high-port DNS, no resolver files)")
}
