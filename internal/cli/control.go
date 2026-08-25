package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/orchestrator"
	"github.com/SDK-E/dnser/internal/state"
	"github.com/spf13/cobra"
)

func supervisorClient(st *state.Store) (*orchestrator.Client, error) {
	sock := supervisorSocketPath()
	if _, err := os.Stat(sock); err != nil {
		return nil, &ElevationRequiredError{Command: "dnser up  (start the dnser service first)"}
	}
	c := orchestrator.NewUDSClient(sock, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Live(ctx); err != nil {
		return nil, fmt.Errorf("supervisor unreachable at %s: %w", sock, err)
	}
	return c, nil
}

func supervisorSocketPath() string {
	dot, err := homeDot()
	if err != nil {
		return filepath.Join(os.TempDir(), "dnser-supervisor.sock")
	}
	return filepath.Join(dot, "supervisor.sock")
}

func NewUpCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "up [--project P]",
		Short: "Ensure infrastructure is current and start projects",
		Long: `Ensures the dnser service and generated configs are current, then
starts the requested project honoring its availability tier. On-request
projects register their route and start on first request instead.

When not to use: if the dnser service is not installed, this exits 4 with
the exact command to run — it never elevates silently.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			ctx := cmd.Context()
			st := mustState()
			super, serr := supervisorClient(st)
			if serr != nil {
				var elev *ElevationRequiredError
				if asElevation(serr, &elev) {
					return serr
				}
				return serr
			}
			targets := linkedTargets(st, project)
			if targets == "" {
				return fmt.Errorf("%w: no linked projects", ErrUsage)
			}
			started := []string{}
			for _, name := range strings.Split(targets, ",") {
				lp, _ := st.Linked(name)
				if lp.Availability == config.AvailabilityOnRequest {
					continue
				}
				mgr := orchestrator.NewManager(super, nil, 30*time.Second)
				if err := mgr.Start(ctx, name, time.Now()); err != nil {
					return err
				}
				started = append(started, name)
			}
			return o.Emit(map[string]any{"started": started, "on_request_only": targets != "" && len(started) == 0})
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "limit to one linked project")
	return cmd
}

func asElevation(err error, target **ElevationRequiredError) bool {
	e, ok := err.(*ElevationRequiredError)
	if ok {
		*target = e
	}
	return ok
}

func linkedTargets(st *state.Store, only string) string {
	names := []string{}
	for _, lp := range st.ListLinked() {
		if only == "" || lp.Name == only {
			names = append(names, lp.Name)
		}
	}
	return strings.Join(names, ",")
}

func NewDownCommand() *cobra.Command {
	var infra bool
	cmd := &cobra.Command{
		Use:   "down [--infra]",
		Short: "Stop supervised projects (infrastructure keeps running)",
		Long: `Stops all supervised projects gracefully. Infrastructure (Caddy,
DNS listener, service definition) keeps running unless --infra is given.

When not to use: --infra also removes /etc/resolver entries; if you only
want a coding break, plain down is enough.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			ctx := cmd.Context()
			st := mustState()
			super, serr := supervisorClient(st)
			if serr != nil {
				return serr
			}
			mgr := orchestrator.NewManager(super, nil, 10*time.Second)
			stopped := []string{}
			for _, lp := range st.ListLinked() {
				if err := mgr.Stop(ctx, lp.Name, time.Now()); err != nil {
					continue
				}
				stopped = append(stopped, lp.Name)
			}
			if infra {
				fmt.Fprintln(o.Stderr, "--infra requested: stopping DNS listener and removing resolver registrations via journal-aware path requires the daemon (M7); resolver files remain safe to remove manually with dnser unelevate")
			}
			return o.Emit(map[string]any{"stopped": stopped})
		},
	}
	cmd.Flags().BoolVar(&infra, "infra", false, "also stop Caddy/DNS/service")
	return cmd
}

