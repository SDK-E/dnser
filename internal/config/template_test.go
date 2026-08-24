package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryEmbeddedDefaults(t *testing.T) {
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"nodejs", "laravel", "symfony", "rails", "go", "python", "bash", "sh", "zsh", "static"} {
		if _, ok := registry[want]; !ok {
			t.Fatalf("embedded template %q missing; have %v", want, KnownTypes(registry))
		}
	}
	node := registry["nodejs"]
	if node.Detect.DefaultPort != 3000 || node.Command == "" {
		t.Fatalf("nodejs template malformed: %+v", node)
	}
	if _, ok := node.Env["NODE_OPTIONS"]; !ok {
		t.Fatalf("nodejs env tuning missing")
	}
}

func TestUserTemplatesOverrideEmbedded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".dnser", "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := "name: nodejs\ndescription: overridden\ncommand: pnpm dev --port {port}\n"
	if err := os.WriteFile(filepath.Join(dir, "nodejs.yaml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	custom := "name: deno\ndescription: user added\ncommand: deno task dev\n"
	if err := os.WriteFile(filepath.Join(dir, "deno.yaml"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := registry["nodejs"]
	if !ok || got.Command != "pnpm dev --port {port}" {
		t.Fatalf("override not applied: %+v", got)
	}
	if _, ok := registry["deno"]; !ok {
		t.Fatalf("user-added template missing")
	}

	bad := "name: nope\nbogus_key: 1\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(); err == nil {
		t.Fatalf("strict decode should reject unknown template keys")
	}
}

func TestGetTemplateUnknown(t *testing.T) {
	_, err := GetTemplate("cobol")
	if err == nil {
		t.Fatalf("expected error for unknown type")
	}
}
