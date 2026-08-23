package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

func TestE2E_HomeFlagIsolation(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "state", ".dnser")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	ports := reservePorts(t)
	upstreamPort := ports.Upstream - 1000
	startFakeUpstream(t, upstreamPort)
	writeConfig(t, home, freePorts{DNS: ports.DNS, HTTP: ports.HTTP, HTTPS: ports.HTTPS, UI: ports.UI}, upstreamPort, []config.Project{{
		Domain:   "flagged.test",
		Port:     ports.DNS,
		Wildcard: true,
	}})

	logF, err := os.Create(filepath.Join(home, "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binPath, "start", "--foreground",
		"--home", home,
		"--bind-port", fmt.Sprint(ports.DNS))
	baseEnv := environWithout(t, "DNSER_HOME")
	cmd.Env = append(baseEnv, "DNSER_HOME=")
	cmd.Stdout = logF
	cmd.Stderr = logF
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		_ = logF.Close()
		if t.Failed() {
			if data, rerr := os.ReadFile(filepath.Join(home, "daemon.log")); rerr == nil {
				t.Logf("daemon.log:\n%s", data)
			}
		}
	})

	waitHealthy(t, fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", ports.UI), 20*time.Second)

	client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion("x.flagged.test.", dns.TypeA)
	resp, _, err := client.Exchange(req, fmt.Sprintf("127.0.0.1:%d", ports.DNS))
	if err != nil || len(resp.Answer) != 1 {
		t.Fatalf("--home config not used: err=%v answers=%v", err, resp.Answer)
	}

	if _, err := os.Stat(filepath.Join(parent, "dnser.json")); !os.IsNotExist(err) {
		t.Fatal("daemon leaked dnser.json outside --home (off-by-one-directory regression)")
	}
	if _, err := os.Stat(filepath.Join(parent, "certs")); !os.IsNotExist(err) {
		t.Fatal("daemon leaked certs directory outside --home")
	}
}

func environWithout(t *testing.T, key string) []string {
	t.Helper()
	var out []string
	for _, kv := range os.Environ() {
		if len(kv) > len(key) && kv[:len(key)] == key && kv[len(key)] == '=' {
			continue
		}
		out = append(out, kv)
	}
	return out
}
