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

			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain == fullDomain {
						c.Projects[i].Port = port
						c.Projects[i].Wildcard = wildcard || c.Projects[i].Wildcard
						c.Projects[i].HTTPS = !noHTTPS || c.Projects[i].HTTPS
						c.Projects[i].ForceHTTPS = forceHTTPS
						c.Projects[i].Aliases = mergeUnique(c.Projects[i].Aliases, normAliases)
						return
					}
				}
				c.Projects = append(c.Projects, config.Project{
					Domain:     fullDomain,
					Port:       port,
					Wildcard:   wildcard,
					HTTPS:      !noHTTPS,
					ForceHTTPS: forceHTTPS,
					Aliases:    normAliases,
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

func mergeUnique(existing, add []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(add))
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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
