//go:build linux

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
	return PlatformInfo{Name: "linux"}
}

const resolvConf = "/etc/resolv.conf"

func useResolved(r Runner) bool {
	_, err := r.CombinedOutput("resolvectl", "version")
	return err == nil
}

func ConfigureDNS(r Runner, bind string) (map[string][]string, error) {
	if useResolved(r) {
		ifaces := activeInterfaces(r)
		for _, iface := range ifaces {
			out, err := r.CombinedOutput("resolvectl", "dns", iface, bind)
			if err != nil {
				return nil, fmt.Errorf("resolvectl dns %s: %w\n%s", iface, err, out)
			}
			out, err = r.CombinedOutput("resolvectl", "domain", iface, "~.")
			if err != nil {
				return nil, fmt.Errorf("resolvectl domain %s: %w\n%s", iface, err, out)
			}
		}
		return map[string][]string{"__resolved__": activeInterfaces(r)}, nil
	}
	data, err := os.ReadFile(resolvConf)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resolvConf, err)
	}
	var nameservers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			nameservers = append(nameservers, strings.TrimPrefix(line, "nameserver "))
		}
	}
	backup := string(data)
	lines := []string{"# managed by DNSer (dnser setup)"}
	for _, ns := range nameservers {
		lines = append(lines, "nameserver "+ns)
	}
	lines[0] = "# managed by DNSer"
	content := "nameserver " + bind + "\n# previous entries follow\n" + stripNameservers(backup)
	if err := os.WriteFile(resolvConf, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", resolvConf, err)
	}
	return map[string][]string{"__resolv__": {backup}}, nil
}

func RestoreDNS(r Runner, saved map[string][]string) error {
	if backupList, ok := saved["__resolv__"]; ok && len(backupList) > 0 {
		if err := os.WriteFile(resolvConf, []byte(backupList[0]), 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", resolvConf, err)
		}
		return nil
	}
	if ifaces, ok := saved["__resolved__"]; ok && len(ifaces) > 0 {
		for _, iface := range ifaces {
			_, _ = r.CombinedOutput("resolvectl", "revert", iface)
		}
		return nil
	}
	return nil
}

func stripNameservers(content string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "nameserver ") && trimmed != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func activeInterfaces(r Runner) []string {
	out, err := r.CombinedOutput("ip", "-o", "-4", "route", "show", "default")
	if err != nil || len(out) == 0 {
		return []string{}
	}
	var ifaces []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				ifaces = append(ifaces, fields[i+1])
			}
		}
	}
	return ifaces
}

func TrustCA(r Runner, caPEM []byte, dir string) (string, error) {
	targetPath := "/usr/local/share/ca-certificates/dnser-ca.crt"
	tmpPath, err := WriteTempFile(dir, "dnser-ca-*.crt", caPEM)
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(
		"mkdir -p %q && cp %q %q", filepath.Dir(targetPath), tmpPath, targetPath,
	)
	if out, err := r.CombinedOutput("pkexec", "/bin/sh", "-c", script); err != nil {
		return "", fmt.Errorf("install CA cert: %w\n%s", err, out)
	}
	updateOut, updateErr := r.CombinedOutput("update-ca-certificates")
	if updateErr != nil {
		if out2, err2 := r.CombinedOutput("update-ca-trust", "extract"); err2 != nil {
			return "", fmt.Errorf("refresh CA store: %w\n%s / %v\n%s", updateErr, updateOut, err2, out2)
		}
	}
	_ = os.Remove(tmpPath)
	return targetPath, nil
}

func UntrustCA(r Runner, installPath string) error {
	script := fmt.Sprintf("rm -f %q", installPath)
	if out, err := r.CombinedOutput("pkexec", "/bin/sh", "-c", script); err != nil {
		return fmt.Errorf("remove CA cert: %w\n%s", err, out)
	}
	_, _ = r.CombinedOutput("update-ca-certificates", "--fresh")
	_, _ = r.CombinedOutput("update-ca-trust", "extract")
	return nil
}
