package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenSchemaPath = "schema/dnser.manifest.schema.json"

func TestGoldenSchemaMatchesGenerated(t *testing.T) {
	want, err := os.ReadFile(goldenSchemaPath)
	if err != nil {
		t.Fatalf("golden schema missing (run: go run ./cmd/dnser schema > %s): %v", goldenSchemaPath, err)
	}
	got, err := GenerateManifestSchema()
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		out := filepath.Join(t.TempDir(), "actual.json")
		if err := os.WriteFile(out, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("committed schema drifted from generated types.\nregenerate with:\n  go run ./cmd/dnser schema > %s\ndiff:\n  diff %s %s", goldenSchemaPath, goldenSchemaPath, out)
	}
}

func TestSchemaRejectsAdditionalProperties(t *testing.T) {
	got, err := GenerateManifestSchema()
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, must := range []string{
		`"additionalProperties": false`,
		`"domain"`,
		`"env_file"`,
		`"availability"`,
	} {
		if !strings.Contains(s, must) {
			t.Fatalf("schema missing %s", must)
		}
	}
}
