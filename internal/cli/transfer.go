package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export -o dnser-export.json",
		Short: "Export all projects and records (JSON or BIND zone file)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			isBind := strings.HasSuffix(out, ".zone")
			data, _, err := renderExport(store, isBind)
			if err != nil {
				return err
			}
			target := out
			if target == "" {
				target = "dnser-export.json"
				if isBind {
					target = "dnser-export.zone"
				}
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return fmt.Errorf("write export %s: %w", target, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ exported %d project(s) to %s\n", len(store.Projects()), target)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file (.json or .zone)")
	return cmd
}

func renderExport(store *config.Store, bind bool) ([]byte, bool, error) {
	cfg := store.Get()
	if bind {
		return []byte(renderBIND(cfg)), true, nil
	}
	payload := struct {
		Exporter string           `json:"exporter"`
		Version  int              `json:"version"`
		Date     time.Time        `json:"date"`
		Projects []config.Project `json:"projects"`
	}{
		Exporter: "dnser/" + version,
		Version:  config.CurrentVersion,
		Date:     time.Now().UTC(),
		Projects: cfg.Projects,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(data, '\n'), false, nil
}

func renderBIND(cfg config.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "; exported by dnser %s at %s\n", version, time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintln(&b, "; dnser-specific settings are stored in '; dnser:' comments and restored on import")
	for _, p := range cfg.Projects {
		b.WriteString("\n")
		fmt.Fprintf(&b, "$ORIGIN %s\n", dns.Fqdn(p.Domain))
		apexBackend, wildcardRoute := "", false
		https := false
		for _, route := range p.Routes {
			if route.TCP || len(route.Backends) == 0 {
				continue
			}
			switch route.Host {
			case "@":
				apexBackend = route.Backends[0]
				https = route.HTTPS
			case "*":
				wildcardRoute = true
			}
		}
		fmt.Fprintf(&b, "; dnser: backend=%s wildcard=%t https=%t\n", apexBackend, wildcardRoute, https)

		hasApexA, hasWildA := false, false
		for _, r := range p.Records {
			switch {
			case r.Type == "A" && (r.Name == "@" || r.Name == "*"):
				hasApexA = true
			case strings.HasPrefix(r.Name, "*"):
				hasWildA = true
			}
		}
		if !hasApexA && !hasWildA && cfg.Settings.Bind != "" {
			fmt.Fprintf(&b, "@ %d IN A %s\n", configDefaultTTL(), cfg.Settings.Bind)
			if wildcardRoute {
				fmt.Fprintf(&b, "* %d IN A %s\n", configDefaultTTL(), cfg.Settings.Bind)
			}
		}
		for _, r := range p.Records {
			ttl := r.TTL
			if ttl == 0 {
				ttl = 120
			}
			switch r.Type {
			case "A", "AAAA":
				fmt.Fprintf(&b, "%s %d IN %s %s\n", ownerFor(r.Name), ttl, r.Type, r.Value)
			case "CNAME", "NS":
				fmt.Fprintf(&b, "%s %d IN %s %s\n", ownerFor(r.Name), ttl, r.Type, dns.Fqdn(r.Value))
			case "TXT":
				fmt.Fprintf(&b, "%s %d IN TXT \"%s\"\n", ownerFor(r.Name), ttl, r.Value)
			case "MX":
				fmt.Fprintf(&b, "%s %d IN MX %d %s\n", ownerFor(r.Name), ttl, r.Priority, dns.Fqdn(r.Value))
			case "SRV":
				fmt.Fprintf(&b, "%s %d IN SRV %d %d %d %s\n", ownerFor(r.Name), ttl, r.Priority, r.Weight, r.Port, dns.Fqdn(r.Value))
			}
		}
	}
	return b.String()
}

func ownerFor(name string) string {
	if name == "@" || name == "*" || name == "" {
		if name == "" {
			return "@"
		}
		return name
	}
	return name
}

func configDefaultTTL() uint32 { return 120 }

var dnserDirectiveRe = regexp.MustCompile(`;\s*dnser:\s*(.*)`)

type bindMeta struct {
	backend  string
	wildcard bool
	https    bool
}

func parseDirectives(content string) map[string]bindMeta {
	metas := make(map[string]bindMeta)
	currentOrigin := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$ORIGIN") {
			origin := strings.TrimSpace(strings.TrimPrefix(trimmed, "$ORIGIN"))
			currentOrigin = strings.ToLower(strings.TrimSuffix(origin, "."))
			continue
		}
		if m := dnserDirectiveRe.FindStringSubmatch(trimmed); m != nil && currentOrigin != "" {
			meta := metas[currentOrigin]
			for _, kv := range strings.Fields(m[1]) {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					continue
				}
				switch parts[0] {
				case "backend":
					meta.backend = strings.Trim(parts[1], `"`)
				case "wildcard":
					meta.wildcard = parts[1] == "true"
				case "https":
					meta.https = parts[1] == "true"
				}
			}
			metas[currentOrigin] = meta
		}
	}
	return metas
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import projects from a JSON export or BIND zone file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			var added int
			if strings.HasSuffix(strings.ToLower(path), ".zone") {
				added, err = importBIND(store, string(data))
			} else {
				added, err = importJSON(store, data)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ imported %d project(s) from %s\n", added, path)
			return nil
		},
	}
	return cmd
}

