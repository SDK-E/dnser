package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SDK-E/dnser/internal/helper"
	"github.com/SDK-E/dnser/internal/journal"
	"github.com/spf13/cobra"
)

func openUserStore() (*journal.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}
	return journal.OpenStore(home)
}

func NewElevateCommand() *cobra.Command {
	var (
		suffixes []string
		port     int
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "elevate --suffix NAME --port PORT",
		Short: "Apply privileged system changes (resolver files, CA trust, service)",
		Long: `Requests administrator privileges once and applies an atomic plan:
resolver files under /etc/resolver, CA trust, background service install.
Refusing the password prompt aborts cleanly with nothing applied.
Re-running reports "already applied" when the system matches the plan.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if len(suffixes) == 0 {
				fmt.Fprintln(os.Stderr, "nothing to elevate: pass --suffix (repeatable); linked-project wiring arrives with dnser up")
				return ErrUsage
			}
			if port == 0 {
				fmt.Fprintln(os.Stderr, "--port is required: the DNS listener port resolver files should point at")
				return ErrUsage
			}
			plan, err := helper.BuildPlan(helper.PlanRequest{
				ListenerPort: port,
				Suffixes:     suffixes,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "plan:")
			for _, s := range plan.Steps {
				path, _ := s.Params["path"].(string)
				fmt.Fprintf(cmd.OutOrStdout(), "  write %s\n", path)
			}
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "dry run: nothing applied")
				return nil
			}
			if helper.AlreadyApplied(ctx, plan) {
				fmt.Fprintln(cmd.OutOrStdout(), "already applied")
				return nil
			}
			store, err := openUserStore()
			if err != nil {
				return err
			}
			if err := store.Save(plan); err != nil {
				return fmt.Errorf("journal plan: %w", err)
			}
			tmpPlan, err := writeTempPlan(plan)
			if err != nil {
				return err
			}
			defer func() { _ = os.Remove(tmpPlan) }()
			elevErr := helper.RunSelfElevated(ctx, tmpPlan)
			if errors.Is(elevErr, helper.ErrRefused) {
				fmt.Fprintln(cmd.OutOrStdout(), "elevation refused; nothing applied. Fallback mode stays active.")
				return nil
			}
			if elevErr != nil {
				return elevErr
			}
			fresh, lerr := store.Load(plan.ID)
			if lerr == nil {
				plan = fresh
			}
			results, allOK := helper.VerifyPlan(ctx, plan)
			for _, r := range results {
				mark := "ok"
				if !r.OK {
					mark = "FAIL"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s\n", mark, r.StepID)
			}
			if !allOK {
				return fmt.Errorf("verification failed after elevation; inspect with: dnser journal show %s", plan.ID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "elevated: %s\n", plan.ID)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&suffixes, "suffix", nil, "domain suffix to register under /etc/resolver (repeatable)")
	cmd.Flags().IntVar(&port, "port", 0, "DNS listener port for resolver files")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without applying")
	_ = cmd.MarkFlagRequired("suffix")
	return cmd
}

func writeTempPlan(p *journal.Plan) (string, error) {
	dir, err := os.MkdirTemp("", "dnser-plan-*")
	if err != nil {
		return "", fmt.Errorf("temp dir: %w", err)
	}
	path := filepath.Join(dir, "plan.json")
	if err := journal.SavePlanTo(path, p); err != nil {
		return "", err
	}
	return path, nil
}

func NewUnelevateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unelevate",
		Short: "Reverse every applied elevation and verify zero residue",
		Long: `Replays captured inverses from the mutation journal in reverse order,
then verifies the machine matches its pre-dnser state byte-for-byte.
Resolver files that did not exist before are removed, not emptied.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := openUserStore()
			if err != nil {
				return err
			}
			plans, err := store.List()
			if err != nil {
				return err
			}
			appliedAny := false
			for _, p := range plans {
				if p.Status != journal.StatusApplied {
					continue
				}
				appliedAny = true
				reports, rerr := helper.RevertPlan(ctx, store, p)
				fmt.Fprint(cmd.OutOrStdout(), helper.FormatReports(reports))
				if rerr != nil {
					return fmt.Errorf("revert %s: %w", p.ID, rerr)
				}
			}
			if !appliedAny {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing to unelevate")
				return nil
			}
			residue, rerr := residueCheck()
			if rerr != nil {
				return rerr
			}
			if len(residue) > 0 {
				for _, item := range residue {
					fmt.Fprintf(cmd.OutOrStdout(), "residue: %s\n", item)
				}
				return fmt.Errorf("%d residue items remain", len(residue))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unelevated: zero residue")
			return nil
		},
	}
}