func lifecycleControlCmd(verb string, _ int) *cobra.Command {
	return &cobra.Command{
		Use: verb + " <project>",
		Short: map[string]string{
			"start":   "Start one linked project now",
			"stop":    "Stop one running project",
			"restart": "Restart and hold until ready (wake semantics)",
		}[verb],
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			ctx := cmd.Context()
			st := mustState()
			name := args[0]
			if _, ok := st.Linked(name); !ok {
				return unknownProject(name, st)
			}
			super, serr := supervisorClient(st)
			if serr != nil {
				return serr
			}
			switch verb {
			case "stop":
				ps, gerr := super.GetProcess(ctx, name)
				if gerr == nil && !ps.IsRunning && ps.Status == orchestrator.StateStopped {
					return o.Emit(map[string]any{"project": name, "phase": string(orchestrator.PhaseStopped)})
				}
				if err := super.Stop(ctx, name); err != nil {
					return fmt.Errorf("stop %s: %w", name, err)
				}
				if err := waitSuperPhase(ctx, super, name, false, 30*time.Second); err != nil {
					return err
				}
				return o.Emit(map[string]any{"project": name, "phase": string(orchestrator.PhaseStopped)})
			case "start":
				if err := ensureStarted(ctx, super, name); err != nil {
					return err
				}
				return o.Emit(map[string]any{"project": name, "phase": string(orchestrator.PhaseReady)})
			case "restart":
				ps, gerr := super.GetProcess(ctx, name)
				running := gerr == nil && ps.IsRunning
				if running {
					if err := super.Restart(ctx, name); err != nil {
						return fmt.Errorf("restart %s: %w", name, err)
					}
				} else if err := ensureStarted(ctx, super, name); err != nil {
					return err
				}
				return o.Emit(map[string]any{"project": name, "phase": string(orchestrator.PhaseReady)})
			}
			return ErrUsage
		},
	}
}

func ensureStarted(ctx context.Context, super *orchestrator.Client, name string) error {
	ps, err := super.GetProcess(ctx, name)
	if err == nil && ps.IsRunning && ps.IsReady == orchestrator.HealthReady {
		return nil
	}
	if err := super.Start(ctx, name); err != nil {
		if !strings.Contains(err.Error(), "already") {
			return fmt.Errorf("start %s: %w", name, err)
		}
	}
	return waitSuperPhase(ctx, super, name, true, orchestrator.DefaultWakeWait)
}

