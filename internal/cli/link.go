package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/detect"
	"github.com/SDK-E/dnser/internal/runner"
)

func newLinkCmd() *cobra.Command {
	var (
		domain     string
		port       int
		tld        string
		wildcard   bool
		noHTTPS    bool
		forceHTTPS bool
		aliases    []string
		command    string
		noRun      bool
	)
	cmd := &cobra.Command{
		Use:   "link <path>",
		Short: "Link a project directory — detects the stack, assigns a port, and serves it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			if info, err := os.Stat(abs); err != nil || !info.IsDir() {
				return fmt.Errorf("not a directory: %s", abs)
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			settings := store.Settings()
			useTLD := settings.TLD
			if tld != "" {
				useTLD = tld
			}
			if domain == "" {
				domain = filepath.Base(abs)
			}
			fullDomain, err := config.EnsureTLD(domain, useTLD)
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}

			result, recipe, err := detect.DetectStack(abs)
			framework := result.Framework
			if err != nil {
				framework = ""
			}

			var runCmd []string
			switch {
			case strings.TrimSpace(command) != "":
				runCmd = strings.Fields(command)
			default:
				if override, ok := runner.ReadLinkOverride(abs); ok {
					runCmd = strings.Fields(override.Command)
				} else if len(recipe.Command) > 0 {
					runCmd = recipe.Command
				}
			}
			runCommandStr := strings.Join(runCmd, " ")

			var existingRun *config.RunConfig
			for _, p := range store.Get().Projects {
				if p.Domain == fullDomain {
					existingRun = p.Run
					break
				}
			}
			if existingRun != nil && existingRun.Port > 0 && !containsFlag(cmd, "port") {
				port = existingRun.Port
			}
			portOwnedByProject := existingRun != nil && existingRun.Port == port
			if port > 0 && !portAvailable(port) && !portOwnedByProject {
				return fmt.Errorf("port %d is already in use — pick another with --port", port)
			}
			if port == 0 {
				allocated, err := runner.AllocateFreePort(portExcludes(store.Get(), ""))
				if err != nil {
					return fmt.Errorf("allocate port: %w", err)
				}
				port = allocated
			}

			normAliases := make([]string, 0, len(aliases))
			for _, a := range aliases {
				na, err := config.EnsureTLD(a, useTLD)
				if err != nil {
					return fmt.Errorf("alias %q: %w", a, err)
				}
				if na != fullDomain {
					normAliases = append(normAliases, na)
				}
			}

			backend := net.JoinHostPort("localhost", strconv.Itoa(port))
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != fullDomain {
						continue
					}
					p := &c.Projects[i]
					applyLink(p, abs, runCommandStr, port, noRun)
					p.Routes = patchRoutes(p.Routes, backend, wildcard, normAliases, !noHTTPS, forceHTTPS)
					return
				}
				p := &config.Project{Domain: fullDomain}
				applyLink(p, abs, runCommandStr, port, noRun)
				p.Routes = buildRoutes(backend, wildcard, normAliases, !noHTTPS, forceHTTPS)
				c.Projects = append(c.Projects, *p)
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "✓ linked %s\n", fullDomain)
			fmt.Fprintf(out, "  path:  %s\n", abs)
			if framework != "" {
				fmt.Fprintf(out, "  stack: %s\n", framework)
			}
			switch {
			case noRun:
				fmt.Fprintf(out, "  serves localhost:%d (external process)\n", port)
			case len(runCmd) > 0:
				source := "detected"
				if containsFlag(cmd, "command") {
					source = "--command"
				} else if _, ok := runner.ReadLinkOverride(abs); ok {
					source = ".dnser.yaml"
				}
				fmt.Fprintf(out, "  runs:  %s (%s)\n", strings.Join(runCmd, " "), source)
				fmt.Fprintf(out, "  port:  %d\n", port)
			default:
				fmt.Fprintf(out, "  serves localhost:%d (no dev command detected)\n", port)
				fmt.Fprintf(out, "  tip:   create %s with a `command:` line to let dnser run it\n", filepath.Join(abs, ".dnser.yaml"))
			}
			if framework != "" && !detect.DepsInstalled(abs, framework) {
				if hint := detect.InstallHint(framework); hint != "" {
					fmt.Fprintf(out, "  note:  dependencies missing — run `%s`\n", hint)
				}
			}
			fmt.Fprintf(out, "  url:   http://%s:%d after `dnser start` (hot-reloads automatically)\n", fullDomain, port)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain name (defaults to the directory name)")
	cmd.Flags().IntVar(&port, "port", 0, "local port (allocated automatically when omitted)")
	cmd.Flags().StringVar(&tld, "tld", "", "override TLD for this link")
	cmd.Flags().BoolVar(&wildcard, "wildcard", false, "resolve all subdomains (*.domain)")
	cmd.Flags().BoolVar(&noHTTPS, "no-https", false, "disable TLS for this project")
	cmd.Flags().BoolVar(&forceHTTPS, "force-https", false, "redirect plain HTTP to HTTPS")
	cmd.Flags().StringSliceVar(&aliases, "alias", nil, "additional hostname(s) for this project")
	cmd.Flags().StringVar(&command, "command", "", "dev-server command to manage ({port} placeholder supported)")
	cmd.Flags().BoolVar(&noRun, "no-run", false, "do not manage a dev server; proxy to --port only")
	return cmd
}

