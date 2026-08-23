//go:build windows

package desktop

import (
	"fmt"

	"github.com/SDK-E/dnser/internal/setup"
)

func prepareRouting(_ string, _ *setup.State) error { return nil }
func routingNeedsPort53() bool                      { return true }

func commitRouting(_ string, bind string, dnsPort int, st *setup.State) error {
	if dnsPort != 53 {
		return fmt.Errorf("port 53 unavailable (dnser on %d); free port 53 and run setup again", dnsPort)
	}
	saved, err := setup.ConfigureDNS(elevRunner{}, bind)
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
