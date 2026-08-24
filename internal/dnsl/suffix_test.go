package dnsl

import (
	"context"
	"testing"
)

func TestPublicSuffixWarnings(t *testing.T) {
	warns := PublicSuffixWarnings([]string{
		"myapp.dev",
		"box.io",
		"anything.com",
		"app.test",
		"auth.mycompany.internal",
		"service.local",
	})
	if len(warns) != 3 {
		t.Fatalf("expected exactly 3 warnings (dev, io, com), got %v", warns)
	}
	joined := warns[0] + warns[1] + warns[2]
	for _, tld := range []string{`"dev"`, `"io"`, `"com"`} {
		if !containsSubstr(joined, tld) {
			t.Fatalf("missing warning for %s in %v", tld, warns)
		}
	}
}

func containsSubstr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestRootSuffixOf(t *testing.T) {
	tests := []struct{ in, want string }{
		{"App.Test", "test"},
		{"*.deep.app.example.com", "app.example.com"},
		{"localhost", "localhost"},
	}
	for _, tt := range tests {
		if got := RootSuffixOf(tt.in); got != tt.want {
			t.Fatalf("RootSuffixOf(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnsureResolversGateBlocksDeadListener(t *testing.T) {
	dir := t.TempDir()
	w := ResolverWriter{Dir: dir}
	l := New(Config{PortChain: []int{0}})
	ctx := context.Background()
	if _, _, err := EnsureResolvers(ctx, l, w, []Entry{{Suffix: "test", Addr: "127.0.0.1:35353"}}); err == nil {
		t.Fatalf("must refuse to write resolver entries for dead listener (I1)")
	}
	if entries, _ := w.List(); len(entries) != 0 {
		t.Fatalf("no files may exist when gate blocks: %v", entries)
	}

	fake := startFakeUpstream(t, nil)
	l2 := New(Config{Upstreams: []string{fake.addr}, PortChain: []int{0}})
	defer func() { _ = l2.Stop(context.Background()) }()
	if err := l2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	applied, drifted, err := EnsureResolvers(ctx, l2, w, []Entry{{Suffix: "test", Addr: "127.0.0.1:35353"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || len(drifted) != 0 {
		t.Fatalf("live listener must pass gate: applied=%v drifted=%v", applied, drifted)
	}
}