type importPayload struct {
	Projects []config.Project `json:"projects"`
}

func importJSON(store *config.Store, data []byte) (int, error) {
	var payload importPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, fmt.Errorf("parse JSON export: %w", err)
	}
	added := 0
	existing := map[string]bool{}
	for _, p := range store.Projects() {
		existing[p.Domain] = true
	}
	err := store.Update(func(c *config.Config) {
		for _, p := range payload.Projects {
			if existing[p.Domain] {
				continue
			}
			c.Projects = append(c.Projects, p)
			added++
		}
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

func importBIND(store *config.Store, content string) (int, error) {
	zones := map[string]*config.Project{}
	var order []string
	zp := dns.NewZoneParser(strings.NewReader(content), ".", "import")
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		header := rr.Header()
		name := strings.TrimSuffix(strings.ToLower(header.Name), ".")

		zoneDomain := name
		parts := strings.Split(name, ".")
		if len(parts) > 2 {
			zoneDomain = strings.Join(parts[len(parts)-2:], ".")
		}
		project := zones[zoneDomain]
		if project == nil {
			meta := parseDirectives(content)[zoneDomain]
			project = &config.Project{Domain: zoneDomain}
			if meta.backend != "" {
				routes := []config.Route{{Host: "@", Backends: []string{meta.backend}, HTTPS: meta.https}}
				if meta.wildcard {
					routes = append(routes, config.Route{Host: "*", Backends: []string{meta.backend}, HTTPS: meta.https})
				}
				project.Routes = routes
			}
			zones[zoneDomain] = project
			order = append(order, zoneDomain)
		}

		label := "@"
		if trimmed := strings.TrimSuffix(name, "."+zoneDomain); trimmed != name && trimmed != "" {
			label = trimmed
		}
		rec := config.Record{Name: label, TTL: header.Ttl}
		switch v := rr.(type) {
		case *dns.A:
			if label == "@" {
				continue
			}
			rec.Type, rec.Value = "A", v.A.String()
		case *dns.AAAA:
			if label == "@" {
				continue
			}
			rec.Type, rec.Value = "AAAA", v.AAAA.String()
		case *dns.CNAME:
			rec.Type, rec.Value = "CNAME", strings.TrimSuffix(v.Target, ".")
		case *dns.NS:
			rec.Type, rec.Value = "NS", strings.TrimSuffix(v.Ns, ".")
		case *dns.TXT:
			rec.Type, rec.Value = "TXT", strings.Join(v.Txt, "")
		case *dns.MX:
			rec.Type, rec.Value = "MX", strings.TrimSuffix(v.Mx, ".")
			rec.Priority = v.Preference
		case *dns.SRV:
			rec.Type, rec.Value = "SRV", strings.TrimSuffix(v.Target, ".")
			rec.Priority, rec.Weight, rec.Port = v.Priority, v.Weight, v.Port
		default:
			continue
		}
		project.Records = append(project.Records, rec)
	}
	if err := zp.Err(); err != nil {
		return 0, fmt.Errorf("parse zone file: %w", err)
	}

	added := 0
	err := store.Update(func(c *config.Config) {
		for _, name := range order {
			p := zones[name]
			dup := false
			for _, e := range c.Projects {
				if e.Domain == p.Domain {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			c.Projects = append(c.Projects, *p)
			added++
		}
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}
