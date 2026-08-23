//go:build darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PlatformInfo struct {
	Name string
}

const caCommonName = "DNSer Local Root CA"

func Platform() PlatformInfo {
	return PlatformInfo{Name: "darwin"}
}

func ListDNSServices(r Runner) ([]string, error) {
	out, err := r.CombinedOutput("networksetup", "-listallnetworkservices")
	if err != nil {
		return nil, fmt.Errorf("list network services: %w\n%s", err, out)
	}
	var services []string
	for i, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, "denotes") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func GetDNSServers(r Runner, service string) ([]string, error) {
	out, err := r.CombinedOutput("networksetup", "-getdnsservers", service)
	if err != nil {
		return nil, nil
	}
	text := strings.TrimSpace(string(out))
	if strings.Contains(text, "There aren't any DNS Servers") {
		return nil, nil
	}
	return strings.Fields(text), nil
}

func ConfigureDNS(r Runner, bind string) (map[string][]string, error) {
	services, err := ListDNSServices(r)
	if err != nil {
		return nil, err
	}
	saved := make(map[string][]string)
	for _, svc := range services {
		prev, _ := GetDNSServers(r, svc)
		saved[svc] = prev
		out, err := r.CombinedOutput("networksetup", "-setdnsservers", svc, bind)
		if err != nil {
			return saved, fmt.Errorf("set dns for %s: %w\n%s", svc, err, out)
		}
	}
	return saved, nil
}

func RestoreDNS(r Runner, saved map[string][]string) error {
	for svc, servers := range saved {
		var out []byte
		var err error
		if len(servers) == 0 {
			out, err = r.CombinedOutput("networksetup", "-setdnsservers", svc, "Empty")
		} else {
			args := append([]string{"-setdnsservers", svc}, servers...)
			out, err = r.CombinedOutput("networksetup", args...)
		}
		if err != nil {
			return fmt.Errorf("restore dns for %s: %w\n%s", svc, err, out)
		}
	}
	return nil
}

func TrustCA(r Runner, caPEM []byte, dir string) (string, string, error) {
	tmpPath, err := WriteTempFile(dir, "dnser-ca-*.pem", caPEM)
	if err != nil {
		return "", "", err
	}
	targetPath := filepath.Join("/Library/Application Support/DNSer", "dnser-ca.pem")

	userScript := fmt.Sprintf(
		"security add-trusted-cert -r trustRoot -p ssl -p basic -k %q %q",
		loginKeychain(), caPEMPath(dir),
	)
	if out, uerr := runWithTimeout(r, "/bin/zsh", "-c", userScript); uerr == nil {
		_ = os.Remove(tmpPath)
		return caPEMPath(dir), TrustModeUser, nil
	} else if !errors.Is(uerr, context.DeadlineExceeded) && strings.Contains(uerr.Error(), "timed out") {
		_ = out
	}

	adminScript := fmt.Sprintf(
		"mkdir -p %q && cp %q %q && security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %q",
		filepath.Dir(targetPath), tmpPath, targetPath, targetPath,
	)
	if _, sudoErr := r.CombinedOutput("sudo", "-n", "true"); sudoErr == nil {
		if _, serr := runWithTimeout(r, "sudo", "-n", "/bin/sh", "-c", adminScript); serr == nil {
			_ = os.Remove(tmpPath)
			return targetPath, TrustModeAdmin, nil
		}
	}

	out, aerr := r.CombinedOutput("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", adminScript))
	if aerr != nil {
		_ = os.Remove(tmpPath)
		return "", "", fmt.Errorf("trust CA failed (user, sudo and admin): %w\n%s", aerr, out)
	}
	return targetPath, TrustModeAdmin, nil
}

func caPEMPath(dir string) string {
	return filepath.Join(dir, "certs", "dnser-ca.pem")
}

func loginKeychain() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "login.keychain-db"
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db")
}

func UntrustCA(r Runner, installPath, mode string) error {
	if mode == TrustModeUser {
		script := fmt.Sprintf(
			"security remove-trusted-cert %q 2>/dev/null; security delete-certificate -c %q 2>/dev/null; rm -f %q; true",
			installPath, caCommonName, installPath,
		)
		if _, err := r.CombinedOutput("/bin/zsh", "-c", script); err != nil {
			return fmt.Errorf("untrust CA (user): %w", err)
		}
		return nil
	}
	script := fmt.Sprintf(
		"security remove-trusted-cert -d %q 2>/dev/null; security delete-certificate -c %q /Library/Keychains/System.keychain 2>/dev/null; rm -f %q; true",
		installPath, caCommonName, installPath,
	)
	if _, sudoErr := r.CombinedOutput("sudo", "-n", "true"); sudoErr == nil {
		if _, serr := runWithTimeout(r, "sudo", "-n", "/bin/sh", "-c", script); serr == nil {
			return nil
		}
	}
	out, err := r.CombinedOutput("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", script))
	if err != nil {
		return fmt.Errorf("untrust CA: %w\n%s", err, out)
	}
	return nil
}
