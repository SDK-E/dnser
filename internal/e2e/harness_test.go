package e2e

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/SDK-E/dnser/internal/config"
)

var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "dnser-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e temp:", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	binPath = filepath.Join(tmp, binaryName())
	root := repoRoot()
	build := exec.Command("go", "build", "-o", binPath, "./cmd/dnser")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.Exit(code)
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "dnser.exe"
	}
	return "dnser"
}

func repoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

type freePorts struct{ DNS, HTTP, HTTPS, UI, Upstream int }

func reservePorts(t *testing.T) freePorts {
	t.Helper()
	var got []int
	for i := 0; i < 5; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ln.Addr().(*net.TCPAddr).Port)
		_ = ln.Close()
	}
	return freePorts{DNS: got[0], HTTP: got[1], HTTPS: got[2], UI: got[3], Upstream: got[4]}
}

type daemon struct {
	cmd     *exec.Cmd
	home    string
	bin     string
	ports   freePorts
	cacert  string
	baseURL string
	logFile string
}

func writeConfig(t *testing.T, home string, ports freePorts, upstreamPort int, projects []config.Project) {
	t.Helper()
	cfg := config.Default()
	cfg.Settings.Ports = config.Ports{DNS: ports.DNS, HTTP: ports.HTTP, HTTPS: ports.HTTPS, UI: ports.UI}
	cfg.Settings.Upstreams = []string{fmt.Sprintf("127.0.0.1:%d", upstreamPort)}
	cfg.Projects = projects
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "dnser.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startDaemon(t *testing.T, projects ...config.Project) *daemon {
	t.Helper()
	home := t.TempDir()
	ports := reservePorts(t)

	upstreamAddr := startFakeUpstream(t, ports.Upstream)
	_ = upstreamAddr

	writeConfig(t, home, ports, ports.Upstream, projects)

	d := &daemon{
		home:    home,
		bin:     binPath,
		ports:   ports,
		cacert:  filepath.Join(home, "certs", "dnser-ca.pem"),
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", ports.UI),
		logFile: filepath.Join(home, "daemon.log"),
	}
	logF, err := os.Create(d.logFile)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath, "start", "--foreground")
	cmd.Env = append(os.Environ(), "DNSER_HOME="+home)
	cmd.Stdout = logF
	cmd.Stderr = logF
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	d.cmd = cmd

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		_ = logF.Close()
	})

	waitHealthy(t, d.baseURL+"/api/v1/status", 20*time.Second)
	return d
}

func waitHealthy(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("daemon never became healthy at %s", url)
}

func queryDNS(t *testing.T, port int, name string, qtype uint16) *dns.Msg {
	t.Helper()
	client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	resp, _, err := client.Exchange(req, fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("query %s %s: %v", name, dns.TypeToString[qtype], err)
	}
	return resp
}

func startFakeUpstream(t *testing.T, port int) string {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		q := req.Question[0]
		m := new(dns.Msg)
		m.SetReply(req)
		name := strings.ToLower(strings.TrimSuffix(q.Name, "."))
		switch {
		case !strings.HasSuffix(name, ".forward.test"), strings.HasPrefix(name, "missing."):
			soa, _ := dns.NewRR(fmt.Sprintf("%s 30 IN SOA ns.%s host.%s 1 2 3 25 5", q.Name, q.Name, q.Name))
			m.Ns = append(m.Ns, soa)
			m.SetRcode(req, dns.RcodeNameError)
		case q.Qtype == dns.TypeA:
			rr, _ := dns.NewRR(fmt.Sprintf("%s 60 IN A 203.0.113.77", q.Name))
			m.Answer = append(m.Answer, rr)
		default:
			rr, _ := dns.NewRR(fmt.Sprintf("%s 30 IN A 203.0.113.78", q.Name))
			m.Answer = append(m.Answer, rr)
		}
		_ = w.WriteMsg(m)
	})
	srv := &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	deadline := time.Now().Add(5 * time.Second)
	client := &dns.Client{Net: "udp", Timeout: 500 * time.Millisecond}
	probe := new(dns.Msg)
	probe.SetQuestion("ready.forward.test.", dns.TypeA)
	for time.Now().Before(deadline) {
		if _, _, err := client.Exchange(probe, addr); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Cleanup(func() { _ = srv.Shutdown() })
	return addr
}

func tlsClient(t *testing.T, d *daemon) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(d.cacert)
	if err != nil {
		t.Fatalf("read CA (daemon may not have created it): %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("CA pem did not parse")
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   5 * time.Second,
	}
}

func startDaemonExistingHome(t *testing.T, home string) *daemon {
	t.Helper()
	ports := reservePorts(t)
	startFakeUpstream(t, ports.Upstream)
	raw, err := os.ReadFile(filepath.Join(home, "dnser.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Settings.Ports = config.Ports{DNS: ports.DNS, HTTP: ports.HTTP, HTTPS: ports.HTTPS, UI: ports.UI}
	out, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(filepath.Join(home, "dnser.json"), out, 0o600); err != nil {
		t.Fatal(err)
	}

	logF, err := os.Create(filepath.Join(home, "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binPath, "start", "--foreground")
	cmd.Env = append(os.Environ(), "DNSER_HOME="+home)
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
			logPath := filepath.Join(home, "daemon.log")
			if data, rerr := os.ReadFile(logPath); rerr == nil {
				t.Logf("daemon.log (%s):\n%s", runtime.GOOS, string(data))
			}
		}
	})
	d := &daemon{
		home: home, bin: binPath, ports: ports,
		cacert:  filepath.Join(home, "certs", "dnser-ca.pem"),
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", ports.UI),
		logFile: filepath.Join(home, "daemon.log"),
		cmd:     cmd,
	}
	waitHealthy(t, d.baseURL+"/api/v1/status", 20*time.Second)
	return d
}
