package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "dnser %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
			fmt.Fprintln(out, "DNSer — SDK Enterprises")
			return nil
		},
	}
}
