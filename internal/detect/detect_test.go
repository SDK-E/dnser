package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectVite(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vite.config.ts", "export default defineConfig({ server: { port: 5174 } })")
	res, err := DetectPort(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Framework != "Vite" || res.Port != 5174 || res.Confidence != "high" {
		t.Errorf("got %+v", res)
	}
}

func TestDetectNextJS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "next.config.mjs", "export default {}")
	res, err := DetectPort(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Framework != "Next.js" || res.Port != 3000 {
		t.Errorf("got %+v", res)
	}
}

func TestDetectPackageJSONScriptPort(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
	  "name": "app",
	  "scripts": { "dev": "PORT=4000 node server.js" }
	}`)
	res, err := DetectPort(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Port != 4000 || res.Confidence != "high" {
		t.Errorf("got %+v", res)
	}
}

func TestDetectDependenciesFallback(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{ "dependencies": { "@angular/core": "^18.0.0" } }`)
	res, _ := DetectPort(dir)
	if res.Port != 4200 || res.Framework != "Angular" {
		t.Errorf("got %+v", res)
	}
}

func TestDetectNonJSProjects(t *testing.T) {
	django := t.TempDir()
	writeFile(t, django, "manage.py", "#!/usr/bin/env python")
	res, _ := DetectPort(django)
	if res.Port != 8000 || res.Framework != "Django" {
		t.Errorf("django: got %+v", res)
	}

	gomod := t.TempDir()
	writeFile(t, gomod, "go.mod", "module example.com/x\n\ngo 1.27\n")
	res, _ = DetectPort(gomod)
	if res.Framework != "Go service" {
		t.Errorf("go: got %+v", res)
	}
}

func TestDetectEmptyDirReturnsZero(t *testing.T) {
	res, err := DetectPort(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if res.Port != 0 {
		t.Errorf("empty dir should yield no detection, got %+v", res)
	}
}

func TestDetectMissingDirErrors(t *testing.T) {
	if _, err := DetectPort(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error for missing directory")
	}
}
