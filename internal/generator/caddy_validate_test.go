package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaddyValidateGoldens(t *testing.T) {
	bin, err := exec.LookPath("caddy")
	if err != nil {
		t.Skip("caddy binary not installed; skipping real validation")
	}
	goldens, err := filepath.Glob(filepath.Join("testdata", "golden", "*.caddyfile"))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("no goldens found: %v", err)
	}
	for _, g := range goldens {
		t.Run(filepath.Base(g), func(t *testing.T) {
			raw, err := os.ReadFile(g)
			if err != nil {
				t.Fatal(err)
			}
			logsRoot := t.TempDir()
			content := strings.ReplaceAll(string(raw), "/home/dev/.dnser/logs", logsRoot)
			tmpPath := filepath.Join(t.TempDir(), "Caddyfile.test")
			if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			v := CaddyValidate(context.Background(), bin)
			if err := v(tmpPath); err != nil {
				t.Fatalf("caddy rejected generated config: %v", err)
			}
		})
	}
}

func TestEmitFileWithRealCaddyValidator(t *testing.T) {
	bin, err := exec.LookPath("caddy")
	if err != nil {
		t.Skip("caddy binary not installed; skipping real validation")
	}
	valid := []byte("generated.test {\n\trespond \"ok\" 200\n}\n")
	path := filepath.Join(t.TempDir(), "sites.caddyfile")
	if err := EmitFile(path, valid, 0o644, CaddyValidate(context.Background(), bin)); err != nil {
		t.Fatalf("valid config must pass through validator: %v", err)
	}
	invalid := []byte("generated.test {\n\tbogus_directive xyz\n}\n")
	if err := EmitFile(path, invalid, 0o644, CaddyValidate(context.Background(), bin)); err == nil {
		t.Fatalf("invalid config must be rejected")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(valid) {
		t.Fatalf("last-known-good must survive rejected swap")
	}
}
