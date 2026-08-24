package cli

import (
	"context"
	"errors"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "dnser",
		Short:         "Local domains, TLS and dev-server orchestration",
		Long:          "dnser orchestrates local development infrastructure: project manifests, DNS, TLS and supervised dev servers.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewInitCommand())
	cmd.AddCommand(NewSchemaCommand())
	return cmd
}

func Execute(ctx context.Context, args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	return fang.Execute(ctx, root)
}

func ExitCode(err error) int {
	if errors.Is(err, ErrUsage) {
		return 2
	}
	return 1
}
