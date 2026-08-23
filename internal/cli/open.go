package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the DNSer dashboard in your browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			st := store.Settings()
			url := fmt.Sprintf("http://%s:%d", st.Bind, st.Ports.UI)
			if _, err := openBrowser(url); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Dashboard: %s\n", url)
				fmt.Fprintf(cmd.OutOrStdout(), "(also reachable at https://%s once trusted)\n", config.DashboardDomain(st.TLD))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ opened %s\n", url)
			return nil
		},
	}
}

func openBrowser(url string) (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return true, exec.Command("open", url).Start()
	case "windows":
		return true, exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return true, exec.Command("xdg-open", url).Start()
	}
}