func waitSuperPhase(ctx context.Context, super *orchestrator.Client, name string, wantReady bool, within time.Duration) error {
	deadline := time.Now().Add(within)
	for {
		ps, err := super.GetProcess(ctx, name)
		if err == nil {
			if wantReady && ps.IsRunning && ps.IsReady == orchestrator.HealthReady {
				return nil
			}
			if !wantReady && !ps.IsRunning {
				return nil
			}
		}
		if time.Now().After(deadline) {
			state := "unknown"
			if err == nil {
				state = ps.Status + "/" + ps.IsReady
			}
			return fmt.Errorf("%s did not reach desired state within %s (last: %s)", name, within, state)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [--project P]",
		Short: "Read-only snapshot of daemon, DNS, and per-project state",
		Long: `Shows actual ports (never configured ones), per-project phases,
domains and resolver-file health. Read-only: never starts or stops anything.

When not to use: for live log lines use "dnser logs"; status does not tail.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			st := mustState()
			projects := []map[string]any{}
			var super *orchestrator.Client
			if sock := supervisorSocketPath(); fileExists(sock) {
				super = orchestrator.NewUDSClient(sock, "")
			}
			for _, lp := range st.ListLinked() {
				entry := map[string]any{
					"name":         lp.Name,
					"domain":       lp.Domain,
					"port":         lp.Port,
					"availability": lp.Availability,
					"phase":        string(orchestrator.PhaseStopped),
				}
				if super != nil {
					if ps, gerr := super.GetProcess(cmd.Context(), lp.Name); gerr == nil {
						if ps.IsRunning {
							if ps.IsReady == orchestrator.HealthReady {
								entry["phase"] = string(orchestrator.PhaseReady)
							} else {
								entry["phase"] = string(orchestrator.PhaseStarting)
							}
						} else if ps.Status == orchestrator.StateDone || ps.Status == orchestrator.StateError {
							entry["phase"] = string(orchestrator.PhaseStopped)
						}
						entry["pid"] = ps.Pid
					}
				}
				projects = append(projects, entry)
			}
			payload := map[string]any{
				"daemon":   map[string]any{"running": super != nil},
				"dns_port": dnsPortFor(st),
				"projects": projects,
			}
			if o.Format == FormatText {
				fmt.Fprintf(o.Stderr, "daemon: %v  dns: 127.0.0.1:%d\n", super != nil, dnsPortFor(st))
				for _, p := range projects {
					fmt.Fprintf(o.Stderr, "  %-16s %-8s %s:%v\n", p["name"], p["phase"], p["domain"], p["port"])
				}
				return nil
			}
			return o.Emit(payload)
		},
	}
}

func NewLogsCommand() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <project> [-f]",
		Short: "Print or follow a project's logs (stdout only carries log lines)",
		Long: `Streams the supervisor-managed log file. NDJSON mode wraps each line
as {ts, stream, line}. Ctrl-C once exits cleanly with code 130.

When not to use: for process metadata use "dnser status"; logs carry no state.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			st := mustState()
			name := args[0]
			if _, ok := st.Linked(name); !ok {
				return unknownProject(name, st)
			}
			dot, err := homeDot()
			if err != nil {
				return err
			}
			var super *orchestrator.Client
			if sock := supervisorSocketPath(); fileExists(sock) {
				super = orchestrator.NewUDSClient(sock, "")
			}
			path := filepath.Join(dot, "logs", name+".log")
			var lines []string
			if data, rerr := os.ReadFile(path); rerr == nil && len(strings.TrimSpace(string(data))) > 0 {
				lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			} else if super != nil {
				fetched, _, ferr := super.ProcessLogs(cmd.Context(), name, 0, 200)
				if ferr != nil {
					return fmt.Errorf("no logs for %s yet (file and supervisor both empty)", name)
				}
				lines = fetched
			} else {
				return fmt.Errorf("no log file yet at %s (has the project run?)", path)
			}
			for len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			if o.Format == FormatNDJSON {
				items := make([]any, 0, len(lines))
				modTime := fileModTime(path)
				for _, l := range lines {
					items = append(items, map[string]any{"ts": modTime, "stream": "stdout", "line": l})
				}
				return o.EmitList(items)
			}
			for _, l := range lines {
				fmt.Fprintln(o.Stdout, l)
			}
			if follow {
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
				fmt.Fprintln(o.Stderr, "following… Ctrl-C to stop")
				<-sig
				os.Exit(130)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the log file")
	return cmd
}

func NewExplainCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "explain [project]",
		Short: "Print fully-resolved effective configuration annotated by source",
		Long: `Every effective value carries its source (manifest|template|detected|
default). Env values are redacted unless --redact=false. Read-only.

When not to use: explain shows what WOULD be generated; use link/up to apply.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			_ = config.SourceManifest
			st := mustState()
			var name string
			if len(args) > 0 {
				name = args[0]
			} else if linked := st.ListLinked(); len(linked) > 0 {
				name = linked[0].Name
			}
			lp, ok := st.Linked(name)
			if !ok {
				return unknownProject(name, st)
			}
			m, err := loadManifestAt(lp.Dir)
			if err != nil {
				return err
			}
			tmpl, _ := config.GetTemplate(m.Type)
			eff, rerr := config.ResolveEffective(m, tmpl, config.FlagOverrides{})
			if rerr != nil {
				return rerr
			}
			payload := map[string]any{
				"project":      name,
				"type":         eff.Type,
				"domain":       eff.Domain,
				"port":         eff.Port,
				"availability": eff.Availability,
				"redacted_env": true,
			}
			if o.Format == FormatText {
				fmt.Fprintf(o.Stderr, "%s: domain=%s(%s) port=%d(%s) availability=%s(%s)\n",
					name, eff.Domain.Value, eff.Domain.Source, eff.Port.Value, eff.Port.Source, eff.Availability.Value, eff.Availability.Source)
				return nil
			}
			return o.Emit(payload)
		},
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func fileModTime(p string) string {
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}
