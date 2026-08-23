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
		return Result{Port: 8000, Framework: StackDjango, Confidence: "medium"}, nil
	}
	if fw := detectComposer(dir); fw != "" {
		port := 8000
		if fw == StackSymfony {
			port = 8000
		}
		return Result{Port: port, Framework: fw, Confidence: "high"}, nil
	}
	if fw := detectSpring(dir); fw != "" {
		return Result{Port: 8080, Framework: fw, Confidence: "high"}, nil
	}
	if fileExists(filepath.Join(dir, "Gemfile")) {
		return Result{Port: 3000, Framework: StackRails, Confidence: "medium"}, nil
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

const (
	StackNextJS       = "Next.js"
	StackNuxt         = "Nuxt"
	StackVite         = "Vite"
	StackSvelte       = "SvelteKit"
	StackAstro        = "Astro"
	StackAngular      = "Angular"
	StackRemix        = "Remix"
	StackCRA          = "Create React App"
	StackDjango       = "Django"
	StackRails        = "Rails"
	StackGo           = "Go service"
	StackRust         = "Rust (Axum/Actix)"
	StackLaravel      = "Laravel"
	StackSymfony      = "Symfony"
	StackSpringMaven  = "Spring Boot (maven)"
	StackSpringGradle = "Spring Boot (gradle)"
)

type Recipe struct {
	Command  []string
	UseShell bool
	PortEnv  bool
}

func DetectStack(dir string) (Result, Recipe, error) {
	res, err := DetectPort(dir)
	if err != nil {
		return res, Recipe{}, err
	}
	return res, buildRecipe(res.Framework, dir), nil
}

func buildRecipe(framework, dir string) Recipe {
	pm := packageManager(dir)
	switch framework {
	case StackNextJS, StackNuxt, StackRemix, StackCRA:
		return Recipe{Command: pm.run("dev"), PortEnv: true}
	case StackVite, StackSvelte, StackAstro:
		if pm.name == "npm" {
			return Recipe{Command: []string{"npm", "run", "dev", "--", "--port", "{port}"}}
		}
		return Recipe{Command: append(pm.run("dev"), "--port", "{port}")}
	case StackAngular:
		if pm.name == "npm" {
			return Recipe{Command: []string{"npm", "run", "start", "--", "--port", "{port}"}}
		}
		return Recipe{Command: append(pm.run("start"), "--port", "{port}")}
	case StackDjango:
		return Recipe{Command: []string{"python", "manage.py", "runserver", "127.0.0.1:{port}"}}
	case StackRails:
		return Recipe{Command: []string{"bin/rails", "server", "-p", "{port}", "-b", "127.0.0.1"}}
	case StackGo:
		return Recipe{Command: []string{"go", "run", "."}, PortEnv: true}
	case StackRust:
		return Recipe{Command: []string{"cargo", "run"}, PortEnv: true}
	case StackLaravel:
		return Recipe{Command: []string{"php", "artisan", "serve", "--host=127.0.0.1", "--port={port}"}}
	case StackSymfony:
		return Recipe{Command: []string{"php", "-S", "127.0.0.1:{port}", "-t", "public"}}
	case StackSpringMaven:
		if fileExists(filepath.Join(dir, "mvnw")) {
			return Recipe{Command: []string{"./mvnw", "-q", "spring-boot:run", "-Dspring-boot.run.arguments=--server.port={port}"}}
		}
		return Recipe{Command: []string{"mvn", "-q", "spring-boot:run", "-Dspring-boot.run.arguments=--server.port={port}"}}
	case StackSpringGradle:
		wrapper := "./gradlew"
		if !fileExists(filepath.Join(dir, "gradlew")) {
			wrapper = "gradle"
		}
		return Recipe{Command: []string{wrapper, "bootRun", "--args=--server.port={port}"}}
	}
	return Recipe{}
}

type pkgManager struct {
	name    string
	bin     string
	runArgs []string
}

func (p pkgManager) run(script string) []string {
	return append([]string{p.bin}, append(append([]string{}, p.runArgs...), script)...)
}

func packageManager(dir string) pkgManager {
	switch {
	case fileExists(filepath.Join(dir, "bun.lockb")), fileExists(filepath.Join(dir, "bun.lock")):
		return pkgManager{name: "bun", bin: "bun"}
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return pkgManager{name: "pnpm", bin: "pnpm", runArgs: []string{"run"}}
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return pkgManager{name: "yarn", bin: "yarn"}
	default:
		return pkgManager{name: "npm", bin: "npm", runArgs: []string{"run"}}
	}
}

func DepsInstalled(dir string, framework string) bool {
	markers := map[string][]string{
		StackNextJS: {"node_modules"}, StackNuxt: {"node_modules"},
		StackVite: {"node_modules"}, StackSvelte: {"node_modules"},
		StackAstro: {"node_modules"}, StackAngular: {"node_modules"},
		StackRemix: {"node_modules"}, StackCRA: {"node_modules"},
		StackLaravel: {"vendor"}, StackSymfony: {"vendor"},
		StackRust: {"target"},
	}
	files, ok := markers[framework]
	if !ok {
		return true
	}
	for _, f := range files {
		info, err := os.Stat(filepath.Join(dir, f))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func InstallHint(framework string) string {
	hints := map[string]string{
		StackNextJS:  "pnpm install  (or npm install)",
		StackNuxt:    "pnpm install",
		StackVite:    "pnpm install",
		StackSvelte:  "pnpm install",
		StackAstro:   "pnpm install",
		StackAngular: "npm install",
		StackRemix:   "npm install",
		StackCRA:     "npm install",
		StackLaravel: "composer install",
		StackSymfony: "composer install",
		StackRust:    "cargo fetch",
	}
	if h, ok := hints[framework]; ok {
		return h
	}
	return ""
}

func detectComposer(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "composer.json")); err == nil {
		var comp struct {
			Require map[string]string `json:"require"`
		}
		if json.Unmarshal(data, &comp) == nil {
			if _, ok := comp.Require["laravel/framework"]; ok {
				return StackLaravel
			}
			if _, ok := comp.Require["symfony/framework-bundle"]; ok {
				return StackSymfony
			}
		}
	}
	switch {
	case fileExists(filepath.Join(dir, "artisan")):
		return StackLaravel
	case fileExists(filepath.Join(dir, "symfony.lock")):
		return StackSymfony
	}
	return ""
}

func detectSpring(dir string) string {
	if fileExists(filepath.Join(dir, "pom.xml")) &&
		fileContains(filepath.Join(dir, "pom.xml"), "spring-boot-starter", "spring-boot-maven-plugin") {
		return StackSpringMaven
	}
	for _, g := range []string{"build.gradle", "build.gradle.kts"} {
		if fileExists(filepath.Join(dir, g)) && fileContains(filepath.Join(dir, g), "org.springframework.boot") {
			return StackSpringGradle
		}
	}
	return ""
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func fileContains(path string, needles ...string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return containsAny(string(data), needles...)
}
