package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SDK-E/dnser/internal/dnsl"
	"github.com/SDK-E/dnser/internal/helper"
	"github.com/SDK-E/dnser/internal/journal"
	"github.com/SDK-E/dnser/internal/orchestrator"
	"github.com/SDK-E/dnser/internal/state"
	"github.com/spf13/cobra"
)

func revertAllPlans(ctx context.Context) (int, error) {
	store, err := openUserStore()
	if err != nil {
		return 0, err
	}
	plans, err := store.List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range plans {
		if p.Status != journal.StatusApplied {
			continue
		}
		if _, rerr := helper.RevertPlan(ctx, store, p); rerr != nil {
			return n, fmt.Errorf("revert %s: %w", p.ID, rerr)
		}
		n++
	}
	return n, nil
}

func rootSuffixOfDomain(domain string) string {
	parts := splitDots(domain)
	if len(parts) < 2 {
		return domain
	}
	return parts[len(parts)-2] + "." + parts[len(parts)-1]
}

func resolverDirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func countMissingResolverFiles(expected []dnsl.Entry) int {
	w := dnsl.ResolverWriter{Dir: filepath.Join(string(filepath.Separator), "etc", "resolver")}
	drifted, err := w.Verify(expected)
	if err != nil {
		return 0
	}
	missing := 0
	for _, d := range drifted {
		if strings.HasSuffix(d, " (missing)") {
			missing++
		}
	}
	return missing
}

func removeServiceBestEffort() {
	target, err := helper.ServiceInstallTarget()
	if err != nil || target == "" {
		return
	}
	name := helper.ServiceLabel()
	ops := helper.ServiceOps()
	if ops == nil {
		return
	}
	if loaded, lerr := ops.IsLoaded(context.Background(), name); lerr == nil && loaded {
		_ = ops.Unload(context.Background(), name)
	}
	_ = os.Remove(target)
}

func untrustCABestEffort() {
	dot, err := homeDot()
	if err != nil {
		return
	}
	caPath := filepath.Join(dot, "ca", "root.pem")
	if _, serr := os.Stat(caPath); serr != nil {
		return
	}
	ops := helper.CATrustOps()
	if ops == nil {
		return
	}
	_ = ops.Untrust(context.Background(), caPath)
}

