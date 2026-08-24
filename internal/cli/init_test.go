package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SDK-E/dnser/internal/config"
)

func TestInitWritesValidManifestsForRequiredTypes(t *testing.T) {
	tests := []struct {
		typ      string
		wantCmd  string
		wantPort bool
	}{
		{"nodejs", "npm run dev", true},
		{"laravel", "php artisan serve", true},
		{"bash", "./serve.sh", false},
		{"static", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			dir := t.TempDir()
			cmd := NewInitCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--type", tt.typ, dir})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("init failed: %v", err)
			}
			m := loadManifest(t, dir)
			if m.Type != tt.typ {
				t.Fatalf("type = %q", m.Type)
			}
			if tt.wantCmd != "" && !strings.Contains(m.Command, tt.wantCmd) {
				t.Fatalf("command = %q", m.Command)
			}
			if tt.typ == "static" && m.Command != "" {
				t.Fatalf("static must be proxy-only (no command), got %q", m.Command)
			}
			assertManifestValid(t, m)
		})
	}
}

func TestInitDetectsStackFromDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := NewInitCommand()
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	m := loadManifest(t, dir)
	if m.Type != "nodejs" {
		t.Fatalf("detection failed, got %q", m.Type)
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	cmd := NewInitCommand()
	cmd.SetArgs([]string{"--type", "go", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	before := loadManifest(t, dir)

	cmd2 := NewInitCommand()
	cmd2.SetArgs([]string{"--type", "bash", dir})
	if err := cmd2.Execute(); err == nil {
		t.Fatalf("second init without --force must fail")
	}
	after := loadManifest(t, dir)
	if after.Type != before.Type {
		t.Fatalf("manifest was modified despite refusal")
	}

	cmd3 := NewInitCommand()
	cmd3.SetArgs([]string{"--type", "bash", "--force", dir})
	if err := cmd3.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := loadManifest(t, dir); got.Type != "bash" {
		t.Fatalf("force overwrite failed: %+v", got.Type)
	}
}

func TestInitUnknownTypeExitsUsageStyle(t *testing.T) {
	dir := t.TempDir()
	cmd := NewInitCommand()
	cmd.SetArgs([]string{"--type", "cobol", dir})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("want unknown type error, got %v", err)
	}
}

func loadManifest(t *testing.T, dir string) *config.Manifest {
	t.Helper()
	path, ok, err := config.Find(dir)
	if err != nil || !ok {
		t.Fatalf("manifest not found in %s: %v", dir, err)
	}
	m, err := config.Load(path)
	if err != nil {
		t.Fatalf("written manifest does not decode: %v", err)
	}
	return m
}

func assertManifestValid(t *testing.T, m *config.Manifest) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
}
