package journal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type ExecutorFuncs struct {
	CaptureFn func(ctx context.Context, s *Step) (*Capture, error)
	ApplyFn   func(ctx context.Context, s *Step) error
	InvertFn  func(ctx context.Context, s *Step) error
	VerifyFn  func(ctx context.Context, s *Step) error
}

func (e ExecutorFuncs) Capture(ctx context.Context, s *Step) (*Capture, error) {
	return e.CaptureFn(ctx, s)
}

func (e ExecutorFuncs) Apply(ctx context.Context, s *Step) error {
	return e.ApplyFn(ctx, s)
}

func (e ExecutorFuncs) Invert(ctx context.Context, s *Step) error {
	return e.InvertFn(ctx, s)
}

func (e ExecutorFuncs) Verify(ctx context.Context, s *Step) error {
	return e.VerifyFn(ctx, s)
}

func TestKillDashNineMidPlanResumes(t *testing.T) {
	if os.Getenv("DNSER_CRASH_CHILD") == "" {
		return
	}
	root := os.Getenv("DNSER_CRASH_ROOT")
	st, err := OpenStore(root)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	p := NewPlan("crash-child")
	target := filepath.Join(root, "resolver", "a")
	p.Steps = append(p.Steps,
		&Step{ID: "first", Kind: KindFileWrite, Params: map[string]any{"path": target, "content": "one\n", "mode": 0o644}},
		&Step{ID: "second", Kind: KindFileWrite, Params: map[string]any{"path": filepath.Join(root, "resolver", "b"), "content": "two\n", "mode": 0o644}},
	)
	fs := &FS{}
	crasher := ExecutorFuncs{
		CaptureFn: fs.Capture,
		ApplyFn: func(ctx context.Context, s *Step) error {
			if strings.HasSuffix(s.Params["path"].(string), "b") {
				fmt.Fprintln(os.Stderr, "child dying at write-ahead boundary of step b")
				os.Exit(137)
			}
			return fs.Apply(ctx, s)
		},
		InvertFn: fs.Invert,
		VerifyFn: fs.Verify,
	}
	ap := &Applier{Registry: Registry{KindFileWrite: crasher}}
	_, _ = ap.Apply(context.Background(), st, p)
	os.Exit(0)
}

func TestSubprocessCrashLeavesResumableJournal(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestKillDashNineMidPlanResumes", "-test.v")
	cmd.Env = append(os.Environ(), "DNSER_CRASH_CHILD=1", "DNSER_CRASH_ROOT="+root)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("crash child hung")
	}
	if !strings.Contains(out.String(), "exit status 137") && cmd.ProcessState == nil {
		t.Fatalf("expected hard exit")
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := st.List()
	if err != nil || len(plans) != 1 {
		t.Fatalf("journal must contain exactly the crashed plan: %v %v", plans, err)
	}
	crashed := plans[0]
	if crashed.Status != StatusPending || !HasInterrupted(crashed) {
		t.Fatalf("crashed plan not recognized as interrupted: %s", crashed.Status)
	}
	stepStates := map[string]StepStatus{}
	for _, s := range crashed.Steps {
		stepStates[s.ID] = s.Status
	}
	if stepStates["first"] != StatusApplied || stepStates["second"] != StatusInflight {
		t.Fatalf("write-ahead boundary wrong after kill -9: %+v", stepStates)
	}
	if _, err := os.Stat(filepath.Join(root, "resolver", "b")); !os.IsNotExist(err) {
		t.Fatalf("inflight step must NOT have produced a file before crash")
	}

	reports, err := Finish(context.Background(), st, crashed, NewFSRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if crashed.Status != StatusApplied || len(reports) < 2 {
		t.Fatalf("finish did not converge: %s %v", crashed.Status, reports)
	}
	data, rerr := os.ReadFile(filepath.Join(root, "resolver", "b"))
	if rerr != nil || string(data) != "two\n" {
		t.Fatalf("resumed plan did not write second file: %q %v", data, rerr)
	}
}

func TestRevertAfterRealCrashRestoresPreStateByteForByte(t *testing.T) {
	root := t.TempDir()
	pre := filepath.Join(root, "resolver", "pre")
	if err := os.MkdirAll(filepath.Dir(pre), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pre, []byte("nameserver 10.0.0.1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	st, _ := OpenStore(root)
	p := NewPlan("overwrite-pre")
	p.Steps = append(p.Steps, &Step{ID: "w", Kind: KindFileWrite, Params: map[string]any{"path": pre, "content": "hijacked\n", "mode": 0o644}})
	ap := &Applier{Registry: NewFSRegistry()}
	if _, err := ap.Apply(context.Background(), st, p); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := st.Load(p.ID)
	if _, err := Revert(context.Background(), st, reloaded, NewFSRegistry()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(pre)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("perm drift after revert: %v want 0640", info.Mode().Perm())
	}
	body, _ := os.ReadFile(pre)
	if string(body) != "nameserver 10.0.0.1\n" {
		t.Fatalf("content drift after revert: %q", body)
	}
}

func TestPlanFileRoundTripViaTempPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	p := NewPlan("rt")
	p.Steps = append(p.Steps, &Step{ID: "x", Kind: KindFileWrite, Params: map[string]any{"path": filepath.Join(dir, "t"), "content": "c"}})
	if err := SavePlanTo(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPlanFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID || len(got.Steps) != 1 || got.Steps[0].Params["path"] == nil {
		t.Fatalf("round trip lost data: %+v", got)
	}
}
