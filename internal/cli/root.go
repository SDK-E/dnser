package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

var (
	configPath string
	homeDir    string
	verbose    bool
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "dnser",
		Short:         "Local DNS management for development",
		Long:          "DNSer runs a local DNS server, HTTPS reverse proxy and web dashboard for your development projects.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runWizard(cmd)
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to dnser.json (default ~/.dnser/dnser.json)")
	root.PersistentFlags().StringVar(&homeDir, "home", "", "dnser home directory (default ~/.dnser; used by the privileged daemon)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "verbose output")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newUnsetupCmd())
	root.AddCommand(newStartCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newRestartCmd())
	root.AddCommand(newLinkCmd())
	root.AddCommand(newUnlinkCmd())
	root.AddCommand(newAddRecordCmd())
	root.AddCommand(newRemoveRecordCmd())
	root.AddCommand(newListRecordsCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(newUpdateCmd())
	return root
}

func Execute() int {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func resolveConfigPath() (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	if homeDir != "" {
		return filepath.Join(homeDir, "dnser.json"), nil
	}
	return config.DefaultPath()
}

func openStore() (*config.Store, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	s, err := config.Open(path)
	if err != nil {
		return nil, err
	}
	return s, nil
}
