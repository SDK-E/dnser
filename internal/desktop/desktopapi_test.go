package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlistContent(t *testing.T) {
	data := plistContent("enterprises.sdk.dnser.desktop", "/Applications/DNSer.app/Contents/MacOS/dnser-desktop")
	s := string(data)
	for _, want := range []string{
		"<string>enterprises.sdk.dnser.desktop</string>",
		"<string>/Applications/DNSer.app/Contents/MacOS/dnser-desktop</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("plist missing %q:\n%s", want, s)
		}
	}
}

func TestDesktopEntryContent(t *testing.T) {
	s := string(desktopEntryContent("/opt/dnser/dnser-desktop"))
	for _, want := range []string{"[Desktop Entry]", "Exec=/opt/dnser/dnser-desktop", "X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(s, want) {
			t.Errorf("desktop entry missing %q:\n%s", want, s)
		}
	}
}

func TestAutostartRoundTripDarwinLinux(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "linux":
	default:
		t.Skipf("file-based autostart not applicable on %s", runtime.GOOS)
	}
	exe := "/tmp/opencode/fake-dnser-desktop"
	if err := setAutostart(exe, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	defer func() { _ = setAutostart(exe, false) }()
	if !autostartActive() {
		t.Fatal("expected autostart active after enable")
	}
	var path string
	if runtime.GOOS == "darwin" {
		path = filepath.Join(homeDirForTest(), "Library", "LaunchAgents", darwinAutostartLabel+".plist")
	} else {
		path = filepath.Join(homeDirForTest(), ".config", "autostart", "dnser-desktop.desktop")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if !strings.Contains(string(data), exe) {
		t.Errorf("artifact missing exe path:\n%s", data)
	}
	if err := setAutostart(exe, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if autostartActive() {
		t.Fatal("expected autostart inactive after disable")
	}
}

func homeDirForTest() string {
	home, _ := os.UserHomeDir()
	return home
}

func TestLoopbackOnlyRejectsRemote(t *testing.T) {
	svc := testService(t)
	handler := svc.APIRoutes()

	req := httptest.NewRequest("GET", "/api/v1/desktop/status", nil)
	req.RemoteAddr = "192.168.1.50:55555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote addr must be forbidden, got %d", rec.Code)
	}

	req2 := httptest.NewRequest("GET", "/api/v1/desktop/status", nil)
	req2.RemoteAddr = "127.0.0.1:40001"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("loopback status failed: %d %s", rec2.Code, rec2.Body.String())
	}
	var payload desktopStatusPayload
	if err := json.Unmarshal(rec2.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if payload.Status.Version == "" {
		t.Error("status payload missing version")
	}
}

func TestSetAutostartEndpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("registry-backed on windows; covered by command tests")
	}
	svc := testService(t)
	handler := svc.APIRoutes()
	body := strings.NewReader(`{"enabled":true}`)
	req := httptest.NewRequest("POST", "/api/v1/desktop/autostart", body)
	req.RemoteAddr = "127.0.0.1:40002"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("autostart enable failed: %d %s", rec.Code, rec.Body.String())
	}
	defer func() { _ = svc.SetAutostart(false) }()
	if !autostartActive() {
		t.Error("expected autostart enabled after endpoint call")
	}
}
