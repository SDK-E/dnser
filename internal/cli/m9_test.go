package cli

import (
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/buildinfo"
)

func TestVersionIsSane(t *testing.T) {
	v := buildinfo.Version()
	if v == "" || v == "(devel)" {
		t.Fatalf("version must never surface (devel): %q", v)
	}
}

func TestRewriteLegacyManifest(t *testing.T) {
	legacy := "label: myapp\ntld: dev\nname: myapp\nport: 3000\ncommand: npm run dev\n"
	rewrites := detectLegacyKeys([]byte(legacy))
	if len(rewrites) != 3 {
		t.Fatalf("expected 3 rewrites, got %v", rewrites)
	}
	out := string(rewriteLegacyManifest([]byte(legacy), rewrites))
	for _, want := range []string{"domain: myapp.test", "port: 3000", "command: npm run dev"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	for _, banned := range []string{"tld:", "name:", "label:"} {
		if strings.Contains(out, banned) {
			t.Fatalf("legacy key %q survived rewrite:\n%s", banned, out)
		}
	}
}

func TestDetectLegacyKeysIgnoresCleanManifests(t *testing.T) {
	clean := "domain: myapp.test\nport: 3000\n"
	if got := detectLegacyKeys([]byte(clean)); len(got) != 0 {
		t.Fatalf("clean manifest flagged: %v", got)
	}
}

func TestClassifyInstallPaths(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/opt/homebrew/bin/dnser", "brew"},
		{"/home/linuxbrew/.linuxbrew/bin/dnser", "brew"},
		{"/usr/local/bin/dnser", "manual"},
		{"/Users/x/go/bin/dnser", "script"},
	}
	for _, tt := range tests {
		if got := classifyInstall(tt.path)[0]; got != tt.want {
			t.Fatalf("%s → %q want %q", tt.path, got, tt.want)
		}
	}
}
