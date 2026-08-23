package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

func runCLI(t *testing.T, home string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), "DNSER_HOME="+home)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("cli %v: %v\n%s", args, err, out)
	}
	return string(out), code
}

func TestE2E_CLIWorkflow(t *testing.T) {
	home := t.TempDir()

	out, code := runCLI(t, home, "version")
	if code != 0 || !strings.HasPrefix(out, "dnser ") {
		t.Fatalf("version: %d %q", code, out)
	}

	dir := filepath.Join(home, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	viteCfg := "export default defineConfig({ server: { port: 5199 } })"
	if err := os.WriteFile(filepath.Join(dir, "vite.config.ts"), []byte(viteCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	out, code = runCLI(t, home, "link", "--domain=demo", "--wildcard")
	_ = os.Chdir(cwd)
	if code != 0 || !strings.Contains(out, "demo.test") || !strings.Contains(out, "5199") {
		t.Fatalf("link autodetect: %d %q", code, out)
	}

	out, _ = runCLI(t, home, "status")
	if !strings.Contains(out, "demo.test") || !strings.Contains(out, "5199") {
		t.Errorf("status output missing link:\n%s", out)
	}

	if _, code = runCLI(t, home, "add-record", "--domain=demo.test", "--type=TXT", "--name=_x", "--value=v1"); code != 0 {
		t.Fatalf("add-record failed")
	}
	out, _ = runCLI(t, home, "list-records", "demo.test")
	if !strings.Contains(out, "_x") || !strings.Contains(out, "v1") {
		t.Errorf("list-records:\n%s", out)
	}

	zoneFile := filepath.Join(home, "export.zone")
	if _, code = runCLI(t, home, "export", "-o", zoneFile); code != 0 {
		t.Fatal("export failed")
	}
	zoneData, err := os.ReadFile(zoneFile)
	if err != nil || !strings.Contains(string(zoneData), "$ORIGIN demo.test.") || !strings.Contains(string(zoneData), "; dnser: backend=localhost:5199") {
		t.Fatalf("zone export wrong: %v\n%s", err, zoneData)
	}

	if _, code = runCLI(t, home, "unlink", "--domain=demo.test"); code != 0 {
		t.Fatal("unlink failed")
	}
	if out, _ = runCLI(t, home, "status"); strings.Contains(out, "demo.test") {
		t.Fatalf("unlink did not remove project:\n%s", out)
	}

	if _, code = runCLI(t, home, "import", zoneFile); code != 0 {
		t.Fatal("import failed")
	}
	cfgPath := filepath.Join(home, "dnser.json")
	raw, _ := os.ReadFile(cfgPath)
	var cfg config.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	p := cfg.Projects[0]
	hasTXT, hasWildA := false, false
	for _, r := range p.Records {
		switch {
		case r.Type == "TXT" && r.Name == "_x":
			hasTXT = true
		case r.Type == "A" && r.Name == "*":
			hasWildA = true
		}
	}
	wildRoute := false
	for _, route := range p.Routes {
		if route.Host == "*" && len(route.Backends) > 0 && route.Backends[0] == "localhost:5199" {
			wildRoute = true
		}
	}
	if len(cfg.Projects) != 1 || p.Domain != "demo.test" ||
		len(p.Routes) == 0 || !wildRoute || !hasTXT || !hasWildA {
		t.Fatalf("round-trip state: %+v records=%+v", p, p.Records)
	}

	d := startDaemonExistingHome(t, home)
	client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion("anything.demo.test.", dns.TypeA)
	resp, _, err := client.Exchange(req, fmt.Sprintf("127.0.0.1:%d", d.ports.DNS))
	if err != nil || len(resp.Answer) != 1 {
		t.Fatalf("imported zone not resolving: err=%v answers=%v", err, resp.Answer)
	}

	logsOut, _ := runCLI(t, home, "logs", "--last", "5")
	if !strings.Contains(logsOut, "anything.demo.test") {
		t.Errorf("logs --last missing query:\n%s", logsOut)
	}
}
