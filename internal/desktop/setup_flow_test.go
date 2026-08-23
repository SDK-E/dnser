package desktop

import (
	"runtime"
	"testing"

	"github.com/SDK-E/dnser/internal/setup"
)

func TestSetupStatusIdle(t *testing.T) {
	svc := testService(t)
	v := svc.SetupStatus()
	if v.RoutingMode != "none" || v.Routed || v.CATrusted || v.DNSPort != 0 {
		t.Fatalf("unexpected idle status: %+v", v)
	}
	wantPort53 := runtime.GOOS != "darwin"
	if v.NeedsPort53 != wantPort53 {
		t.Fatalf("NeedsPort53 = %v, want %v", v.NeedsPort53, wantPort53)
	}
}

func TestRoutingModeFromState(t *testing.T) {
	cases := []struct {
		name string
		st   setup.State
		want string
	}{
		{"empty", setup.State{}, "none"},
		{"resolver-files", setup.State{ResolverDomains: []string{"test"}}, "resolver-files"},
		{"system-resolver", setup.State{DNSApplied: true}, "system-resolver"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routingMode(&tc.st); got != tc.want {
				t.Errorf("routingMode = %q, want %q", got, tc.want)
			}
		})
	}
}
