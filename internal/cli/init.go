package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a .dnser.yaml manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("init: implemented in config core milestone")
		},
	}
	return cmd
}
