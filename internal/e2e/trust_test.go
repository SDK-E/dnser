package e2e

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SDK-E/dnser/internal/certs"
	"github.com/SDK-E/dnser/internal/setup"
)

func TestE2E_TrustStoreValidatesLeafWithoutPinnedCA(t *testing.T) {
	if testing.Short() {
		t.Skip("needs system trust store integration")
	}
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skipf("no trust flow for %s", runtime.GOOS)
	}

	home := t.TempDir()
	ca, err := certs.NewCA(filepath.Join(home, "certs"))
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	manager := certs.NewManager(ca)
	leaf, err := manager.Leaf("e2e.trust.test")
	if err != nil {
		t.Fatalf("leaf issuance: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "trust-ok")
	})
	srv := &http.Server{
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{*leaf}, NextProtos: []string{"http/1.1"}},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:39443")
	if err != nil {
		t.Skipf("port 39443 busy: %v", err)
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })

	r := setup.SystemRunner()
	installPath, mode, err := setup.TrustCA(r, ca.CertificatePEM(), home)
	if err != nil {
		t.Skipf("system trust installation unavailable on this runner: %v", err)
	}
	t.Cleanup(func() { _ = setup.UntrustCA(r, installPath, mode) })

	curl := curlBin()
	args := func() []string {
		return []string{"-s", "-o", "/dev/null", "-w", "%{http_code}",
			"--resolve", "e2e.trust.test:39443:127.0.0.1",
			"https://e2e.trust.test:39443/hello"}
	}
	waitFor(t, func() bool {
		out, err := exec.Command(curl, args()...).CombinedOutput()
		return err == nil && strings.TrimSpace(string(out)) == "200"
	}, 20*time.Second, runtime.GOOS+": system curl never validated the installed CA chain")
}

func curlBin() string {
	if runtime.GOOS == "windows" {
		return "curl.exe"
	}
	return "curl"
}
