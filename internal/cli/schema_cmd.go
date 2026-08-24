package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/api"
)

func newSchemaCmd() *cobra.Command {
	var project bool
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print JSON Schema for dnser.json (or .dnser.yaml with --project)",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := api.ConfigSchemaFile
			if project {
				name = api.ProjectSchemaFile
			}
			data, err := api.SchemaFile(name)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().BoolVar(&project, "project", false, "print the .dnser.yaml project manifest schema instead")
	return cmd
}
