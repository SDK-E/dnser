package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const pathCacheVersion = 1

const pathCacheFile = "path-cache.json"

type PathOptions struct {
	UserHome    string
	DnserHome   string
	CurrentPATH string
	ExtraPATH   string
	Shell       string
	TTL         time.Duration
	Clock       func() time.Time
	Capture     func(shell string) (string, error)
}

type pathCache struct {
	Version    int       `json:"version"`
	Shell      string    `json:"shell"`
	Path       string    `json:"path"`
	CapturedAt time.Time `json:"captured_at"`
}

type PathResolver struct {
	opts    PathOptions
	mu      sync.Mutex
	dirs    []string
	login   string
	builtAt time.Time
}

func NewPathResolver(opts PathOptions) *PathResolver {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.CurrentPATH == "" {
		opts.CurrentPATH = os.Getenv("PATH")
	}
	if opts.ExtraPATH == "" {
		opts.ExtraPATH = os.Getenv("DNSER_EXTRA_PATH")
	}
	if opts.UserHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.UserHome = home
		}
	}
	if opts.Capture == nil {
		opts.Capture = captureLoginShellPath
	}
	return &PathResolver{opts: opts}
}

func DefaultPathRefresh() time.Duration { return 24 * time.Hour }

func (p *PathResolver) ttl() time.Duration {
	if p.opts.TTL > 0 {
		return p.opts.TTL
	}
	return DefaultPathRefresh()
}

func (p *PathResolver) Dirs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.opts.Clock()
	if p.dirs != nil && now.Sub(p.builtAt) < p.ttl() {
		return p.dirs
	}
	p.refreshLocked(now)
	return p.dirs
}

func (p *PathResolver) String() string {
	return strings.Join(p.Dirs(), string(os.PathListSeparator))
}

func (p *PathResolver) refreshLocked(now time.Time) {
	ttl := p.ttl()
	cachedPath, cachedAt, cachedShell := p.readCache()
	currentShell := p.opts.Shell
	if currentShell == "" {
		currentShell = os.Getenv("SHELL")
	}
	var loginPath string
	if !cachedAt.IsZero() && now.Sub(cachedAt) < ttl && cachedPath != "" && (currentShell == "" || cachedShell == currentShell) {
		loginPath = cachedPath
	} else {
		loginPath = p.capture(cachedPath, cachedAt)
	}
	p.login = loginPath
	p.dirs = BuildPathList(p.opts.CurrentPATH, loginPath, p.opts.ExtraPATH, p.opts.UserHome, runtime.GOOS, os.Getenv)
	p.builtAt = now
}

func (p *PathResolver) capture(stalePath string, staleAt time.Time) string {
	shell := p.opts.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if p.opts.Capture == nil || shell == "" {
		return stalePath
	}
	fresh, err := p.opts.Capture(shell)
	if err != nil || strings.TrimSpace(fresh) == "" {
		p.writeCache(pathCache{Version: pathCacheVersion, Shell: shell, Path: stalePath, CapturedAt: staleAt})
		return stalePath
	}
	p.writeCache(pathCache{Version: pathCacheVersion, Shell: shell, Path: fresh, CapturedAt: p.opts.Clock()})
	return fresh
}

func (p *PathResolver) cachePath() string {
	if p.opts.DnserHome == "" {
		return ""
	}
	return filepath.Join(p.opts.DnserHome, pathCacheFile)
}

func (p *PathResolver) readCache() (string, time.Time, string) {
	file := p.cachePath()
	if file == "" {
		return "", time.Time{}, ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", time.Time{}, ""
	}
	var c pathCache
	if json.Unmarshal(data, &c) != nil || c.Version != pathCacheVersion {
		return "", time.Time{}, ""
	}
	return c.Path, c.CapturedAt, c.Shell
}

