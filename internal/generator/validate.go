package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SDK-E/dnser/internal/config"
)

var reservedCaddyKeys = []string{"reverse_proxy", "tls"}

var conflictingProcessKeys = []string{"command", "environment", "working_dir"}

func validateForGenerate(m *config.Manifest, names []string, primary string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Caddy != nil {
		for key := range m.Caddy.Value {
			for _, reserved := range reservedCaddyKeys {
				if key == reserved {
					return fmt.Errorf("caddy.%s conflicts with a dnser-required directive; required directives (%s) cannot be removed or replaced, only extended via other directives", key, strings.Join(reservedCaddyKeys, ", "))
				}
			}
		}
	}
	if m.Process != nil {
		for _, bad := range conflictingProcessKeys {
			if _, exists := m.Process.Value[bad]; exists {
				return fmt.Errorf("process.%s conflicts with the typed manifest keys; remove it or set the typed key instead (allowed: everything process-compose supports except %s)", bad, strings.Join(conflictingProcessKeys, ", "))
			}
		}
	}
	for i, r := range m.Records {
		target := r.Name
		switch target {
		case "@":
			target = primary
		default:
			target = strings.TrimPrefix(target, "*.")
		}
		if !underOrEqual(target, names) {
			return fmt.Errorf("records[%d] name %q is outside every declared domain; records must live under declared domains", i, r.Name)
		}
	}
	for i, r := range m.Routes {
		hasPort := r.Port != nil
		hasBackend := r.Backend != ""
		if hasPort == hasBackend {
			return fmt.Errorf("routes[%d]: exactly one of port or backend is required", i)
		}
		if r.Port != nil && (*r.Port < 1 || *r.Port > 65535) {
			return fmt.Errorf("routes[%d].port must be in 1-65535", i)
		}
		if r.Host != "" && !underOrEqual(r.Host, names) {
			return fmt.Errorf("routes[%d].host %q is outside every declared domain", i, r.Host)
		}
	}
	for name, enabled := range httpsPerName(m) {
		_ = enabled
		if !underOrEqual(name, names) {
			return fmt.Errorf("https map references undeclared name %q", name)
		}
	}
	if m.ForceHTTPS {
		for name, enabled := range httpsPerName(m) {
			if !enabled {
				return fmt.Errorf("force_https contradicts https: false for declared name %q", name)
			}
		}
	}
	return nil
}

func underOrEqual(target string, pool []string) bool {
	t := normalizeName(target)
	for _, p := range pool {
		n := normalizeName(p)
		if t == n || strings.HasSuffix(t, "."+n) {
			return true
		}
	}
	return false
}

func normalizeName(n string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(n)), "*."), ".")
}

func httpsPerName(m *config.Manifest) map[string]bool {
	out := map[string]bool{}
	if m.HTTPS.PerName != nil {
		for k, v := range m.HTTPS.PerName {
			out[k] = v
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
