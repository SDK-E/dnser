package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/spf13/cobra"
)

type initOptions struct {
	dir    string
	name   string
	typ    string
	port   int
	domain string
	force  bool
}

func NewInitCommand() *cobra.Command {
	opts := &initOptions{}
	cmd := &cobra.Command{
		Use:   "init [--type T] [dir]",
		Short: "Scaffold a .dnser.yaml manifest",
		Long: `Scaffold a .dnser.yaml manifest for a project.

Writes a starter manifest tuned for the given --type (or detected from the
directory contents). The written manifest is validated strictly before it is
written atomically; an existing manifest is diffed and requires confirmation
unless --force is passed.

Examples:
  dnser init                          detect the stack in the current dir
  dnser init --type=nodejs ~/app      scaffold for Node.js in ~/app
  dnser init --type=bash --port 8080  pin a port for a shell-script server

When not to use this command: to change routing or services of a linked
project — edit .dnser.yaml directly and re-run "dnser link".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.dir = "."
			if len(args) == 1 {
				opts.dir = args[0]
			}
			return runInit(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "project name (defaults to directory name)")
	cmd.Flags().StringVar(&opts.typ, "type", "", "project type template ("+strings.Join(mustKnownTypes(), ", ")+")")
	cmd.Flags().IntVar(&opts.port, "port", 0, "pin the dev-server port")
	cmd.Flags().StringVar(&opts.domain, "domain", "", "primary domain")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite an existing manifest without confirmation")
	_ = cmd.RegisterFlagCompletionFunc("type", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return mustKnownTypes(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func mustKnownTypes() []string {
	registry, err := config.LoadRegistry()
	if err != nil {
		return []string{}
	}
	return config.KnownTypes(registry)
}

func runInit(cmd *cobra.Command, opts *initOptions) error {
	absDir, err := filepath.Abs(opts.dir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	name := opts.name
	if name == "" {
		name = filepath.Base(absDir)
	}

	typ := opts.typ
	tmpl, err := resolveTemplate(typ, absDir)
	if err != nil {
		return err
	}
	typ = tmpl.Name

	m := buildManifest(name, typ, tmpl, opts)

	manifestPath, exists, err := config.Find(absDir)
	if err != nil {
		return err
	}
	if exists && !opts.force {
		if _, err := config.Load(manifestPath); err != nil {
			return fmt.Errorf("existing %s does not decode: %w", manifestPath, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "existing manifest at %s\n", manifestPath)
		fmt.Fprintf(cmd.ErrOrStderr(), "refusing to overwrite; pass --force or edit in place\n")
		return fmt.Errorf("init: manifest already exists")
	}

	data, err := config.Encode(m)
	if err != nil {
		return fmt.Errorf("render manifest: %w", err)
	}
	if _, err := config.Decode(data); err != nil {
		return fmt.Errorf("generated manifest invalid: %w", err)
	}
	if err := config.WriteAtomic(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", manifestPath)
	fmt.Fprintf(cmd.OutOrStdout(), "next: dnser link %s\n", absDir)
	return nil
}

func resolveTemplate(typ, absDir string) (*config.Template, error) {
	registry, err := config.LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("load template registry: %w", err)
	}
	if typ != "" {
		t, ok := registry[typ]
		if !ok {
			return nil, fmt.Errorf("unknown type %q (known types: %s)", typ, strings.Join(config.KnownTypes(registry), ", "))
		}
		return t, nil
	}
	if detected := detectType(absDir, registry); detected != "" {
		return registry[detected], nil
	}
	fallback := "static"
	t, ok := registry[fallback]
	if !ok {
		return nil, fmt.Errorf("no template detected and fallback %q missing", fallback)
	}
	return t, nil
}

func detectType(dir string, registry map[string]*config.Template) string {
	bestName := ""
	bestLen := -1
	for name, t := range registry {
		for _, f := range t.Detect.Files {
			full := filepath.Join(dir, f)
			if st, err := os.Stat(full); err == nil && !st.IsDir() {
				if len(f) > bestLen {
					bestName, bestLen = name, len(f)
				}
			}
		}
	}
	return bestName
}

func buildManifest(name, typ string, tmpl *config.Template, opts *initOptions) *config.Manifest {
	m := &config.Manifest{
		Type:   typ,
		Domain: opts.domain,
	}
	if m.Domain == "" {
		m.Domain = fmt.Sprintf("%s.%s", strings.ReplaceAll(strings.ToLower(name), "_", "-"), config.DefaultTLD)
	}
	command := tmpl.Command
	if command != "" {
		m.Command = command
	}
	if opts.port > 0 {
		p := opts.port
		m.Port = &p
	} else if tmpl.Detect.DefaultPort > 0 {
		p := tmpl.Detect.DefaultPort
		m.Port = &p
	}
	if m.Port != nil && *m.Port == 0 {
		m.Port = nil
	}
	return m
}
