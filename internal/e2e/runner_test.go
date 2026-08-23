package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func buildRunnerHelper(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "e2e-apphelper")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/apphelper")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build apphelper: %v\n%s", err, out)
	}
	return bin
}

func TestE2E_RunnerLifecycle(t *testing.T) {
	helperBin := buildRunnerHelper(t)
	projectDir := t.TempDir()

	d := startDaemon(t)

	linkOut, code := runCLI(t, d.home, "link", "--domain=demo2", "--command="+helperBin, projectDir)
	if code != 0 {
		t.Fatalf("link: %d %s", code, linkOut)
	}

	var managedPort int
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(d.baseURL + "/api/v1/runner")
		if err == nil {
			var payload struct {
				Apps []struct {
					Domain string `json:"domain"`
					State  string `json:"state"`
					Port   int    `json:"port"`
				} `json:"apps"`
			}
			err = json.NewDecoder(resp.Body).Decode(&payload)
			_ = resp.Body.Close()
			if err == nil {
				for _, appInfo := range payload.Apps {
					if appInfo.Domain == "demo2.test" && appInfo.State == "up" && appInfo.Port > 0 {
						managedPort = appInfo.Port
					}
				}
			}
			if managedPort > 0 {
				break
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	if managedPort == 0 {
		out, _ := runCLI(t, d.home, "ps")
		logs, _ := os.ReadFile(d.logFile)
		t.Fatalf("managed app never came up\nps:\n%s\nlog:\n%s", out, logs)
	}

	answer := queryDNS(t, d.ports.DNS, "demo2.test", dns.TypeA)
	hasLoopbackA := false
	for _, rr := range answer.Answer {
		if a, ok := rr.(*dns.A); ok && a.A.String() == "127.0.0.1" {
			hasLoopbackA = true
		}
	}
	if !hasLoopbackA {
		t.Fatalf("expected A record for demo2.test, got %+v", answer.Answer)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", d.ports.HTTP), nil)
	req.Host = "demo2.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy to managed app: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "apphelper-up") {
		t.Fatalf("proxy response status=%d body=%q", resp.StatusCode, body)
	}

	apiResp, err := http.Get(d.baseURL + "/api/v1/runner")
	if err != nil {
		t.Fatal(err)
	}
	apiBody, _ := io.ReadAll(apiResp.Body)
	_ = apiResp.Body.Close()
	if !strings.Contains(string(apiBody), "demo2.test") || !strings.Contains(string(apiBody), `"state":"up"`) {
		t.Fatalf("runner API payload: %s", apiBody)
	}

	docResp, err := http.Get(d.baseURL + "/api/v1/doctor")
	if err != nil {
		t.Fatal(err)
	}
	docBody, _ := io.ReadAll(docResp.Body)
	_ = docResp.Body.Close()
	if !strings.Contains(string(docBody), `"dns-port"`) {
		t.Fatalf("doctor payload: %s", docBody)
	}

	stopReq, _ := http.NewRequest(http.MethodPost, d.baseURL+"/api/v1/runner/demo2.test/stop", nil)
	stopResp, err := http.DefaultClient.Do(stopReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = stopResp.Body.Close()

	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := runCLI(t, d.home, "ps")
		if strings.Contains(out, "stopped") {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	out, _ := runCLI(t, d.home, "ps")
	t.Fatalf("app never stopped:\n%s", out)
}
