package cli

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Manage routes of a linked project (http, https, tcp, udp)",
	}
	cmd.AddCommand(newRouteAddCmd())
	cmd.AddCommand(newRouteRemoveCmd())
	cmd.AddCommand(newRouteListCmd())
	return cmd
}

func newRouteAddCmd() *cobra.Command {
	var (
		host       string
		backends   []string
		paths      []string
		tcp        bool
		udp        bool
		listen     int
		https      bool
		forceHTTPS bool
	)
	cmd := &cobra.Command{
		Use:   "add <domain> --host <label> --backend <host:port> [...]",
		Short: "Declare a route; tcp/udp routes require --listen",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			if host == "" {
				host = "@"
			}
			if len(backends) == 0 {
				return fmt.Errorf("at least one --backend is required")
			}
			route := config.Route{
				Host:       host,
				Backends:   backends,
				TCP:        tcp,
				UDP:        udp,
				Listen:     listen,
				HTTPS:      https,
				ForceHTTPS: forceHTTPS,
				Paths:      paths,
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != domain {
						continue
					}
					p := &c.Projects[i]
					replaced := false
					resolved := route.Hostname(domain, c.Settings.TLD)
					for j := range p.Routes {
						if p.Routes[j].Hostname(domain, c.Settings.TLD) != resolved || len(route.Paths) == 0 && len(p.Routes[j].Paths) > 0 {
							continue
						}
						if len(route.Paths) > 0 && len(p.Routes[j].Paths) == 0 {
							continue
						}
						p.Routes[j] = route
						replaced = true
						break
					}
					if !replaced {
						p.Routes = append(p.Routes, route)
					}
					return
				}
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ route %s on %s → %s\n", host, domain, strings.Join(backends, ", "))
			if len(paths) > 0 {
				fmt.Fprintf(out, "  paths: %s\n", strings.Join(paths, ", "))
			}
			if route.Forwarded() {
				fmt.Fprintf(out, "  frontend: %s://127.0.0.1:%d\n", map[bool]string{true: "udp", false: "tcp"}[udp], listen)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "@", "route host label (@ = apex, * = wildcard, api = api.<domain>)")
	cmd.Flags().StringSliceVar(&backends, "backend", nil, "backend host:port ({port}/{port:<service>} placeholders supported)")
	cmd.Flags().StringSliceVar(&paths, "path", nil, "HTTP path prefixes for this route (longest prefix wins)")
	cmd.Flags().BoolVar(&tcp, "tcp", false, "raw TCP forwarding")
	cmd.Flags().BoolVar(&udp, "udp", false, "UDP relay")
	cmd.Flags().IntVar(&listen, "listen", 0, "local frontend port for tcp/udp routes")
	cmd.Flags().BoolVar(&https, "https", false, "serve this route over HTTPS too")
	cmd.Flags().BoolVar(&forceHTTPS, "force-https", false, "redirect plain HTTP to HTTPS (requires --https)")
	return cmd
}

func newRouteRemoveCmd() *cobra.Command {
	var host string
	var paths []string
	cmd := &cobra.Command{
		Use:     "remove <domain> --host <label>",
		Aliases: []string{"rm"},
		Short:   "Remove routes by host label",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			if host == "" {
				return fmt.Errorf("--host is required")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			var removed int
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != domain {
						continue
					}
					p := &c.Projects[i]
					kept := p.Routes[:0]
					for _, r := range p.Routes {
						match := r.Host == host || r.Hostname(domain, c.Settings.TLD) == config.ResolveHost(host, domain, c.Settings.TLD)
						if match && len(paths) == 0 {
							removed++
							continue
						}
						if match && len(paths) > 0 {
							normPaths := map[string]bool{}
							for _, raw := range paths {
								if np := config.NormalizePathPrefix(raw); np != "" {
									normPaths[np] = true
								}
							}
							filtered := make([]string, 0, len(r.Paths))
							for _, pref := range r.Paths {
								if !normPaths[pref] {
									filtered = append(filtered, pref)
								}
							}
							if len(filtered) == len(r.Paths) {
								kept = append(kept, r)
								continue
							}
							removed++
							if len(filtered) > 0 {
								r.Paths = filtered
								kept = append(kept, r)
							}
							continue
						}
						kept = append(kept, r)
					}
					p.Routes = kept
					return
				}
			})
			if err != nil {
				return err
			}
			if removed == 0 {
				return fmt.Errorf("no matching routes on %s", domain)
			}
			fmt.Fprintf(out, "✓ removed %d route(s) from %s\n", removed, domain)
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "route host label to remove (@, *, or label)")
	cmd.Flags().StringSliceVar(&paths, "path", nil, "remove only these path prefixes instead of the whole route")
	return cmd
}

func newRouteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list <domain>",
		Aliases: []string{"ls"},
		Short:   "List declared routes",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			settings := store.Settings()
			p, ok := store.FindProject(domain)
			if !ok {
				return fmt.Errorf("project %q is not linked", domain)
			}
			if len(p.Routes) == 0 {
				fmt.Fprintf(out, "no routes declared on %s\n", domain)
				return nil
			}
			tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "HOSTNAME\tFRONTEND\tPATHS\tBACKENDS\tTLS\tFORCE-HTTPS")
			for _, r := range p.Routes {
				frontend := "http/https"
				if r.TCP {
					frontend = "tcp:" + strconv.Itoa(r.Listen)
				} else if r.UDP {
					frontend = "udp:" + strconv.Itoa(r.Listen)
				}
				tls := "-"
				if r.HTTPS {
					tls = "https"
				}
				force := "-"
				if r.EffectiveForceHTTPS(settings.ForceHTTPS) && r.HTTPS {
					force = "redirect"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					r.Hostname(domain, settings.TLD), frontend,
					orDash(strings.Join(r.Paths, ",")),
					strings.Join(r.Backends, ", "), tls, force)
			}
			_ = tw.Flush()
			return nil
		},
	}
}
