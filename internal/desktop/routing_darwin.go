//go:build darwin

package desktop

import (
	"fmt"

	"github.com/SDK-E/dnser/internal/setup"
)

func prepareRouting(_ string, _ *setup.State) error { return nil }
func routingNeedsPort53() bool                      { return false }

func commitRouting(tld, bind string, dnsPort int, st *setup.State) error {
	for _, d := range st.ResolverDomains {
		if d == tld {
			return nil
		}
	}
	if err := setup.WriteResolverDomain(setup.SystemRunner(), tld, bind, dnsPort); err != nil {
		return fmt.Errorf("route .%s queries: %w", tld, err)
	}
	st.ResolverDomains = append(st.ResolverDomains, tld)
	return nil
}

func routedAlready(tld string, st *setup.State) bool {
	for _, d := range st.ResolverDomains {
		if d == tld {
			return true
		}
	}
	return false
}
