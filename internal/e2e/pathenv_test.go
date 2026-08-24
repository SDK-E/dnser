package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func minimalLaunchdPATH() string {
	if runtime.GOOS == "windows" {
		sysRoot := os.Getenv("SystemRoot")
		return sysRoot + "\\System32;" + sysRoot
	}
	return "/usr/bin:/bin:/usr/sbin:/sbin"
}

func installPathedHelper(t *testing.T, home string, name string) string {
	t.Helper()
	binDir := filepath.Join(home, "tools-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	helperName := name
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	dst := filepath.Join(binDir, helperName)
	src := buildRunnerHelper(t)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir
}

func seedLoginShellPathCache(t *testing.T, home string, dirs ...string) {
	t.Helper()
	cache := map[string]any{
		"version":     1,
		"shell":       "/bin/sh",
		"path":        strings.Join(dirs, string(os.PathListSeparator)),
		"captured_at": time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "path-cache.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func stripPathEnv(base []string) []string {
	out := base[:0]
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "DNSER_EXTRA_PATH=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PATH="+minimalLaunchdPATH())
}

type runnerAppView struct {
	Domain    string `json:"domain"`
	State     string `json:"state"`
	Port      int    `json:"port"`
	LastError string `json:"last_error"`
}

func waitAppState(t *testing.T, d *daemon, domain string, want string) runnerAppView {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	var found runnerAppView
	for time.Now().Before(deadline) {
		resp, err := http.Get(d.baseURL + "/api/v1/runner")
		if err == nil {
			var payload struct {
				Apps []runnerAppView `json:"apps"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			_ = resp.Body.Close()
			if err == nil {
				for _, app := range payload.Apps {
					if app.Domain == domain {
						found = app
						if app.State == want && app.Port > 0 {
							return app
						}
					}
				}
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	logs, _ := os.ReadFile(d.logFile)
	t.Fatalf("app %q never reached state %q; last=%+v\nlog:\n%s", domain, want, found, logs)
	return found
}

func doctorCheckByName(t *testing.T, baseURL string, name string) (status, detail string) {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/v1/doctor")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, c := range payload.Checks {
		if c.Name == name {
			return c.Status, c.Detail
		}
	}
	t.Fatalf("doctor check %q missing", name)
	return "", ""
}

func TestE2E_ManagedCommandResolvesViaAugmentedPATH(t *testing.T) {
	home := t.TempDir()
	binDir := installPathedHelper(t, home, "e2e-pathed")
	seedLoginShellPathCache(t, home, binDir)
	projectDir := t.TempDir()
	writeFile(t, filepath.Join(projectDir, ".dnser.yaml"), "command: e2e-pathed\n")

	d := startDaemonWithEnv(t, stripPathEnv, home)

	if out, code := runCLI(t, d.home, "link", "--domain=pathdemo", projectDir); code != 0 {
		t.Fatalf("link: %d %s", code, out)
	}
	stopAppsBeforeExit(t, d, "pathdemo.test")

	waitAppState(t, d, "pathdemo.test", "up")

	status, detail := doctorCheckByName(t, d.baseURL, "commands")
	if status == "fail" || strings.Contains(detail, "e2e-pathed") {
		t.Fatalf("doctor commands check should resolve the bare command, got %s: %s", status, detail)
	}
}

func TestE2E_MissingCommandSurfacesHintInPS(t *testing.T) {
	home := t.TempDir()
	binDir := installPathedHelper(t, home, "e2e-unrelated")
	seedLoginShellPathCache(t, home, binDir)
	projectDir := t.TempDir()

	d := startDaemonWithEnv(t, stripPathEnv, home)

	if out, code := runCLI(t, d.home, "link", "--domain=ghost", "--command=definitely-not-a-real-binary", projectDir); code != 0 {
		t.Fatalf("link: %d %s", code, out)
	}

	deadline := time.Now().Add(40 * time.Second)
	var lastErr string
	doctorWarned := false
	for time.Now().Before(deadline) {
		resp, err := http.Get(d.baseURL + "/api/v1/runner")
		if err == nil {
			var payload struct {
				Apps []runnerAppView `json:"apps"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			_ = resp.Body.Close()
			if err == nil {
				for _, app := range payload.Apps {
					if app.Domain == "ghost.test" {
						lastErr = app.LastError
					}
				}
			}
		}
		status, detail := doctorCheckByNameQuiet(d.baseURL, "commands")
		doctorWarned = status == "warn" && strings.Contains(detail, "definitely-not-a-real-binary")
		if doctorWarned {
			if runtime.GOOS == "windows" {
				return
			}
			if !strings.Contains(lastErr, "not found in daemon PATH") {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			psOut, _ := runCLI(t, d.home, "ps")
			if !strings.Contains(psOut, "not found in daemon PATH") {
				t.Fatalf("ps output missing actionable hint:\n%s", psOut)
			}
			return
		}
		time.Sleep(400 * time.Millisecond)
	}
	logs, _ := os.ReadFile(d.logFile)
	t.Fatalf("doctor never warned about missing command; lastErr=%q\nlog:\n%s", lastErr, logs)
}

func doctorCheckByNameQuiet(baseURL string, name string) (string, string) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/v1/doctor")
	if err != nil {
		return "", ""
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return "", ""
	}
	for _, c := range payload.Checks {
		if c.Name == name {
			return c.Status, c.Detail
		}
	}
	return "", ""
}

func TestE2E_ServicesMultiPortAndForwarders(t *testing.T) {
	home := t.TempDir()
	binDir := installPathedHelper(t, home, "e2e-svc-main")
	installPathedHelper(t, home, "e2e-svc-aux")
	seedLoginShellPathCache(t, home, binDir)
	projectDir := t.TempDir()

	writeFile(t, filepath.Join(projectDir, ".dnser.yaml"), fmt.Sprintf(`command: e2e-svc-main
services:
  aux:
    type: http
    command: e2e-svc-aux
routes:
  - host: api
    backends:
      - 127.0.0.1:{port}
    https: true
  - host: relay
    tcp: true
    listen: %[1]d
    backends:
      - 127.0.0.1:{port:aux}
`, freeTCPPort(t)))

	d := startDaemonWithEnv(t, stripPathEnv, home)

	if out, code := runCLI(t, d.home, "link", "--domain=svcproj", projectDir); code != 0 {
		t.Fatalf("link: %d %s", code, out)
	}
	stopAppsBeforeExit(t, d, "svcproj.test", "svcproj.test/aux")

	waitAppState(t, d, "svcproj.test", "up")
	waitAppState(t, d, "svcproj.test/aux", "up")

	waitForServiceRecord(t, d.ports.DNS, "aux.svcproj.test")

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", d.ports.HTTP), nil)
	req.Host = "api.svcproj.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy via path route: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "apphelper-up") {
		t.Fatalf("route proxy response status=%d body=%q", resp.StatusCode, body)
	}

	assertTCPReachable(t, d.home, "relay.svcproj.test")

	svcOut, _ := runCLI(t, d.home, "service", "list", "svcproj.test")
	for _, want := range []string{"aux", "managed"} {
		if !strings.Contains(svcOut, want) {
			t.Fatalf("service list output missing %q:\n%s", want, svcOut)
		}
	}
}

func assertTCPReachable(t *testing.T, home string, _ string) {
	t.Helper()
	cfgRaw, err := os.ReadFile(filepath.Join(home, "dnser.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Projects []struct {
			Domain string `json:"domain"`
			Routes []struct {
				TCP    bool `json:"tcp"`
				Listen int  `json:"listen"`
			} `json:"routes"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatal(err)
	}
	listen := 0
	for _, p := range cfg.Projects {
		if p.Domain != "svcproj.test" {
			continue
		}
		for _, r := range p.Routes {
			if r.TCP && r.Listen > 0 {
				listen = r.Listen
			}
		}
	}
	if listen == 0 {
		t.Fatal("tcp route with listen not persisted")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(listen)), time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("tcp forwarder on port %d never became reachable", listen)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stopAppsBeforeExit(t *testing.T, d *daemon, keys ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, k := range keys {
			req, err := http.NewRequest(http.MethodPost, d.baseURL+"/api/v1/runner/action/stop/"+k, nil)
			if err != nil {
				continue
			}
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
			}
		}
		time.Sleep(600 * time.Millisecond)
	})
}

func waitForServiceRecord(t *testing.T, port int, name string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		answer := queryDNS(t, port, name, dns.TypeA)
		for _, rr := range answer.Answer {
			if a, ok := rr.(*dns.A); ok && a.A.String() == "127.0.0.1" {
				return
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	answer := queryDNS(t, port, name, dns.TypeA)
	t.Fatalf("service A record %s → 127.0.0.1 never appeared, got %+v", name, answer.Answer)
}