func applyLink(p *config.Project, path, command string, port int, noRun bool) {
	p.Path = path
	if p.Run == nil {
		p.Run = &config.RunConfig{}
	}
	p.Run.Port = port
	if noRun {
		return
	}
	p.Run.Command = command
}

func patchRoutes(routes []config.Route, backend string, wildcard bool, aliases []string, https, forceHTTPS bool) []config.Route {
	patched := false
	for j := range routes {
		switch routes[j].Host {
		case "@":
			if len(routes[j].Backends) > 0 {
				routes[j].Backends[0] = backend
				routes[j].HTTPS = https
				routes[j].ForceHTTPS = forceHTTPS
				patched = true
			}
		case "*":
			if len(routes[j].Backends) > 0 {
				routes[j].Backends[0] = backend
			}
		}
	}
	if !patched {
		routes = append(routes, config.Route{Host: "@", Backends: []string{backend}, HTTPS: https, ForceHTTPS: forceHTTPS})
	}
	if wildcard && !hasWildcardRoute(routes) {
		routes = append(routes, config.Route{Host: "*", Backends: []string{backend}, HTTPS: https})
	}
	for _, a := range aliases {
		if !hasRouteFor(routes, a) {
			routes = append(routes, config.Route{Host: a, Backends: []string{backend}, HTTPS: https})
		}
	}
	return routes
}

func buildRoutes(backend string, wildcard bool, aliases []string, https, forceHTTPS bool) []config.Route {
	routes := []config.Route{{Host: "@", Backends: []string{backend}, HTTPS: https, ForceHTTPS: forceHTTPS}}
	if wildcard {
		routes = append(routes, config.Route{Host: "*", Backends: []string{backend}, HTTPS: https})
	}
	for _, a := range aliases {
		routes = append(routes, config.Route{Host: a, Backends: []string{backend}, HTTPS: https})
	}
	return routes
}

func portExcludes(cfg config.Config, skipDomain string) map[int]bool {
	exclude := map[int]bool{
		cfg.Settings.Ports.DNS:   true,
		cfg.Settings.Ports.HTTP:  true,
		cfg.Settings.Ports.HTTPS: true,
		cfg.Settings.Ports.UI:    true,
	}
	for _, p := range cfg.Projects {
		if p.Domain == skipDomain {
			continue
		}
		if p.Run != nil && p.Run.Port > 0 {
			exclude[p.Run.Port] = true
		}
		for _, r := range p.Routes {
			if r.Listen > 0 {
				exclude[r.Listen] = true
			}
			for _, b := range r.Backends {
				if _, portStr, err := net.SplitHostPort(b); err == nil {
					if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
						exclude[port] = true
					}
				}
			}
		}
	}
	return exclude
}

func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func containsFlag(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name)
}

func hasWildcardRoute(routes []config.Route) bool {
	for _, r := range routes {
		if r.Host == "*" {
			return true
		}
	}
	return false
}

func hasRouteFor(routes []config.Route, host string) bool {
	for _, r := range routes {
		if r.Host == host {
			return true
		}
	}
	return false
}

func newUnlinkCmd() *cobra.Command {
	var keepDNS bool
	cmd := &cobra.Command{
		Use:   "unlink <path|domain>",
		Short: "Unlink a project — stops its managed dev server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			if target == "" {
				return fmt.Errorf("specify a project path or domain")
			}
			norm := target
			if !strings.Contains(target, ".") || filepath.IsAbs(target) || strings.HasPrefix(target, ".") {
				if abs, err := filepath.Abs(target); err == nil {
					matched := ""
					for _, p := range store.Get().Projects {
						if samePath(p.Path, abs) {
							matched = p.Domain
							break
						}
					}
					if matched != "" {
						norm = matched
					}
				}
			}
			if nd, err := config.NormalizeDomain(norm); err == nil {
				norm = nd
			}

			var found bool
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != norm {
						continue
					}
					found = true
					p := &c.Projects[i]
					p.Path = ""
					p.Run = nil
					if keepDNS {
						return
					}
					c.Projects = append(c.Projects[:i], c.Projects[i+1:]...)
					return
				}
			})
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("project %q is not linked", norm)
			}
			bestEffortStop(norm)
			fmt.Fprintf(out, "✓ unlinked %s\n", norm)
			if keepDNS {
				fmt.Fprintf(out, "  DNS records kept; dev server management removed\n")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&keepDNS, "keep-dns", false, "keep DNS/routes, stop managing the app only")
	return cmd
}

func samePath(a, b string) bool {
	if a == "" {
		return false
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 == nil && err2 == nil {
		return ra == rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

type apiClient struct {
	base string
}

func newAPIClient(st config.Settings) *apiClient {
	return &apiClient{base: apiBase(st)}
}

func (c *apiClient) get(path string, out any) error {
	resp, err := httpGet(c.base + path)
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s (is it running?)", c.base)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d for %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *apiClient) post(path string, out any) error {
	resp, err := httpPost(c.base + path)
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s (is it running?)", c.base)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("daemon returned %d for %s", resp.StatusCode, path)
	}
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

var httpClient = &http.Client{Timeout: 3 * time.Second}

func httpGet(url string) (*http.Response, error) {
	return httpClient.Get(url)
}

func httpPost(url string) (*http.Response, error) {
	return httpClient.Post(url, "application/json", nil)
}

func bestEffortStop(domain string) {
	store, err := openStore()
	if err != nil {
		return
	}
	_ = newAPIClient(store.Settings()).post("/runner/"+domain+"/stop", nil)
}
