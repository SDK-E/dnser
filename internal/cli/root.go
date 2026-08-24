package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

type globals struct {
	outputFormat string
	fields       string
	yes          bool
	noInput      bool
}

func NewRootCommand() *cobra.Command {
	g := &globals{}
	cmd := &cobra.Command{
		Use:           "dnser",
		Short:         "Local domains, TLS and dev-server orchestration",
		Long:          "dnser orchestrates local development infrastructure: project manifests, DNS, TLS and supervised dev servers.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.DisableSuggestions = true
	cmd.SuggestionsMinimumDistance = 0
	pf := cmd.PersistentFlags()
	pf.StringVarP(&g.outputFormat, "output", "o", "", "output format: text|json|ndjson (default: text on TTY, json when piped)")
	pf.StringVar(&g.fields, "fields", "", "comma-separated top-level JSON fields to keep")
	pf.BoolVarP(&g.yes, "yes", "y", false, "skip moderate-severity confirmations")
	pf.BoolVar(&g.noInput, "no-input", false, "fail fast instead of prompting (agent-safe)")

	wrapper := func(runE func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			o := NewOutputWriter(cmd.OutOrStdout(), cmd.ErrOrStderr(), g.outputFormat, g.fields)
			ctx := context.WithValue(cmd.Context(), outputKey{}, o)
			cmd.SetContext(ctx)
			lock, lerr := AcquireLock()
			if lerr != nil {
				return emitError(o, lerr)
			}
			defer lock.Release()
			if err := runE(cmd, args); err != nil {
				return emitError(o, err)
			}
			return nil
		}
	}
	for _, c := range []*cobra.Command{
		NewInitCommand(), NewSchemaCommand(), NewElevateCommand(), NewUnelevateCommand(),
		NewJournalCommand(), NewHelperCommand(), NewLinkCommand(), NewUnlinkCommand(),
		NewUpCommand(), NewDownCommand(),
		lifecycleControlCmd("start", 0),
		lifecycleControlCmd("stop", 1),
		lifecycleControlCmd("restart", 2),
		NewStatusCommand(), NewLogsCommand(), NewExplainCommand(),
		NewDoctorCommand(), NewUpdateCommand(), NewMigrateCommand(), NewUninstallCommand(),
	} {
		if c.RunE != nil {
			inner := c.RunE
			c.RunE = wrapper(inner)
		} else {
			for _, sub := range c.Commands() {
				if sub.RunE != nil {
					inner := sub.RunE
					sub.RunE = wrapper(inner)
				}
			}
		}
		cmd.AddCommand(c)
	}
	return cmd
}

var exitFn = os.Exit

func emitError(o *Output, err error) error {
	env, code := ErrorEnvelope(err)
	if o.Format == FormatText {
		fmt.Fprintf(o.Stderr, "%s\n", env.ErrorText)
		if env.Remediation != "" {
			fmt.Fprintf(o.Stderr, "→ %s\n", env.Remediation)
		}
	} else {
		b, _ := json.Marshal(env)
		fmt.Fprintln(o.Stderr, string(b))
	}
	exitFn(code)
	return nil
}

func Execute(ctx context.Context, args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	return fang.Execute(ctx, root)
}

func ExitCode(err error) int {
	_, code := ErrorEnvelope(err)
	return code
}

var _ = errors.Is
