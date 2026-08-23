//go:build windows

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
	return PlatformInfo{Name: "windows"}
}

func activeInterfaces(r Runner) ([]string, error) {
	out, err := r.CombinedOutput("netsh", "interface", "show", "interface")
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w\n%s", err, out)
	}
	var ifaces []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && (fields[0] == "Enabled" || fields[0] == "Connected") {
			name := strings.Join(fields[3:], " ")
			ifaces = append(ifaces, name)
		}
	}
	return ifaces, nil
}

func GetDNSServers(r Runner, iface string) ([]string, error) {
	out, err := r.CombinedOutput("netsh", "interface", "ip", "show", "dns", "name="+iface)
	if err != nil {
		return nil, nil
	}
	var servers []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Statically Configured DNS Servers") || strings.Contains(line, "DNS Servers:") {
			continue
		}
		f := strings.TrimSpace(strings.TrimPrefix(line, ""))
		if isIPLine(f) {
			servers = append(servers, f)
		}
	}
	return servers, nil
}

func isIPLine(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 && !strings.Contains(s, ":") {
		return false
	}
	return s != "" && (s[0] >= '0' && s[0] <= '9' || strings.Contains(s, ":"))
}

func ConfigureDNS(r Runner, bind string) (map[string][]string, error) {
	ifaces, err := activeInterfaces(r)
	if err != nil {
		return nil, err
	}
	saved := make(map[string][]string)
	for _, iface := range ifaces {
		prev, _ := GetDNSServers(r, iface)
		saved[iface] = prev
		out, err := r.CombinedOutput("netsh", "interface", "ip", "set",
			"dns", "name="+iface, "source=static", "addr="+bind, "register=primary")
		if err != nil {
			return saved, fmt.Errorf("set dns for %s: %w\n%s", iface, err, out)
		}
	}
	return saved, nil
}

func RestoreDNS(r Runner, saved map[string][]string) error {
	for iface := range saved {
		out, err := r.CombinedOutput("netsh", "interface", "ip", "set",
			"dns", "name="+iface, "source=dhcp")
		if err != nil {
			return fmt.Errorf("restore dns for %s: %w\n%s", iface, err, out)
		}
	}
	return nil
}

func TrustCA(r Runner, caPEM []byte, dir string) (string, error) {
	targetPath := filepath.Join(os.Getenv("ProgramData"), "DNSer", "dnser-ca.pem")
	tmpPath, err := WriteTempFile(dir, "dnser-ca-*.pem", caPEM)
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(
		"New-Item -Force -Type Directory '%s' | Out-Null; Copy-Item '%s' '%s'; Import-Certificate -FilePath '%s' -CertStoreLocation Cert:\\LocalMachine\\Root",
		filepath.Dir(targetPath), tmpPath, targetPath, targetPath,
	)
	out, err := r.CombinedOutput("powershell", "-Command",
		fmt.Sprintf("Start-Process powershell -Verb RunAs -Wait -ArgumentList '-Command','%s'", script))
	if err != nil {
		return "", fmt.Errorf("trust CA via elevation: %w\n%s", err, out)
	}
	_ = os.Remove(tmpPath)
	return targetPath, nil
}

func UntrustCA(r Runner, installPath string) error {
	script := fmt.Sprintf(
		"Remove-Item -Force '%s' -ErrorAction SilentlyContinue; Remove-ItemProperty -Path 'HKLM:\\SOFTWARE\\Microsoft\\SystemCertificates\\Root\\Certificates' -ErrorAction SilentlyContinue",
		installPath,
	)
	out, err := r.CombinedOutput("powershell", "-Command",
		fmt.Sprintf("Start-Process powershell -Verb RunAs -Wait -ArgumentList '-Command','%s'", script))
	if err != nil {
		return fmt.Errorf("untrust CA via elevation: %w\n%s", err, out)
	}
	return nil
}
