package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
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
			projects := s.Projects()
			if len(projects) == 0 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "No linked projects yet. Run:")
				fmt.Fprintln(out, "  dnser link --domain=myproject --port=3000")
				return nil
			}
			fmt.Fprintln(out)
			for _, p := range projects {
				flags := []string{}
				if p.Wildcard {
					flags = append(flags, "*")
				}
				if p.HTTPS {
					flags = append(flags, "https")
				}
				port := "-"
				if p.Port > 0 {
					port = fmt.Sprintf("%d", p.Port)
				}
				fmt.Fprintf(out, "%-40s -> localhost:%-6s %s\n", p.Domain, port, strings.Join(flags, ","))
				if verbose {
					for _, a := range p.Aliases {
						fmt.Fprintf(out, "  alias %s\n", a)
					}
					for _, r := range p.Records {
						fmt.Fprintf(out, "  %-5s %-16s %s\n", r.Type, r.Name, r.Value)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show aliases and records")
	return cmd
}
