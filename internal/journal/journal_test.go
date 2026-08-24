package journal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWritesAheadAndMarksApplied(t *testing.T) {
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	p := NewPlan("test")
	target := filepath.Join(root, "resolver", "test")
	p.Steps = append(p.Steps, &Step{ID: "s1", Kind: KindFileWrite, Params: map[string]any{"path": target, "content": "nameserver 127.0.0.1\nport 35353\n", "mode": 0o644}})
	ap := &Applier{Registry: NewFSRegistry()}
	if _, err := ap.Apply(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusApplied {
		t.Fatalf("plan status %q want applied", p.Status)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "nameserver 127.0.0.1\nport 35353\n" {
		t.Fatalf("file content wrong: %q err=%v", got, err)
	}
}

func TestCrashMidPlanIsResumableBothDirections(t *testing.T) {
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	preExisting := filepath.Join(root, "resolver", "keep")
	if err := os.MkdirAll(filepath.Dir(preExisting), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preExisting, []byte("search lan\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(root, "resolver", "fresh")
	p := NewPlan("crash")
	p.Steps = append(p.Steps,
		&Step{ID: "step-applied", Kind: KindFileWrite, Params: map[string]any{"path": fresh, "content": "x\n", "mode": 0o644}},
		&Step{ID: "step-inflight", Kind: KindFileWrite, Params: map[string]any{"path": preExisting, "content": "overwritten\n", "mode": 0o644}},
		&Step{ID: "step-never", Kind: KindFileWrite, Params: map[string]any{"path": filepath.Join(root, "resolver", "third"), "content": "y\n", "mode": 0o644}},
	)
	p.Steps[0].Status = StatusApplied
	p.Steps[0].Capture = &Capture{File: &FileCapture{Path: fresh, Existed: false}}
	p.Steps[1].Status = StatusInflight
	p.Steps[1].Capture = &Capture{File: &FileCapture{Path: preExisting, Existed: true, Perm: 0o600, Content: "search lan\n"}}
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}

	reloaded, err := st.Load(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusPending || !HasInterrupted(reloaded) {
		t.Fatalf("crashed plan must read as interrupted/pending: %s %v", reloaded.Status, HasInterrupted(reloaded))
	}

	finished, err := Finish(context.Background(), st, reloaded, NewFSRegistry())
	if err != nil {
		t.Fatalf("finish forward: %v", err)
	}
	if len(finished) < 2 || reloaded.Status != StatusApplied {
		t.Fatalf("forward convergence failed: status=%s reports=%v", reloaded.Status, finished)
	}
	data, _ := os.ReadFile(preExisting)
	if string(data) != "overwritten\n" {
		t.Fatalf("forward finish did not complete inflight step: %q", data)
	}

	back, err := st.Load(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Revert(context.Background(), st, back, NewFSRegistry()); err != nil {
		t.Fatal(err)
	}
	info, statErr := os.Stat(fresh)
	if !os.IsNotExist(statErr) && info != nil {
		t.Fatalf("unset-marker violated: fresh file must be removed on revert")
	}
	restored, err := os.Stat(preExisting)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Mode().Perm() != 0o600 {
		t.Fatalf("perms not restored byte-for-byte: %v", restored.Mode().Perm())
	}
	body, _ := os.ReadFile(preExisting)
	if string(body) != "search lan\n" {
		t.Fatalf("content not restored: %q", body)
	}
	if back.Status != StatusReversed {
		t.Fatalf("reverted plan status %q", back.Status)
	}
}

func TestRevertSkipsVanishedArtifacts(t *testing.T) {
	root := t.TempDir()
	st, _ := OpenStore(root)
	target := filepath.Join(root, "gone")
	p := NewPlan("vanished")
	p.Steps = append(p.Steps, &Step{
		ID:      "s",
		Kind:    KindFileWrite,
		Params:  map[string]any{"path": filepath.Join(root, "other"), "content": "z"},
		Capture: &Capture{File: &FileCapture{Path: target, Existed: true, Perm: 0o644, Content: "old"}},
		Status:  StatusApplied,
	})
	if _, err := Revert(context.Background(), st, p, NewFSRegistry()); err != nil {
		t.Fatalf("reverting over a vanished artifact must skip it: %v", err)
	}
}

func TestFailedStepStopsAndJournals(t *testing.T) {
	root := t.TempDir()
	st, _ := OpenStore(root)
	p := NewPlan("fail")
	badKind := StepKind("nonexistent_kind")
	p.Steps = append(p.Steps, &Step{ID: "boom", Kind: badKind, Params: map[string]any{}})
	ap := &Applier{Registry: Registry{}}
	_, err := ap.Apply(context.Background(), st, p)
	if err == nil {
		t.Fatal("unknown kind must fail the plan")
	}
	reloaded, lerr := st.Load(p.ID)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if reloaded.Status != StatusFailed || reloaded.Steps[0].Status != StatusFailed {
		t.Fatalf("failure must be journaled: plan=%s step=%s", reloaded.Status, reloaded.Steps[0].Status)
	}
}
