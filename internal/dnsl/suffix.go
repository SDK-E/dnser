package dnsl

import (
	"fmt"
	"strings"

	"golang.org/x/net/publicsuffix"
)

var reservedLocalTLDs = map[string]bool{
	"test":      true,
	"example":   true,
	"invalid":   true,
	"localhost": true,
	"local":     true,
}

func PublicSuffixWarnings(names []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		sfx := RootSuffixOf(n)
		if sfx == "" || seen[sfx] {
			continue
		}
		seen[sfx] = true
		tld := sfx
		if i := strings.LastIndex(sfx, "."); i >= 0 {
			tld = sfx[i+1:]
		}
		if reservedLocalTLDs[tld] {
			continue
		}
		splitErr := publicsuffixEffectiveTLDPlusOne(sfx)
		if splitErr != nil {
			out = append(out, fmt.Sprintf("registered suffix %q shadows real internet names under the public %q TLD; lookups for real domains ending in .%s will resolve locally instead", sfx, tld, tld))
		}
	}
	return out
}

func publicsuffixEffectiveTLDPlusOne(domain string) error {
	_, err := publicsuffix.EffectiveTLDPlusOne(domain)
	return err
}

func RootSuffixOf(domain string) string {
	d := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	d = strings.TrimPrefix(d, "*.")
	labels := strings.Split(d, ".")
	if len(labels) <= 1 || d == "" {
		return d
	}
	return strings.Join(labels[1:], ".")
}
