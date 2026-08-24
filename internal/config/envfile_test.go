package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilesFirstWinsAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, ".env.a")
	b := filepath.Join(dir, ".env.b")
	if err := os.WriteFile(a, []byte("SECRET_A=from-a\nSHARED=first\nQUOTED=\"quoted-value\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("SECRET_B=from-b\nSHARED=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadEnvFiles([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if got["SECRET_A"] != "from-a" || got["SECRET_B"] != "from-b" {
		t.Fatalf("values wrong: %+v", got)
	}
	if got["SHARED"] != "first" {
		t.Fatalf("earlier file must win: %q", got["SHARED"])
	}
	if got["QUOTED"] != "quoted-value" {
		t.Fatalf("quoting handling wrong: %q", got["QUOTED"])
	}
}

func TestLoadEnvFilesMissingErrors(t *testing.T) {
	if _, err := LoadEnvFiles([]string{filepath.Join(t.TempDir(), "missing.env")}); err == nil {
		t.Fatalf("expected error for missing env file")
	}
}

func TestMergeEnvManifestWins(t *testing.T) {
	files := map[string]string{"K": "file", "ONLY_FILE": "x"}
	manifest := map[string]string{"K": "manifest"}
	values, sources := MergeEnv(files, manifest)
	if values["K"] != "manifest" || sources["K"] != SourceManifest.String() {
		t.Fatalf("manifest env must win over env_file: %+v %+v", values, sources)
	}
	if values["ONLY_FILE"] != "x" || sources["ONLY_FILE"] != "env_file" {
		t.Fatalf("env_file value lost")
	}
}

func TestRedaction(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL": "postgres://u:p@h/db",
		"PORT":         "3000",
		"DNSER_DOMAIN": "app.test",
		"APP_ENV":      "local",
	}
	out := RedactEnv(values)
	if out["DATABASE_URL"] != redactedPlaceholder {
		t.Fatalf("secret leaked: %q", out["DATABASE_URL"])
	}
	if out["PORT"] != "3000" || out["DNSER_DOMAIN"] != "app.test" || out["APP_ENV"] != "local" {
		t.Fatalf("operational keys must stay visible: %+v", out)
	}
	if RedactValue("API_TOKEN", "abc") == "abc" {
		t.Fatalf("RedactValue must mask secrets")
	}
}
