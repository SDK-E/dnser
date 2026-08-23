package cli

import (
	"fmt"
	"runtime"

	"github.com/SDK-E/dnser/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for a newer DNSer release",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "current version: %s\n", version)
			rel, err := update.Check(cmd.Context(), version)
			if err != nil {
				return fmt.Errorf("check latest release: %w", err)
			}
			if rel == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "up to date")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "new version:     %s\n\n", rel.Version)
			fmt.Fprintln(cmd.OutOrStdout(), "Install the update for your platform:")
			switch runtime.GOOS {
			case "darwin":
				fmt.Fprintf(cmd.OutOrStdout(), "  brew upgrade sdk-e/tap/dnser   (if installed via Homebrew)\n")
				fmt.Fprintf(cmd.OutOrStdout(), "  or grab DNSer_%s_macOS_%s.dmg from:\n", rel.Version, darwinArch())
			case "windows":
				fmt.Fprintf(cmd.OutOrStdout(), "  run the DNSer_%s_windows_amd64_setup.exe from:\n", rel.Version)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "  replace your dnser-desktop / dnser package, or download the AppImage:\n")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", rel.URL)
			return nil
		},
	}
	return cmd
}

func darwinArch() string {
	if runtime.GOARCH == "amd64" {
		return "amd64"
	}
	return "arm64"
}
