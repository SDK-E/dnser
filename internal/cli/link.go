package cli

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/dnsl"
	"github.com/SDK-E/dnser/internal/generator"
	"github.com/SDK-E/dnser/internal/state"
	"github.com/spf13/cobra"
)

func homeDot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".dnser"), nil
}

func loadManifestAt(dir string) (*config.Manifest, error) {
	path := filepath.Join(dir, ".dnser.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no .dnser.yaml in %s (run: dnser init --dir %s)", dir, dir)
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := config.Decode(data)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func NewLinkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "link <path>",
		Short:   "Register a project with dnser and generate its infrastructure",
		Example: `  dnser link ~/code/my-api`,
		Long: `Validates the project manifest strictly, allocates and pins a port,
registers domains (warning on public-suffix shadowing), generates Caddy,
supervisor and DNS artifacts with validate-before-swap.

When not to use: do not link directories without a .dnser.yaml; use
"dnser init" first. Re-running on a linked project is safe (refresh+diff).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := OutputOf(cmd)
			dir := absDir(args[0])
			m, err := loadManifestAt(dir)
			if err != nil {
				return err
			}
			st := mustState()
			name := filepath.Base(dir)
			adoptProjectPort(st, name, derefInt(m.Port))
			port, err := st.AllocatePort(name, derefInt(m.Port))
			if err != nil {
				return err
			}
			svcPorts := map[string]int{}
			for svcName, svc := range m.Services {
				p, err := st.AllocateServicePort(name, svcName, derefInt(svc.Port))
				if err != nil {
					return err
				}
				svcPorts[svcName] = p
			}
			dot, err := homeDot()
			if err != nil {
				return err
			}
			logsDir := filepath.Join(dot, "logs")
			out, gerr := generator.Generate(generator.Input{
				Project:      name,
				Root:         dot,
				Dir:          dir,
				Manifest:     m,
				Port:         port,
				ServicePorts: svcPorts,
				LogsDir:      logsDir,
				DNSPort:      dnsPortFor(st),
			})
			if gerr != nil {
				return gerr
			}
			genDir := filepath.Join(dot, "generated", name)
			plan, merr := applyMutation(cmd.Context(), "link:"+name, []MutationWrite{
				{Path: filepath.Join(genDir, "Caddyfile"), Content: out.Caddyfile, Mode: 0o644},
				{Path: filepath.Join(genDir, "process-compose.yaml"), Content: out.Supervisor, Mode: 0o644},
			})
			if merr != nil {
				return merr
			}
			fmt.Fprintf(o.Stderr, "journalled mutation %s\n", plan.ID)
			warnings := dnsl.PublicSuffixWarnings(m.EffectiveNames())
			if err := st.Link(state.LinkedProject{
				Name:         name,
				Dir:          dir,
				Domain:       m.PrimaryDomain(name),
				Port:         port,
				Availability: availabilityOr(m.Availability),
				ServicePorts: svcPorts,
			}); err != nil {
				return err
			}
			for _, w := range warnings {
				fmt.Fprintf(o.Stderr, "WARNING public-suffix shadowing: %s\n", w)
			}
			summary := map[string]any{
				"project":      name,
				"domain":       m.PrimaryDomain(name),
				"port":         port,
				"availability": availabilityOr(m.Availability),
				"generated":    genDir,
			}
			if o.Format == FormatText {
				fmt.Fprintf(o.Stderr, "linked %s → %s:%d (%s)\nnext: dnser up\n", name, m.PrimaryDomain(name), port, availabilityOr(m.Availability))
				return nil
			}
			return o.Emit(summary)
		},
	}
	return cmd
}

func NewUnlinkCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink <name>",
		Short: "Remove a project registration and regenerate infrastructure",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st := mustState()
			removed, err := st.Unlink(args[0])
			if err != nil {
				return err
			}
			if !removed {
				return unknownProject(args[0], st)
			}
			fmt.Fprintf(OutputOf(cmd).Stderr, "unlinked %s\n", args[0])
			return nil
		},
	}
}

func unknownProject(name string, st *state.Store) error {
	var known []string
	for _, p := range st.ListLinked() {
		known = append(known, p.Name)
	}
	return fmt.Errorf("%w: unknown project %q; known: %s", ErrUsage, name, strings.Join(known, ", "))
}

func dnsPortFor(st *state.Store) int {
	if p, ok := st.ServicePort("dnser", "dns"); ok {
		return p
	}
	return 35353
}

func availabilityOr(a string) string {
	if a == "" {
		return "always"
	}
	return a
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func absDir(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func OutputOf(cmd *cobra.Command) *Output {
	if v := cmd.Context().Value(outputKey{}); v != nil {
		return v.(*Output)
	}
	return &Output{Stdout: os.Stdout, Stderr: os.Stderr, Format: FormatText}
}

type outputKey struct{}

func stateOpen() (*state.Store, error) {
	path, perr := state.DefaultPath()
	if perr != nil {
		return nil, perr
	}
	return state.Open(path)
}

func mustState() *state.Store {
	st, err := stateOpen()
	if err != nil {
		panic(fmt.Sprintf("load state: %v", err))
	}
	return st
}

func adoptProjectPort(st *state.Store, project string, preferred int) {
	if preferred == 0 {
		return
	}
	pin, hasPin := st.Port(project)
	if !hasPin || pin == preferred {
		return
	}
	if portBusy(preferred) {
		_ = st.SetPort(project, preferred)
	}
}

func portBusy(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
