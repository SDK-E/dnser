package dnsl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolverWriterApplyVerifyRemove(t *testing.T) {
	dir := t.TempDir()
	w := ResolverWriter{Dir: dir}
	entries := []Entry{{Suffix: "test", Addr: "127.0.0.1:35353"}}

	applied, err := w.Apply(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 applied, got %v", applied)
	}
	applied, err = w.Apply(entries)
	if err != nil || len(applied) != 0 {
		t.Fatalf("re-apply must be idempotent: applied=%v err=%v", applied, err)
	}

	drifted, err := w.Verify(entries)
	if err != nil || len(drifted) != 0 {
		t.Fatalf("fresh write must verify clean: %v %v", drifted, err)
	}

	stale := "nameserver 127.0.0.1\nport 5353\n"
	if err := os.WriteFile(filepath.Join(dir, "test"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	drifted, err = w.Verify(entries)
	if err != nil || len(drifted) != 1 {
		t.Fatalf("stale port must be detected (ResolverDomainStale): %v %v", drifted, err)
	}
	if applied, err = w.Apply(entries); err != nil || len(applied) != 1 {
		t.Fatalf("apply must repair drift: %v %v", applied, err)
	}
	drifted, _ = w.Verify(entries)
	if len(drifted) != 0 {
		t.Fatalf("drift not repaired: %v", drifted)
	}

	if err := w.Remove([]string{"test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test")); !os.IsNotExist(err) {
		t.Fatalf("file must be gone after remove")
	}
	if err := w.Remove([]string{"test"}); err != nil {
		t.Fatalf("remove of missing file must be idempotent: %v", err)
	}
}

func TestResolverWriterNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission test meaningless")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()
	w := ResolverWriter{Dir: dir}
	_, err := w.Apply([]Entry{{Suffix: "test", Addr: "127.0.0.1:35353"}})
	if err == nil {
		t.Fatalf("unwritable dir must error")
	}
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("must return ErrNotWritable sentinel for fallback mode, got %v", err)
	}
}

func TestWatchdogRemovesEntriesAfterListenerDeath(t *testing.T) {
	dir := t.TempDir()
	w := ResolverWriter{Dir: dir}
	entries := []Entry{{Suffix: "test", Addr: "127.0.0.1:35999"}}
	if _, err := w.Apply(entries); err != nil {
		t.Fatal(err)
	}

	fake := startFakeUpstream(t, nil)
	l := New(Config{Upstreams: []string{fake.addr}, PortChain: []int{0}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}

	removed := make(chan struct{})
	wd := &Watchdog{
		Interval:      20 * time.Millisecond,
		FailThreshold: 2,
		Probe:         l.Probe,
		OnDead: func() {
			_ = w.Remove([]string{"test"})
			close(removed)
		},
	}
	go wd.Run(ctx)

	if err := l.Stop(ctx); err != nil {
		t.Fatalf("forced crash: %v", err)
	}

	select {
	case <-removed:
	case <-time.After(5 * time.Second):
		t.Fatalf("watchdog did not remove dead resolver entry in time (I1 violation)")
	}
	drifted, err := w.Verify(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifted) != 1 || drifted[0] != "test (missing)" {
		t.Fatalf("entry should now be missing after watchdog removal: %v", drifted)
	}
}
