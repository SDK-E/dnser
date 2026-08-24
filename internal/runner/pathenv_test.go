package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildPathListOrderAndDedupe(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		switch key {
		case "PNPM_HOME":
			return ""
		case "CARGO_HOME":
			return ""
		case "GOPATH":
			return filepath.Join(home, "go")
		default:
			return ""
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "go", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	current := "/usr/bin:/bin:" + local
	login := "/opt/homebrew/bin:/usr/local/bin:" + local
	extra := "/custom/first:/opt/homebrew/bin"

	got := BuildPathList(current, login, extra, home, runtime.GOOS, getenv)

	wantPrefix := []string{"/custom/first", "/opt/homebrew/bin"}
	for i, w := range wantPrefix {
		if got[i] != w {
			t.Fatalf("got %v, want %v first", got, wantPrefix)
		}
	}
	if !containsDir(got, "/bin") || !containsDir(got, "/usr/bin") {
		t.Errorf("current PATH entries must survive: %v", got)
	}
	if !containsDir(got, local) {
		t.Errorf("existing entry should be kept once: %v", got)
	}
	if countOf(got, local) != 1 {
		t.Errorf("expected dedupe of %s: %v", local, got)
	}
	if !containsDir(got, filepath.Join(home, "go", "bin")) {
		t.Errorf("GOPATH/bin should be discovered when it exists: %v", got)
	}
	idxLogin := indexOf(got, "/opt/homebrew/bin")
	idxLocal := indexOf(got, local)
	if idxLogin == -1 || idxLocal == -1 || idxLogin > idxLocal {
		t.Errorf("discovered dirs must outrank current PATH duplicates: %v", got)
	}
}

func containsDir(dirs []string, dir string) bool { return indexOf(dirs, dir) >= 0 }

func indexOf(dirs []string, dir string) int {
	for i, d := range dirs {
		if d == dir {
			return i
		}
	}
	return -1
}

func countOf(dirs []string, dir string) int {
	n := 0
	for _, d := range dirs {
		if d == dir {
			n++
		}
	}
	return n
}

func TestPathResolverCacheTTLAndRefresh(t *testing.T) {
	home := t.TempDir()
	dnserHome := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	calls := 0
	resolver := NewPathResolver(PathOptions{
		UserHome:    home,
		DnserHome:   dnserHome,
		CurrentPATH: "/usr/bin",
		Shell:       "/bin/zsh",
		TTL:         time.Hour,
		Clock:       func() time.Time { return now },
		Capture: func(shell string) (string, error) {
			calls++
			return "/captured/" + shell + "/bin", nil
		},
	})

	first := resolver.Dirs()
	if !containsDir(first, "/captured//bin/zsh/bin") {
		t.Fatalf("captured login PATH should feed the resolver, got %v", first)
	}
	if calls != 1 {
		t.Fatalf("capture called %d times, want 1", calls)
	}

	now = now.Add(30 * time.Minute)
	_ = resolver.Dirs()
	if calls != 1 {
		t.Fatalf("cache should suppress recapture within TTL, calls=%d", calls)
	}

	now = now.Add(2 * time.Hour)
	_ = resolver.Dirs()
	if calls != 2 {
		t.Fatalf("expired cache should recapture, calls=%d", calls)
	}

	data, err := os.ReadFile(filepath.Join(dnserHome, "path-cache.json"))
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	if !strings.Contains(string(data), `"path": "/captured/`) {
		t.Errorf("cache content unexpected: %s", data)
	}
}

func TestPathResolverFallsBackToStaleOnCaptureFailure(t *testing.T) {
	dnserHome := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	cached := map[string]any{"version": 1, "shell": "/bin/zsh", "path": "/old/dir", "captured_at": now.Add(-2 * time.Hour).UTC().Format(time.RFC3339)}
	writeJSONFile(t, filepath.Join(dnserHome, "path-cache.json"), cached)

	resolver := NewPathResolver(PathOptions{
		DnserHome:   dnserHome,
		CurrentPATH: "/usr/bin",
		Shell:       "/bin/zsh",
		TTL:         time.Hour,
		Clock:       func() time.Time { return now },
		Capture: func(shell string) (string, error) {
			return "", errors.New("capture failed")
		},
	})
	got := resolver.String()
	if !strings.Contains(got, "/old/dir") {
		t.Fatalf("stale cache should be kept on capture failure, PATH=%s", got)
	}
}

func TestSubstitutePortMap(t *testing.T) {
	argv := []string{"sh", "-c", "serve --port {port} --peer {port:api} --keep {port:missing}"}
	got := SubstitutePortMap(argv, 3000, map[string]int{"api": 4001})
	want := []string{"sh", "-c", "serve --port 3000 --peer 4001 --keep {port:missing}"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestCommandBinaryExtraction(t *testing.T) {
	cases := map[string]string{
		"pnpm dev --port {port}":                       "pnpm",
		`export PATH="/x:$PATH"; pnpm dev`:             "pnpm",
		"cd /srv && FOO=1 bar run":                     "bar",
		`"C:\Program Files\nodejs\node.exe" server.js`: `node.exe`,
		"": "",
	}
	for input, want := range cases {
		got := CommandBinary(input)
		base := got
		if i := strings.LastIndexByte(base, '\\'); i >= 0 {
			base = base[i+1:]
		}
		if base != want {
			t.Errorf("CommandBinary(%q) = %q, want %q", input, got, want)
		}
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