func (p *PathResolver) writeCache(c pathCache) {
	file := p.cachePath()
	if file == "" {
		return
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	tmp := file + ".tmp"
	if os.WriteFile(tmp, data, 0o644) != nil {
		return
	}
	if os.Rename(tmp, file) != nil {
		_ = os.Remove(tmp)
	}
}

func BuildPathList(current, loginPath, extra, userHome, goos string, getenv func(string) string) []string {
	sep := string(os.PathListSeparator)
	var out []string
	seen := map[string]bool{}
	add := func(entry string) {
		entry = strings.TrimSpace(entry)
		if entry == "" || seen[entry] {
			return
		}
		seen[entry] = true
		out = append(out, entry)
	}
	for _, entry := range strings.Split(extra, sep) {
		add(entry)
	}
	for _, entry := range strings.Split(loginPath, sep) {
		add(entry)
	}
	for _, dir := range knownToolDirs(userHome, goos, getenv) {
		if dirExists(dir) {
			add(dir)
		}
	}
	for _, entry := range strings.Split(current, sep) {
		add(entry)
	}
	return out
}

func knownToolDirs(home, goos string, getenv func(string) string) []string {
	var dirs []string
	dirs = append(dirs, platformSystemDirs(goos, getenv)...)
	dirs = append(dirs, userDirs(home, goos)...)
	dirs = append(dirs, toolchainDirs(home, goos, getenv)...)
	return dirs
}

func platformSystemDirs(goos string, getenv func(string) string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			"/usr/local/bin",
			"/usr/local/sbin",
		}
	case "linux":
		return []string{
			"/usr/local/sbin",
			"/usr/local/bin",
			"/snap/bin",
		}
	case "windows":
		return []string{
			filepath.Join(getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps"),
			filepath.Join(getenv("ProgramData"), "chocolatey", "bin"),
		}
	default:
		return nil
	}
}

func userDirs(home, goos string) []string {
	if home == "" {
		return nil
	}
	dirs := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
	}
	if goos == "windows" {
		return nil
	}
	return dirs
}

func toolchainDirs(home, goos string, getenv func(string) string) []string {
	if home == "" {
		return nil
	}
	var dirs []string
	add := func(parts ...string) {
		dirs = append(dirs, filepath.Join(parts...))
	}
	envOr := func(env, def string) string {
		if v := getenv(env); v != "" {
			return v
		}
		return def
	}
	add(envOr("CARGO_HOME", filepath.Join(home, ".cargo")), "bin")
	add(envOr("GOPATH", filepath.Join(home, "go")), "bin")
	add(envOr("BUN_INSTALL", filepath.Join(home, ".bun")), "bin")
	add(filepath.Join(home, ".deno"), "bin")

	volta := envOr("VOLTA_HOME", filepath.Join(home, ".volta"))
	if goos == "windows" && volta == filepath.Join(home, ".volta") {
		if localAppData := getenv("LOCALAPPDATA"); localAppData != "" {
			volta = filepath.Join(localAppData, "Volta")
		}
	}
	add(volta, "bin")

	switch goos {
	case "windows":
		add(envOr("PNPM_HOME", filepath.Join(getenv("LOCALAPPDATA"), "pnpm")))
	default:
		add(envOr("PNPM_HOME", pnpmDefaultHome(home, goos)))
	}

	nvmRoot := envOr("NVM_DIR", filepath.Join(home, ".nvm"))
	dirs = append(dirs, globDirs(filepath.Join(nvmRoot, "versions", "node", "*", "bin"))...)

	fnmRoots := []string{
		envOr("FNM_DIR", ""),
		filepath.Join(envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "fnm"),
		filepath.Join(home, ".fnm"),
	}
	if ms := getenv("FNM_MULTISHELL_PATH"); ms != "" {
		fnmRoots = append([]string{filepath.Join(ms)}, fnmRoots...)
	}
	for _, root := range fnmRoots {
		if root == "" {
			continue
		}
		if dirExists(filepath.Join(root, "bin")) {
			dirs = append(dirs, filepath.Join(root, "bin"))
			continue
		}
		dirs = append(dirs, globDirs(filepath.Join(root, "node-versions", "*", "installation", "bin"))...)
	}

	asdf := envOr("ASDF_DIR", filepath.Join(home, ".asdf"))
	add(asdf, "shims")
	mise := envOr("MISE_DATA_DIR", filepath.Join(envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "mise"))
	add(mise, "shims")
	return dirs
}

func pnpmDefaultHome(home, goos string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "pnpm")
	default:
		return filepath.Join(home, ".local", "share", "pnpm")
	}
}

func globDirs(pattern string) []string {
	if !strings.Contains(pattern, "*") {
		return nil
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if dirExists(m) {
			out = append(out, m)
		}
	}
	return out
}

func dirExists(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
