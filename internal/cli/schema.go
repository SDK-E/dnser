package cli

import (
	"fmt"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/spf13/cobra"
)

func NewSchemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Emit the JSON Schema for .dnser.yaml",
		Long:  "Emits the generated JSON Schema (draft 2020-12) for the manifest. CI diffs this output against the committed schema to catch breaking changes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := config.GenerateManifestSchema()
			if err != nil {
				return fmt.Errorf("generate schema: %w", err)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
}
