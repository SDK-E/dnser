package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Result struct {
	Port       int
	Framework  string
	Confidence string
}

var frameworkPorts = []struct {
	name  string
	files []string
	port  int
}{
	{"Next.js", []string{"next.config.js", "next.config.mjs", "next.config.ts"}, 3000},
	{"Nuxt", []string{"nuxt.config.ts", "nuxt.config.js"}, 3000},
	{"Create React App", []string{"craco.config.js"}, 3000},
	{"Vite", []string{"vite.config.js", "vite.config.ts"}, 5173},
	{"SvelteKit", []string{"svelte.config.js"}, 5173},
	{"Astro", []string{"astro.config.mjs", "astro.config.js"}, 4321},
	{"Angular", []string{"angular.json"}, 4200},
	{"Remix", []string{"remix.config.js"}, 3000},
}

var envPortRe = regexp.MustCompile(`(?:PORT|port)[= ](\d{2,5})`)

func DetectPort(dir string) (Result, error) {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("resolve cwd: %w", err)
		}
		dir = cwd
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("%s is not a directory", dir)
	}

	for _, fw := range frameworkPorts {
		for _, f := range fw.files {
			if fileExists(filepath.Join(dir, f)) {
				return fromFramework(fw.name, fw.port, dir), nil
			}
		}
	}

	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		return detectFromPackageJSON(data, dir), nil
	}
	if fileExists(filepath.Join(dir, "manage.py")) {
		return Result{Port: 8000, Framework: "Django", Confidence: "medium"}, nil
	}
	if fileExists(filepath.Join(dir, "Gemfile")) {
		return Result{Port: 3000, Framework: "Rails", Confidence: "medium"}, nil
	}
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		return Result{Port: 8080, Framework: "Rust (Axum/Actix)", Confidence: "low"}, nil
	}
	if fileExists(filepath.Join(dir, "go.mod")) {
		return Result{Port: 8080, Framework: "Go service", Confidence: "low"}, nil
	}
	return Result{}, nil
}

func detectFromPackageJSON(data []byte, dir string) Result {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return Result{}
	}

	if port := portFromScripts(pkg.Scripts); port > 0 {
		return Result{Port: port, Framework: "package.json script", Confidence: "high"}
	}
	for dep, port := range map[string]int{
		"next": 3000, "vite": 5173, "astro": 4321,
		"@angular/core": 4200, "@remix-run/node": 3000, "svelte": 5173,
	} {
		if _, ok := pkg.Deps[dep]; ok {
			return fromFramework(depLabel(dep), port, dir)
		}
		if _, ok := pkg.DevDeps[dep]; ok {
			return fromFramework(depLabel(dep), port, dir)
		}
	}
	return Result{Port: 3000, Framework: "Node.js", Confidence: "low"}
}

func portFromScripts(scripts map[string]string) int {
	for _, key := range []string{"dev", "start", "serve"} {
		script, ok := scripts[key]
		if !ok {
			continue
		}
		if m := envPortRe.FindStringSubmatch(script); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil && validPort(p) {
				return p
			}
		}
	}
	return 0
}

func fromFramework(name string, port int, dir string) Result {
	confidence := "medium"
	if p := portFromConfigFile(dir); p > 0 {
		port = p
		confidence = "high"
	}
	return Result{Port: port, Framework: name, Confidence: confidence}
}

func portFromConfigFile(dir string) int {
	for _, cfg := range []string{"vite.config.ts", "vite.config.js"} {
		data, err := os.ReadFile(filepath.Join(dir, cfg))
		if err != nil {
			continue
		}
		re := regexp.MustCompile(`port:\s*(\d{2,5})`)
		if m := re.FindStringSubmatch(string(data)); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil && validPort(p) {
				return p
			}
		}
	}
	return 0
}

func depLabel(dep string) string {
	switch dep {
	case "@angular/core":
		return "Angular"
	case "@remix-run/node":
		return "Remix"
	default:
		return strings.ToUpper(dep[:1]) + dep[1:]
	}
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
