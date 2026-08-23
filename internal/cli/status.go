package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/dnscore"
)

func newStatusCmd() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show configured projects and settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			st := s.Settings()
			fmt.Fprintf(out, "Config     %s\n", s.Path())
			fmt.Fprintf(out, "TLD        .%s\n", st.TLD)
			fmt.Fprintf(out, "Bind       %s\n", st.Bind)
			fmt.Fprintf(out, "Ports      dns=%d http=%d https=%d ui=%d\n",
				st.Ports.DNS, st.Ports.HTTP, st.Ports.HTTPS, st.Ports.UI)
			fmt.Fprintf(out, "Upstreams  %s\n", strings.Join(st.Upstreams, ", "))
			dnsProbe := dnscore.ProbeLocal(st.Bind, 53, config.DashboardDomain(st.TLD))
			if dnsProbe == nil {
				fmt.Fprintln(out, "DNS :53     ✓ answering")
			} else if alt := dnscore.ProbeLocal(st.Bind, 35353, config.DashboardDomain(st.TLD)); alt == nil {
				fmt.Fprintln(out, "DNS :53     ✗ nothing answering — dnser is on :35353 (run: sudo dnser start --bind-port 53)")
			} else {
				fmt.Fprintln(out, "DNS :53     ✗ nothing answering (is the daemon running? dnser start)")
			}
			fmt.Fprintln(out)

			projects := s.Projects()
			if len(projects) == 0 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "No linked projects yet. Run:")
				fmt.Fprintln(out, "  dnser link --domain=myproject --port=3000")
				return nil
			}
			fmt.Fprintln(out)
			for _, p := range projects {
				fmt.Fprintf(out, "%-40s %s\n", p.Domain, describeRoutes(p))
				if verbose {
					for _, route := range p.Routes {
						kind := "http"
						if route.TCP {
							kind = fmt.Sprintf("tcp:%d", route.Listen)
						}
						extra := ""
						if route.ForceHTTPS {
							extra = " force-https"
						} else if route.HTTPS {
							extra = " https"
						}
						fmt.Fprintf(out, "  %-24s %-8s -> %s%s\n", route.Hostname(p.Domain, ""), kind, strings.Join(route.Backends, ", "), extra)
					}
					for _, r := range p.Records {
						fmt.Fprintf(out, "  %-5s %-16s %s\n", r.Type, r.Name, r.Value)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show routes and records")
	return cmd
}

func describeRoutes(p config.Project) string {
	parts := []string{}
	for _, route := range p.Routes {
		kind := ""
		if route.TCP {
			kind = fmt.Sprintf("tcp:%d ", route.Listen)
		} else if route.ForceHTTPS {
			kind = "https "
		} else if route.HTTPS {
			kind = "tls "
		}
		host := route.Hostname(p.Domain, "")
		if route.Host == "@" {
			host = p.Domain
		}
		parts = append(parts, fmt.Sprintf("%s%s → %s", kind, host, strings.Join(route.Backends, ",")))
	}
	return strings.Join(parts, "\n                 ")
}
