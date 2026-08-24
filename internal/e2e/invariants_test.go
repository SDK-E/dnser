package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SDK-E/dnser/internal/dnsl"
	"github.com/SDK-E/dnser/internal/generator"
	"github.com/SDK-E/dnser/internal/journal"
	"github.com/SDK-E/dnser/internal/orchestrator"
	"github.com/miekg/dns"
)

func startFakeDNS(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			var m dns.Msg
			if m.Unpack(buf[:n]) != nil {
				continue
			}
			resp := new(dns.Msg)
			resp.SetReply(&m)
			resp.Rcode = dns.RcodeNameError
			out, _ := resp.Pack()
			_, _ = conn.WriteToUDP(out, addr)
		}
	}()
	t.Cleanup(func() { _ = conn.Close() })
	return conn.LocalAddr().String()
}

func TestI1WatchdogContainsListenerDeath(t *testing.T) {
	dir := t.TempDir()
	w := dnsl.ResolverWriter{Dir: dir}
	entries := []dnsl.Entry{{Suffix: "i1.test", Addr: "127.0.0.1:35998"}}
	if _, err := w.Apply(entries); err != nil {
		t.Fatal(err)
	}
	fake := startFakeDNS(t)
	l := dnsl.New(dnsl.Config{Upstreams: []string{fake}, PortChain: []int{0}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	removed := make(chan struct{})
	wd := &dnsl.Watchdog{Interval: 15 * time.Millisecond, FailThreshold: 2,
		Probe: l.Probe,
		OnDead: func() {
			_ = w.Remove([]string{"i1.test"})
			close(removed)
		},
	}
	go wd.Run(ctx)
	select {
	case <-removed:
	case <-time.After(5 * time.Second):
		t.Fatal("I1 violated: dead resolver entry survived")
	}
}

func TestI2UnelevateRestoresPreStateByteForByte(t *testing.T) {
	dir := t.TempDir()
	pre := filepath.Join(dir, "etc", "resolver", "i2.test")
	if err := os.MkdirAll(filepath.Dir(pre), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre, []byte("nameserver 10.9.9.9\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	st, _ := journal.OpenStore(dir)
	p := journal.NewPlan("elevate-i2")
	p.Steps = append(p.Steps, &journal.Step{ID: "w", Kind: journal.KindFileWrite,
		Params: map[string]any{"path": pre, "content": "nameserver 127.0.0.1\nport 35353\n", "mode": 0o644}})
	ap := &journal.Applier{Registry: journal.NewFSRegistry()}
	if _, err := ap.Apply(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := st.Load(p.ID)
	if _, err := journal.Revert(context.Background(), st, reloaded, journal.NewFSRegistry()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(pre)
	info, _ := os.Stat(pre)
	if string(got) != "nameserver 10.9.9.9\n" || info.Mode().Perm() != 0o640 {
		t.Fatalf("I2 violated: %q %v", got, info.Mode().Perm())
	}
}

func TestI3PurgeRequiresSevereConfirmAndNonPurgeDeletesNothing(t *testing.T) {
	s := newSandbox(t)
	proj := filepath.Join(s.Home, "keepme")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".dnser.yaml"), []byte("domain: keepme.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.run(t, "link", proj)
	stateFile := filepath.Join(s.Home, ".dnser", "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatal("expected state file")
	}

	_, code := s.run(t, "uninstall")
	if code != 0 {
		t.Fatalf("plain uninstall must succeed keeping state")
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatal("I3 violated: non-purge deleted state")
	}

	_, code = s.run(t, "uninstall", "--purge", "--yes")
	if code != 3 {
		t.Fatalf("I3 violated: purge authorized by --yes alone (exit %d)", code)
	}
}

type failingTrust struct{}

func (failingTrust) IsTrusted(ctx context.Context, certPath string) (bool, error) { return false, nil }
func (failingTrust) Trust(ctx context.Context, certPath string) error {
	return errors.New("simulated trust store failure")
}
func (failingTrust) Untrust(ctx context.Context, certPath string) error { return nil }

func TestI4CATrustFailureLeavesInverseInJournal(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(cert, []byte("CERTDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := journal.OpenStore(dir)
	p := journal.NewPlan("ca-trust-i4")
	p.Steps = append(p.Steps, &journal.Step{ID: "trust", Kind: journal.KindCATrust,
		Params: map[string]any{"cert": cert}})
	ap := &journal.Applier{Registry: journal.NewFullRegistry(nil, failingTrust{}, nil)}
	_, err := ap.Apply(context.Background(), st, p)
	if err == nil {
		t.Fatal("trust failure must surface")
	}
	reloaded, lerr := st.Load(p.ID)
	if lerr != nil {
		t.Fatal(lerr)
	}
	step := reloaded.Steps[0]
	if step.Status != journal.StatusFailed || step.Capture == nil || step.Capture.CA == nil {
		t.Fatalf("I4 violated: inverse not captured on failed CA step: %+v", step)
	}
	reg := journal.NewFullRegistry(nil, failingTrust{}, nil)
	tr := reg[journal.KindCATrust].(interface {
		Invert(ctx context.Context, s *journal.Step) error
	})
	if invErr := tr.Invert(context.Background(), step); invErr != nil {
		t.Fatalf("I4 violated: invert of failed trust failed again: %v", invErr)
	}
}

func TestI5RootSpawnRefused(t *testing.T) {
	err := orchestrator.AssertUnprivileged()
	if os.Geteuid() == 0 {
		if err == nil {
			t.Fatal("I5 violated: root spawn allowed")
		}
		return
	}
	if err != nil {
		t.Fatalf("unprivileged user refused incorrectly: %v", err)
	}
	s := newSandbox(t)
	out, code := s.run(t, "up")
	_ = out
	_ = code
}

func TestI6BadGeneratedConfigNeverSwaps(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Caddyfile")
	lastKnownGood := "good-config {\n}\n"
	if werr := os.WriteFile(target, []byte(lastKnownGood), 0o644); werr != nil {
		t.Fatal(werr)
	}
	failValidate := func(path string) error {
		return fmt.Errorf("caddy validate: syntax error at line 1")
	}
	err := generator.EmitFile(target, []byte("{\nbroken"), 0o644, failValidate)
	if err == nil {
		t.Fatal("validation failure must block swap")
	}
	data, _ := os.ReadFile(target)
	if string(data) != lastKnownGood {
		t.Fatalf("I6 violated: LKG replaced by invalid config")
	}
}

func TestI7AllWaitsAreBounded(t *testing.T) {
	if orchestrator.DefaultWakeWait > 60*time.Second {
		t.Fatalf("I7: wake wait unbounded: %s", orchestrator.DefaultWakeWait)
	}
	if dnsl.ForwardTimeout > 10*time.Second {
		t.Fatalf("I7: forward timeout unbounded")
	}
	if dnsl.ProbeTimeout > 10*time.Second {
		t.Fatalf("I7: probe timeout unbounded")
	}
	if journal.DefaultStepTimeout > 60*time.Second {
		t.Fatalf("I7: journal step timeout unbounded")
	}
	if orchestrator.DefaultClientTimeout > 30*time.Second {
		t.Fatalf("I7: supervisor client timeout unbounded")
	}
}
