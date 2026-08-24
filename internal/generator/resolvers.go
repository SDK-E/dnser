package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SDK-E/dnser/internal/config"
)

type ResolverReg struct {
	Suffix string
	Port   int
}

type Answer struct {
	Name  string
	Type  string
	Value string
}

func RootSuffix(domain string) string {
	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "*.")
	labels := strings.Split(d, ".")
	if len(labels) <= 1 {
		return d
	}
	return strings.Join(labels[1:], ".")
}

func resolverRegistrations(names []string, dnsPort int) []ResolverReg {
	if dnsPort == 0 {
		return nil
	}
	seen := map[string]bool{}
	var suffixes []string
	for _, n := range names {
		s := RootSuffix(n)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		suffixes = append(suffixes, s)
	}
	sort.Strings(suffixes)
	out := make([]ResolverReg, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, ResolverReg{Suffix: s, Port: dnsPort})
	}
	return out
}

func answerTable(m *config.Manifest, names []string, primary string) ([]Answer, error) {
	var out []Answer
	seen := map[string]bool{}
	addAnswer := func(name, typ, value string) {
		key := strings.ToLower(name) + "|" + strings.ToUpper(typ) + "|" + value
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Answer{Name: name, Type: typ, Value: value})
	}
	for _, n := range names {
		if strings.HasPrefix(n, "*.") || !underOrEqual(n, names) {
			continue
		}
		addAnswer(n, "A", loopback)
	}
	for _, r := range m.Records {
		name := r.Name
		if name == "@" {
			name = primary
		}
		addAnswer(name, strings.ToUpper(r.Type), r.Value)
	}
	for _, svcName := range sortedKeys(m.Services) {
		svc := m.Services[svcName]
		if !svc.DNS {
			continue
		}
		addAnswer(fmt.Sprintf("%s.%s", svcName, primary), "A", loopback)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

const loopback = "127.0.0.1"