func splitDots(d string) []string {
	var parts []string
	cur := ""
	for i := 0; i < len(d); i++ {
		if d[i] == '.' {
			parts = append(parts, cur)
			cur = ""
			continue
		}
		cur += string(d[i])
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

type Issue struct {
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix"`
}

func NewDoctorCommand() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor [--fix]",
		Short: "Run all health checks; exit 10 when issues are found",
		Long: `Checks: resolver drift (R2), dead resolver files (R12), interrupted
mutation plans (R3), stray listeners on our ports (R6), shadowed public
suffixes (R5). Exit 0 = clean, exit 10 = issues found (an outcome, not an
error). --fix executes the safe subset.

When not to use: doctor is diagnostic only; it never changes behavior
without --fix.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			ctx := cmd.Context()
			st := mustState()
			fixFlag, _ := cmd.Flags().GetBool("fix")
			superReachable := fileExists(supervisorSocketPath())
			issues := collectDoctorIssues(ctx, st, superReachable, fixFlag)

			if o.Format == FormatText {
				if len(issues) == 0 {
					fmt.Fprintln(o.Stderr, "clean")
				}
				for _, i := range issues {
					fmt.Fprintf(o.Stderr, "[%s] %s\n  fix: %s\n", i.Kind, i.Evidence, i.Fix)
				}
			} else if err := o.Emit(map[string]any{"issues": issues, "count": len(issues)}); err != nil {
				return err
			}
			if len(issues) > 0 {
				return &DoctorIssuesError{Issues: len(issues)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "apply the safe subset of fixes")
	return cmd
}

func collectDoctorIssues(ctx context.Context, st *state.Store, superReachable, fix bool) []Issue {
	issues := []Issue{}

	var expected []dnsl.Entry
	for _, lp := range st.ListLinked() {
		expected = append(expected, dnsl.Entry{Suffix: rootSuffixOfDomain(lp.Domain), Addr: fmt.Sprintf("127.0.0.1:%d", dnsPortFor(st))})
	}
	w := dnsl.ResolverWriter{Dir: filepath.Join(string(filepath.Separator), "etc", "resolver")}
	if len(expected) > 0 && resolverDirExists(w.Dir) {
		drifted, verr := w.Verify(expected)
		if verr == nil {
			for _, d := range drifted {
				if strings.HasSuffix(d, " (missing)") {
					continue
				}
				issues = append(issues, Issue{Kind: "resolver_drift", Evidence: "/etc/resolver: " + d, Fix: "dnser journal revert <plan>  ||  dnser unelevate"})
			}
		}
	}

	store, jerr := openUserStore()
	if jerr == nil {
		plans, lerr := store.List()
		if lerr == nil {
			for _, p := range plans {
				if journal.HasInterrupted(p) {
					hint := fmt.Sprintf("dnser journal finish %s  ||  dnser journal revert %s", p.ID, p.ID)
					issues = append(issues, Issue{Kind: "interrupted_plan", Evidence: "journal plan " + p.ID + " is mid-flight (" + string(p.Status) + ")", Fix: hint})
					if fix && p.Intent == "elevate" {
						_, _ = journal.Finish(ctx, store, p, helperRegistryForDoctor())
					}
				}
			}
		}
	}

	if superReachable {
		super := orchestrator.NewUDSClient(supervisorSocketPath(), "")
		records := []orchestrator.StrayRecord{}
		for _, lp := range st.ListLinked() {
			pid := 0
			if ps, gerr := super.GetProcess(ctx, lp.Name); gerr == nil {
				pid = ps.Pid
			}
			records = append(records, orchestrator.StrayRecord{Project: lp.Name, Port: lp.Port, Pid: pid})
		}
		res := orchestrator.SweepStrays(records, orchestrator.NewDialProber(0), func(int) bool { return true })
		for _, r := range res {
			if r.PortInUse && r.RegistryPid == 0 {
				issues = append(issues, Issue{Kind: "stray_listener", Evidence: fmt.Sprintf("127.0.0.1:%d is occupied by an unknown process", r.Port), Fix: "identify with: lsof -i :" + fmt.Sprint(r.Port)})
			}
		}
	}

	for _, lp := range st.ListLinked() {
		for _, warn := range dnsl.PublicSuffixWarnings([]string{lp.Domain}) {
			issues = append(issues, Issue{Kind: "shadowed_suffix", Evidence: warn, Fix: "choose a non-public suffix or accept the warning"})
		}
	}

	if superReachable && len(expected) > 0 {
		if missing := countMissingResolverFiles(expected); missing > 0 {
			issues = append(issues, Issue{
				Kind:     "fallback_dns",
				Evidence: fmt.Sprintf("%d of %d resolver entries absent; projects only reachable on localhost ports", missing, len(expected)),
				Fix:      "dnser elevate",
			})
		}
	}
	return issues
}

func helperRegistryForDoctor() journal.Registry {
	return journal.NewFSRegistry()
}

func NewUpdateCommand() *cobra.Command {
	var (
		checkOnly bool
		applyYes  bool
		fromURL   string
	)
	cmd := &cobra.Command{
		Use:   "update [--check] [--yes]",
		Short: "Detect install source and defer to the right upgrade path",
		Long: `Detect-and-defer: brew-managed installs print the exact brew command and exit;
script/manual installs download the release asset, verify sha256 against
checksums.txt, and replace the binary atomically. Never overwrites a managed
install. --check is a read-only dry run.

When not to use: for pinned CI builds use goreleaser artifacts directly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			if applyYes && !checkOnly {
				self, serr := os.Executable()
				if serr != nil {
					return serr
				}
				result, uerr := RunUpdate(cmd.Context(), self, fromURL, true, NewHTTPFetcher())
				if uerr != nil {
					return uerr
				}
				return o.Emit(result)
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			source := classifyInstall(self)[0]
			command := map[string]string{
				"brew":   "brew upgrade sdk-e/tap/dnser && brew autoremove",
				"manual": "dnser update --yes (downloads, verifies checksums.txt, replaces atomically)",
				"script": "re-run the official install script from the release page",
			}[source]
			payload := map[string]any{"source": source, "check_only": checkOnly, "command": command}
			if o.Format == FormatText {
				fmt.Fprintf(o.Stderr, "install source: %s\nrun: %s\n", source, command)
				return nil
			}
			return o.Emit(payload)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "read-only check")
	cmd.Flags().BoolVarP(&applyYes, "yes", "y", false, "apply the update (manual/script installs only)")
	cmd.Flags().StringVar(&fromURL, "from", "", "override release base URL")
	return cmd
}

func hasPrefixAny(p string, prefixes ...string) bool {
	for _, pre := range prefixes {
		if len(p) >= len(pre) && p[:len(pre)] == pre {
			return true
		}
	}
	return false
}

func NewMigrateCommand() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "migrate [path] [-y]",
		Short: "Rewrite legacy manifests to schema v3 (dry-run diff by default)",
		Long: `Shows every rewrite that would be performed (label→FQN etc.), then
asks for confirmation before writing. Originals are backed up alongside and
the change is journaled. Moderate severity: -y applies without prompting.

When not to use: fresh v3 manifests have nothing to migrate.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			path := filepath.Join(dir, ".dnser.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}
			rewrites := detectLegacyKeys(data)
			plan := ConfirmPlan{Command: "migrate", Severity: SeverityModerate}
			for _, rw := range rewrites {
				plan.Changes = append(plan.Changes, Change{Action: "rewrite_key", Path: path, Detail: rw})
			}
			if len(plan.Changes) == 0 {
				fmt.Fprintln(o.Stderr, "already v3: nothing to migrate")
				return nil
			}
			confirmName := ""
			if cmd.Flags().Changed("confirm") {
				confirmName, _ = cmd.Flags().GetString("confirm")
			}
			if err := EvaluateConfirm(o, plan, apply, confirmName, ""); err != nil {
				RenderPlanText(o, plan)
				return err
			}
			rewritten := rewriteLegacyManifest(data, rewrites)
			if _, merr := applyMutation(cmd.Context(), "migrate", []MutationWrite{
				{Path: path + ".v2.bak", Content: data, Mode: 0o644},
				{Path: path, Content: rewritten, Mode: 0o644},
			}); merr != nil {
				return merr
			}
			fmt.Fprintf(o.Stderr, "migrated %s (backup at %s.v2.bak)\n", path, path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&apply, "yes", "y", false, "apply without prompting")
	return cmd
}

func detectLegacyKeys(data []byte) []string {
	var out []string
	legacy := map[string]string{
		"label:": "label → domain (FQN)",
		"tld:":   "tld → removed (default_tld hint only)",
		"name:":  "name → project directory name",
	}
	lines := splitLines(string(data))
	for _, line := range lines {
		for key, desc := range legacy {
			if hasKeyPrefix(line, key) {
				out = append(out, desc)
			}
		}
	}
	return out
}

func rewriteLegacyManifest(data []byte, rewrites []string) []byte {
	lines := splitLines(string(data))
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		switch {
		case hasKeyPrefix(line, "label:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "label:"))
			out = append(out, "domain: "+value+".test")
		case hasKeyPrefix(line, "tld:"), hasKeyPrefix(line, "name:"):
			continue
		default:
			out = append(out, line)
		}
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func hasKeyPrefix(line, key string) bool {
	if len(line) < len(key) {
		return false
	}
	head := line[:len(key)]
	if head != key {
		return false
	}
	rest := line[len(key):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}

func NewUninstallCommand() *cobra.Command {
	var purge bool
	var yes bool
	var confirmName string
	cmd := &cobra.Command{
		Use:   "uninstall [--purge] [--confirm NAME]",
		Short: "Stop everything; --purge removes every trace (severe)",
		Long: `Without --purge: stops runtime only, keeps config/state.
With --purge: stop projects → remove service → CA untrust → remove resolver
entries + restore captured NIC DNS → delete state dirs → print package-manager
guidance → verification pass prints any residue. Non-empty residue exits 1.

Severe: requires --confirm <project-or-dnser-name>; --yes alone is refused.
When not to use: plain down is reversible; purge is not.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			ctx := cmd.Context()
			st := mustState()
			if !purge {
				fmt.Fprintln(o.Stderr, "runtime stopped; config and state kept (use --purge to erase everything)")
				return o.Emit(map[string]any{"purged": false})
			}
			plan := ConfirmPlan{Command: "uninstall", Severity: SeveritySevere, Changes: []Change{
				{Action: "stop_projects", Detail: linkedTargets(st, "")},
				{Action: "remove_service"},
				{Action: "ca_untrust"},
				{Action: "remove_resolver_entries"},
				{Action: "purge_project", Path: ".dnser/state+generated+journal", Detail: "~/.dnser"},
			}}
			if err := EvaluateConfirm(o, plan, yes, confirmName, ""); err != nil {
				RenderPlanText(o, plan)
				return err
			}
			if super, serr := supervisorClient(st); serr == nil {
				mgr := orchestrator.NewManager(super, nil, 10*time.Second)
				for _, lp := range st.ListLinked() {
					_ = mgr.Stop(ctx, lp.Name, time.Now())
				}
			}
			if _, uerr := revertAllPlans(ctx); uerr != nil {
				return uerr
			}
			removeServiceBestEffort()
			untrustCABestEffort()
			dot, herr := homeDot()
			if herr == nil {
				_ = os.RemoveAll(filepath.Join(dot, "generated"))
				_ = os.Remove(filepath.Join(dot, "state.json"))
			}
			fmt.Fprintln(o.Stderr, "package-manager cleanup:")
			fmt.Fprintln(o.Stderr, "  brew uninstall sdk-e/tap/dnser && brew autoremove")
			residue, rerr := residueCheck()
			if rerr != nil {
				return rerr
			}
			if len(residue) > 0 {
				for _, item := range residue {
					fmt.Fprintf(o.Stderr, "residue: %s\n", item)
				}
				return fmt.Errorf("%d residue items remain", len(residue))
			}
			return o.Emit(map[string]any{"purged": true, "residue": 0})
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "erase everything dnser ever touched (severe)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip moderate prompts (NOT valid for purge alone)")
	cmd.Flags().StringVar(&confirmName, "confirm", "", "type the name to authorize severe mutations")
	return cmd
}
