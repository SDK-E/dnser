//go:build linux

package desktop

import (
	"fmt"

	"github.com/SDK-E/dnser/internal/setup"
)

func prepareRouting(exePath string, st *setup.State) error {
	if st.CapGranted {
		return nil
	}
	if err := setup.GrantBindCap(setup.SystemRunner(), exePath); err != nil {
		return fmt.Errorf("grant port 53 capability: %w", err)
	}
	st.CapGranted = true
	return nil
}

func routingNeedsPort53() bool { return true }

func commitRouting(tld, bind string, _ int, st *setup.State) error {
	saved, err := setup.ConfigureDNS(newElevatedRunner(), bind)
	if err != nil {
		return fmt.Errorf("point system resolver at %s: %w", bind, err)
	}
	st.DNSServices = saved
	st.DNSApplied = true
	return nil
}

func routedAlready(_ string, st *setup.State) bool {
	return st.DNSApplied
}
