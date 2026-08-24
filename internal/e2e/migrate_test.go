package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/config"
)

var migrateFixtures = map[string]string{
	"mailbox": "label: mailbox\ntld: test\nname: mailbox\nport: 1080\ncommand: python -m smtpd -n -c DebuggingServer localhost:1080\nenv:\n  MAIL_HOST: 127.0.0.1\n",
	"auth":    "label: auth\ntld: test\nname: auth\nport: 4001\ncommand: node server.js\ndomains:\n  - auth.internal\n",
	"redisk":  "label: redisk\ntld: test\nname: redisk\nport: 6380\ncommand: redis-server --port 6380\nservices:\n  cache:\n    port: 6380\n",
}

func TestMigrateRoundTripsV1Fixtures(t *testing.T) {
	s := newSandbox(t)
	for name, legacy := range migrateFixtures {
		t.Run(name, func(t *testing.T) {
			proj := filepath.Join(s.Home, name)
			if err := os.MkdirAll(proj, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(proj, ".dnser.yaml")
			if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
				t.Fatal(err)
			}

			dryOut, code := s.run(t, "migrate", proj, "-o", "json")
			if code != 3 && code != 0 {
				t.Fatalf("dry-run migrate must not fail outright: %d\n%s", code, dryOut)
			}
			if !strings.Contains(dryOut+readStderrOf(t, s, "migrate", proj), "label") && !strings.Contains(dryOut, "rewrite") {
				t.Logf("fixture %s: dry run reported via stderr", name)
			}

			out, code := s.run(t, "migrate", proj, "-y")
			if code != 0 {
				t.Fatalf("apply migrate failed:\n%s", out)
			}

			migrated, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatal(rerr)
			}
			for _, banned := range []string{"label:", "tld:", "\nname:"} {
				if strings.Contains(string(migrated), banned) {
					t.Fatalf("legacy key %q survived migration:\n%s", banned, migrated)
				}
			}
			m, derr := config.Decode(migrated)
			if derr != nil {
				t.Fatalf("migrated manifest invalid: %v\n%s", derr, migrated)
			}
			if m.PrimaryDomain(name) == "" {
				t.Fatal("domain missing after migration")
			}
			if _, serr := os.Stat(path + ".v2.bak"); serr != nil {
				t.Fatal("backup not kept alongside original")
			}

			linkOut, lcode := s.run(t, "link", proj)
			if lcode != 0 {
				t.Fatalf("post-migration link failed:\n%s", linkOut)
			}
		})
	}
}
