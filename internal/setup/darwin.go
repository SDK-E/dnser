//go:build darwin

package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PlatformInfo struct {
	Name string
}

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

func TrustCA(r Runner, caPEM []byte, dir string) (string, error) {
	tmpPath, err := WriteTempFile(dir, "dnser-ca-*.pem", caPEM)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join("/Library/Application Support/DNSer", "dnser-ca.pem")
	script := fmt.Sprintf(
		"mkdir -p %q && cp %q %q && security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %q",
		filepath.Dir(targetPath), tmpPath, targetPath, targetPath,
	)
	out, err := r.CombinedOutput("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", script))
	if err != nil {
		return "", fmt.Errorf("trust CA via admin prompt: %w\n%s", err, out)
	}
	_ = os.Remove(tmpPath)
	return targetPath, nil
}

func UntrustCA(r Runner, installPath string) error {
	script := fmt.Sprintf(
		"security remove-trusted-cert -d %q 2>/dev/null; rm -f %q",
		installPath, installPath,
	)
	out, err := r.CombinedOutput("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", script))
	if err != nil {
		return fmt.Errorf("untrust CA: %w\n%s", err, out)
	}
	return nil
}
