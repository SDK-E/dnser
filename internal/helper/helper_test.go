package helper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SDK-E/dnser/internal/journal"
)

type refusingElevator struct{ called bool }

func (r *refusingElevator) Elevate(ctx context.Context, self string, args []string) error {
	r.called = true
	return ErrRefused
}

func TestRefusalAppliesNothing(t *testing.T) {
	root := t.TempDir()
	p, err := BuildPlan(PlanRequest{
		RootDir:      root,
		ListenerPort: 35353,
		Suffixes:     []string{"test", "internal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("expected 2 resolver steps, got %d", len(p.Steps))
	}
	refuser := &refusingElevator{}
	err = refuser.Elevate(context.Background(), "dnser", []string{"helper", "run"})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("expected ErrRefused, got %v", err)
	}
	for _, s := range p.Steps {
		path := s.Params["path"].(string)
		if _, serr := os.Stat(path); !os.IsNotExist(serr) {
			t.Fatalf("refusal must apply nothing: %s exists", path)
		}
	}
}

func TestBuildPlanValidation(t *testing.T) {
	if _, err := BuildPlan(PlanRequest{}); err == nil {
		t.Fatal("empty plan must be rejected")
	}
	if _, err := BuildPlan(PlanRequest{Suffixes: []string{"../evil"}}); err == nil {
		t.Fatal("path traversal suffix must be rejected")
	}
	if _, err := BuildPlan(PlanRequest{ServiceDef: "/tmp/x.plist"}); err == nil {
		t.Fatal("service def without target must be rejected")
	}
}

func TestApplyAndRevertFullCycleInTempRoot(t *testing.T) {
	root := t.TempDir()
	p, err := BuildPlan(PlanRequest{
		RootDir:      root,
		ListenerPort: 35353,
		Suffixes:     []string{"test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := journal.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ap := &journal.Applier{Registry: journal.NewFSRegistry()}
	if _, err := ap.Apply(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	resolverFile := resolverPath(root, "test")
	data, err := os.ReadFile(resolverFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nameserver 127.0.0.1\nport 35353\n" {
		t.Fatalf("resolver content wrong: %q", data)
	}
	info, _ := os.Stat(resolverFile)
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("resolver perms: %v", info.Mode().Perm())
	}
	reloaded, _ := st.Load(p.ID)
	reports, err := journal.Revert(context.Background(), st, reloaded, journal.NewFSRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Status != journal.StatusReversed {
		t.Fatalf("revert report wrong: %+v", reports)
	}
	if _, err := os.Stat(resolverFile); !os.IsNotExist(err) {
		t.Fatalf("revert must remove file that did not pre-exist")
	}
	if _, err := os.Stat(filepath.Join(root, ".dnser", "journal")); err != nil {
		t.Fatalf("journal dir expected under root: %v", err)
	}
}

func TestAlreadyAppliedDetection(t *testing.T) {
	root := t.TempDir()
	p, err := BuildPlan(PlanRequest{RootDir: root, ListenerPort: 5353, Suffixes: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if AlreadyApplied(ctx, p) {
		t.Fatal("fresh plan cannot already be applied")
	}
	st, _ := journal.OpenStore(root)
	ap := &journal.Applier{Registry: journal.NewFSRegistry()}
	if _, err := ap.Apply(ctx, st, p); err != nil {
		t.Fatal(err)
	}
	fresh, _ := BuildPlan(PlanRequest{RootDir: root, ListenerPort: 5353, Suffixes: []string{"dev"}})
	if !AlreadyApplied(ctx, fresh) {
		t.Fatal("identical live state must read as already applied")
	}
	drifted, _ := BuildPlan(PlanRequest{RootDir: root, ListenerPort: 9999, Suffixes: []string{"dev"}})
	if AlreadyApplied(ctx, drifted) {
		t.Fatal("different port must not read as applied")
	}
}

func TestRenderServiceDefs(t *testing.T) {
	plist, err := RenderServiceDef("darwin", ServiceVars{BinPath: "/opt/dnser", LogsDir: "/l", StateDir: "/s"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(plist, []string{"<string>/opt/dnser</string>", "<string>daemon</string>", "/l/daemon.log", "enterprises.sdk.dnser"}) {
		t.Fatalf("launchd template substitution broken:\n%s", plist)
	}
	unit, err := RenderServiceDef("linux", ServiceVars{BinPath: "/usr/bin/dnser", LogsDir: "/l", StateDir: "/s"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(unit, []string{"ExecStart=/usr/bin/dnser daemon", "WantedBy=default.target"}) {
		t.Fatalf("systemd unit substitution broken:\n%s", unit)
	}
	if _, err := RenderServiceDef("windows", ServiceVars{}); err == nil {
		t.Fatal("windows def intentionally deferred to M9")
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, n := range needles {
		if !filepath_Contains(haystack, n) {
			return false
		}
	}
	return true
}

func filepath_Contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

var _ = filepath.Join
