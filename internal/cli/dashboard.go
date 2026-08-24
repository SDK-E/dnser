package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/SDK-E/dnser/internal/dashboard"
	"github.com/spf13/cobra"
)

func NewDashboardCommand() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "dashboard [--port P]",
		Short: "Serve the local web dashboard (loopback-only, token-gated)",
		Long: `Serves the embedded dashboard on 127.0.0.1 only. A random token is
stored at ~/.dnser/dashboard-token (0600) and printed as a ready-to-open URL.
Requests from non-loopback sources are refused regardless of token.

When not to use: do not tunnel this port through SSH or expose it; the
token is meant for the console user's browser.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			st := mustState()
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			superReachable := fileExists(supervisorSocketPath())
			deps := dashboard.Deps{
				State: st,
				ListIssues: func(ctx context.Context) []dashboard.Issue {
					var out []dashboard.Issue
					for _, i := range collectDoctorIssues(ctx, st, superReachable, false) {
						out = append(out, dashboard.Issue{Kind: i.Kind, Evidence: i.Evidence, Fix: i.Fix})
					}
					return out
				},
				DNSPort:       func() int { return dnsPortFor(st) },
				SupervisorRun: superReachable,
				LogLines: func(ctx context.Context, project string, tail int) ([]string, error) {
					return readLogTail(project, tail)
				},
			}
			tok, terr := dashboard.EnsureToken()
			if terr != nil {
				return terr
			}
			fmt.Fprintf(o.Stderr, "dashboard: http://%s/?token=%s\n", addr, tok)
			return dashboard.Serve(cmd.Context(), addr, deps)
		},
	}
	cmd.Flags().IntVar(&port, "port", 7780, "loopback port for the dashboard")
	return cmd
}

func readLogTail(project string, tail int) ([]string, error) {
	dot, err := homeDot()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dot, "logs", project+".log")
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return nil, fmt.Errorf("no log yet for %s", project)
	}
	lines := splitLines(string(data))
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}
