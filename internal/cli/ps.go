package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "Show managed dev servers and their state",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			var payload struct {
				Apps []struct {
					Domain    string `json:"domain"`
					Framework string `json:"framework"`
					State     string `json:"state"`
					Port      int    `json:"port"`
					PID       int    `json:"pid"`
					Restarts  int    `json:"restarts"`
					LastError string `json:"last_error"`
				} `json:"apps"`
				DepsMissing map[string]string `json:"deps_missing"`
			}
			if err := newAPIClient(store.Settings()).get("/runner", &payload); err != nil {
				return err
			}
			if len(payload.Apps) == 0 && len(payload.DepsMissing) == 0 {
				fmt.Fprintln(out, "no managed projects — link one with `dnser link <path>`")
				return nil
			}
			tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "DOMAIN\tSTATE\tPORT\tPID\tRESTARTS\tSTACK")
			for _, app := range payload.Apps {
				pid := "-"
				if app.PID > 0 {
					pid = fmt.Sprint(app.PID)
				}
				stack := app.Framework
				if stack == "" {
					stack = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%d\t%s\n", app.Domain, app.State, app.Port, pid, app.Restarts, stack)
				if app.LastError != "" {
					fmt.Fprintf(tw, "  └ %s\n", app.LastError)
				}
			}
			_ = tw.Flush()
			for domain, hint := range payload.DepsMissing {
				fmt.Fprintf(out, "! %s: %s\n", domain, hint)
			}
			return nil
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common setup problems",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			var payload struct {
				Status string `json:"status"`
				Checks []struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					Detail string `json:"detail"`
				} `json:"checks"`
			}
			if err := newAPIClient(store.Settings()).get("/doctor", &payload); err != nil {
				return err
			}
			mark := map[string]string{"ok": "✓", "warn": "!", "fail": "✗"}
			for _, c := range payload.Checks {
				sym := mark[c.Status]
				if sym == "" {
					sym = "?"
				}
				fmt.Fprintf(out, "%s %-12s %s\n", sym, c.Name, c.Detail)
			}
			switch payload.Status {
			case "ok":
				fmt.Fprintln(out, "\nall checks passed")
			case "warn":
				fmt.Fprintln(out, "\nissues found (see ! above)")
			default:
				fmt.Fprintln(out, "\nproblems found (see ✗ above)")
			}
			return nil
		},
	}
}