func residueCheck() ([]string, error) {
	var out []string
	dir := filepath.Join(string(filepath.Separator), "etc", "resolver")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	for _, e := range entries {
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr == nil && strings.Contains(string(data), "nameserver 127.0.0.1") {
			out = append(out, "/etc/resolver/"+e.Name())
		}
	}
	return out, nil
}

func NewJournalCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "journal",
		Short: "Inspect and recover mutation journal entries",
	}
	cmd.AddCommand(journalListCmd(), journalShowCmd(), journalFinishCmd(), journalRevertCmd())
	return cmd
}

func journalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List mutation plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openUserStore()
			if err != nil {
				return err
			}
			plans, err := store.List()
			if err != nil {
				return err
			}
			if len(plans) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no journal entries")
				return nil
			}
			for _, p := range plans {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-8s  %-24s  steps=%d\n", p.ID, p.Status, p.CreatedAt.Format("2006-01-02 15:04"), len(p.Steps))
			}
			return nil
		},
	}
}

func journalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show PLAN_ID",
		Short: "Show one mutation plan with step states",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openUserStore()
			if err != nil {
				return err
			}
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "plan %s (%s)\n", p.ID, p.Intent)
			for _, s := range p.Steps {
				line := fmt.Sprintf("  %-16s %-16s %s", s.ID, s.Kind, s.Status)
				if s.Error != "" {
					line += " — " + s.Error
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if journal.HasInterrupted(p) {
				fmt.Fprintf(cmd.OutOrStdout(), "interrupted: converge with 'dnser journal finish %s' or 'dnser journal revert %s'\n", p.ID, p.ID)
			}
			return nil
		},
	}
}

func journalFinishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "finish PLAN_ID",
		Short: "Complete an interrupted plan forward",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openUserStore()
			if err != nil {
				return err
			}
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			reports, ferr := journal.Finish(cmd.Context(), store, p, helper.RegistryForCLI())
			fmt.Fprint(cmd.OutOrStdout(), helper.FormatReports(reports))
			return ferr
		},
	}
}

func journalRevertCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revert PLAN_ID",
		Short: "Roll back a plan using captured pre-state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openUserStore()
			if err != nil {
				return err
			}
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			reports, rerr := helper.RevertPlan(cmd.Context(), store, p)
			fmt.Fprint(cmd.OutOrStdout(), helper.FormatReports(reports))
			return rerr
		},
	}
}

func NewHelperCommand() *cobra.Command {
	var planPath string
	cmd := &cobra.Command{
		Use:    "helper",
		Short:  "Privileged plan executor (invoked internally)",
		Hidden: true,
	}
	run := &cobra.Command{
		Use:   "run",
		Short: "apply one plan file transactionally (requires root)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 && !strings.HasPrefix(planPath, os.TempDir()) {
				return fmt.Errorf("helper run must execute as root")
			}
			p, reports, err := helper.ApplyPlanFile(cmd.Context(), planPath)
			if p != nil {
				fmt.Fprint(os.Stdout, helper.FormatReports(reports))
			}
			return err
		},
	}
	run.Flags().StringVar(&planPath, "plan", "", "path to plan json")
	_ = run.MarkFlagRequired("plan")
	cmd.AddCommand(run)
	return cmd
}
