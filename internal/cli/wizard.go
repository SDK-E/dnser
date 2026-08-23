package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/detect"
	"github.com/SDK-E/dnser/internal/setup"
)

func runWizard(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	store, err := openStore()
	if err != nil {
		return err
	}
	dir, err := config.DefaultDir()
	if err != nil {
		return err
	}
	state, _ := setup.LoadState(dir)
	st := store.Settings()

	fmt.Fprintf(out, "\n  DNS.er — local DNS for developers\n\n")

	if !state.DNSApplied && !state.CATrusted {
		fmt.Fprintln(out, "  Welcome! First-time setup takes under a minute. It will:")
		fmt.Fprintf(out, "   1. Point your system DNS at %s (revert anytime with dnser unsetup)\n", st.Bind)
		fmt.Fprintln(out, "   2. Trust a local certificate authority for https:// domains")
		fmt.Fprintln(out, "   3. Offer to start DNSer automatically at login")
		fmt.Fprintln(out)
		answer := prompt(out, "  Run guided setup now? [Y/n] ")
		if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
			setupCmd := newSetupCmd()
			setupCmd.SetOut(out)
			return setupCmd.RunE(cmd, nil)
		}
		fmt.Fprintln(out, "\n  Skipped. Run it anytime with: dnser setup")
	}

	projects := store.Projects()
	if len(projects) == 0 {
		res, derr := detect.DetectPort("")
		fmt.Fprintln(out)
		if derr == nil && res.Port > 0 {
			fmt.Fprintf(out, "  Detected %s project here on port %d.\n", res.Framework, res.Port)
			fmt.Fprintf(out, "  Link as myproject.%s? [Y/n] ", st.TLD)
			answer := prompt(out, "")
			if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
				linkCmd := newLinkCmd()
				linkCmd.SetOut(out)
				_ = linkCmd.Flags().Set("domain", "myproject")
				_ = linkCmd.Flags().Set("port", fmt.Sprint(res.Port))
				_ = linkCmd.Flags().Set("wildcard", "true")
				return linkCmd.RunE(cmd, nil)
			}
		}
		fmt.Fprintln(out, "\n  Link a project when you're ready:")
		fmt.Fprintf(out, "    dnser link --domain=myproject --port=3000 --wildcard\n")
	} else {
		fmt.Fprintln(out, "\n  Linked projects:")
		for _, p := range projects {
			extra := ""
			if p.Wildcard {
				extra = "  (*)"
			}
			fmt.Fprintf(out, "    %-38s → :%d%s\n", p.Domain, p.Port, extra)
		}
	}

	fmt.Fprintf(out, "\n  Dashboard  http://%s:%d\n", st.Bind, st.Ports.UI)
	fmt.Fprintln(out, "  Commands   dnser status | logs -f | open | stop")
	fmt.Fprintln(out)
	return nil
}
