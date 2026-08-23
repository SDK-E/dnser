package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/detect"
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
	)
	cmd := &cobra.Command{
		Use:   "link --domain=myproject",
		Short: "Link a project directory/domain to a local port",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			settings := store.Settings()
			useTLD := settings.TLD
			if tld != "" {
				useTLD = tld
			}
			if domain == "" && len(args) > 0 {
				domain = args[0]
			}
			if domain == "" {
				return fmt.Errorf("specify --domain (e.g. --domain=myproject)")
			}
			fullDomain, err := config.EnsureTLD(domain, useTLD)
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}

			framework := ""
			if port == 0 {
				res, err := detect.DetectPort("")
				if err == nil && res.Port > 0 {
					port = res.Port
					framework = res.Framework
				}
			}
			if port == 0 {
				port = 3000
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

			backend := fmt.Sprintf("localhost:%d", port)
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != fullDomain {
						continue
					}
					p := &c.Projects[i]
					patched := false
					for j := range p.Routes {
						switch p.Routes[j].Host {
						case "@":
							if len(p.Routes[j].Backends) > 0 {
								p.Routes[j].Backends[0] = backend
								p.Routes[j].HTTPS = !noHTTPS
								p.Routes[j].ForceHTTPS = forceHTTPS
								patched = true
							}
						case "*":
							if len(p.Routes[j].Backends) > 0 {
								p.Routes[j].Backends[0] = backend
							}
						}
					}
					if !patched {
						p.Routes = append(p.Routes, config.Route{
							Host:       "@",
							Backends:   []string{backend},
							HTTPS:      !noHTTPS,
							ForceHTTPS: forceHTTPS,
						})
					}
					if wildcard && !hasWildcardRoute(p.Routes) {
						p.Routes = append(p.Routes, config.Route{Host: "*", Backends: []string{backend}, HTTPS: !noHTTPS})
					}
					for _, a := range normAliases {
						if !hasRouteFor(p.Routes, a) {
							p.Routes = append(p.Routes, config.Route{Host: a, Backends: []string{backend}, HTTPS: !noHTTPS})
						}
					}
					return
				}
				routes := []config.Route{{
					Host:       "@",
					Backends:   []string{backend},
					HTTPS:      !noHTTPS,
					ForceHTTPS: forceHTTPS,
				}}
				if wildcard {
					routes = append(routes, config.Route{Host: "*", Backends: []string{backend}, HTTPS: !noHTTPS})
				}
				for _, a := range normAliases {
					routes = append(routes, config.Route{Host: a, Backends: []string{backend}, HTTPS: !noHTTPS})
				}
				c.Projects = append(c.Projects, config.Project{
					Domain: fullDomain,
					Routes: routes,
				})
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "✓ %s → localhost:%d\n", fullDomain, port)
			if framework != "" {
				fmt.Fprintf(out, "  detected %s\n", framework)
			}
			if wildcard {
				fmt.Fprintf(out, "  wildcard *.%s enabled\n", fullDomain)
			} else if !containsFlag(cmd, "wildcard") {
				fmt.Fprintf(out, "  tip: add --wildcard so every subdomain resolves\n")
			}
			if !noHTTPS {
				fmt.Fprintf(out, "  https:// ready once daemon is running\n")
			}
			fmt.Fprintf(out, "  dashboard: dnser open\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain name for the project")
	cmd.Flags().IntVar(&port, "port", 0, "local port to proxy to (auto-detected when omitted)")
	cmd.Flags().StringVar(&tld, "tld", "", "override TLD for this link")
	cmd.Flags().BoolVar(&wildcard, "wildcard", false, "resolve all subdomains (*.domain)")
	cmd.Flags().BoolVar(&noHTTPS, "no-https", false, "disable TLS for this project")
	cmd.Flags().BoolVar(&forceHTTPS, "force-https", false, "redirect plain HTTP to HTTPS")
	cmd.Flags().StringSliceVar(&aliases, "alias", nil, "additional hostname(s) for this project")
	return cmd
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
	var domain string
	cmd := &cobra.Command{
		Use:   "unlink --domain=myproject.test",
		Short: "Remove a linked project and its records",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			target := domain
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			if target == "" {
				return fmt.Errorf("specify --domain")
			}
			norm, err := config.NormalizeDomain(target)
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			var removed bool
			err = store.Update(func(c *config.Config) {
				kept := c.Projects[:0]
				for _, p := range c.Projects {
					if p.Domain == norm {
						removed = true
						continue
					}
					kept = append(kept, p)
				}
				c.Projects = kept
			})
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("project %q is not linked", norm)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ unlinked %s\n", norm)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "linked domain to remove")
	return cmd
}
