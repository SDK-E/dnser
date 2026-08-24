package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLifecycleTable(t *testing.T) {
	tests := []struct {
		name    string
		from    Phase
		event   Event
		want    Phase
		action  Action
		wantErr bool
	}{
		{"start from stopped", PhaseStopped, EventStartRequested, PhaseStarting, ActionSupervisorStart, false},
		{"stop when stopped is no-op", PhaseStopped, EventStopRequested, PhaseStopped, ActionNone, false},
		{"ready while starting", PhaseStarting, EventSuperReady, PhaseReady, ActionNone, false},
		{"running without readiness", PhaseStarting, EventSuperRunning, PhaseRunning, ActionNone, false},
		{"exit during start", PhaseStarting, EventSuperExited, PhaseStopped, ActionNone, false},
		{"stop from running", PhaseRunning, EventStopRequested, PhaseStopping, ActionSupervisorStop, false},
		{"idle stop from ready", PhaseReady, EventIdleFired, PhaseStopping, ActionSupervisorStop, false},
		{"exit ends stopping", PhaseStopping, EventSuperExited, PhaseStopped, ActionNone, false},
		{"backoff then restart", PhaseBackoff, EventBackoffElapsed, PhaseStarting, ActionSupervisorStart, false},
		{"stop cancels backoff", PhaseBackoff, EventStopRequested, PhaseStopped, ActionNone, false},
		{"cannot exit from stopped", PhaseStopped, EventSuperExited, "", ActionNone, true},
		{"cannot idle-fire stopped", PhaseStopped, EventIdleFired, "", ActionNone, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := NewLifecycle("p")
			lc.Phase = tt.from
			act, err := lc.Send(tt.event, time.Now())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s × %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if lc.Phase != tt.want || act != tt.action {
				t.Fatalf("got %s/%s want %s/%s", lc.Phase, act, tt.want, tt.action)
			}
		})
	}
}

type stubSuper struct {
	mu       sync.Mutex
	states   map[string]ProcState
	started  []string
	stopped  []string
	restarts []string
}

func (s *stubSuper) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/processes", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		list := make([]ProcState, 0, len(s.states))
		for _, st := range s.states {
			list = append(list, st)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"processes": list})
	})
	mux.HandleFunc("/process/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.Path)
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Method == http.MethodGet && len(parts) == 2 {
			st := s.states[parts[1]]
			_ = json.NewEncoder(w).Encode(st)
			return
		}
		if len(parts) == 3 {
			name := parts[2]
			switch parts[1] {
			case "start":
				s.started = append(s.started, name)
				st := s.states[name]
				st.Name, st.Status, st.IsRunning = name, StatePending, false
				s.states[name] = st
			case "stop":
				s.stopped = append(s.stopped, name)
				st := s.states[name]
				st.IsRunning, st.IsReady = false, HealthNotReady
				s.states[name] = st
			case "restart":
				s.restarts = append(s.restarts, name)
			}
		}
		w.WriteHeader(200)
		fmt.Fprint(w, "{}")
	})
	return mux
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(p[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func newTestManager(t *testing.T, super *stubSuper) (*Manager, string) {
	t.Helper()
	srv := httptest.NewServer(super.handler())
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	client := NewTCPClient(addr, "")
	idle := NewIdleTracker(fakeClock{}, func() time.Duration { return 5 * time.Minute }, func() time.Duration { return 30 * time.Second })
	return NewManager(client, idle, 3*time.Second), addr
}

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

func (c fakeClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.Now().Add(d)
	return ch
}

func TestWakeEndToEndFirstHitWakesSecondIsDirect(t *testing.T) {
	super := &stubSuper{states: map[string]ProcState{
		"api": {Name: "api", Status: StateStopped},
	}}
	mgr, _ := newTestManager(t, super)

	super.mu.Lock()
	super.states["api"] = ProcState{Name: "api", Status: StatePending, IsRunning: false, IsReady: HealthNotReady}
	super.mu.Unlock()

	var wakeOnce sync.Once
	go func() {
		time.Sleep(200 * time.Millisecond)
		super.mu.Lock()
		super.states["api"] = ProcState{Name: "api", Status: StateRunning, IsRunning: true, IsReady: HealthReady, Pid: 4242}
		super.mu.Unlock()
		wakeOnce.Do(func() {})
	}()

	handler := mgr.WakeHandler()
	req := httptest.NewRequest(http.MethodPost, "/wake/api", nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first hit must wake and hold until ready: got %d %s", rec.Code, rec.Body.String())
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("request must be held until ready, returned after %s", elapsed)
	}
	if mgr.Phase("api") != PhaseReady {
		t.Fatalf("phase after wake: %s", mgr.Phase("api"))
	}

	super.mu.Lock()
	started := append([]string(nil), super.started...)
	super.mu.Unlock()
	if len(started) != 1 || started[0] != "api" {
		t.Fatalf("supervisor start called wrong: %v", started)
	}

	if WakeHookAllowed(mgr.Phase("api")) {
		t.Fatal("wake hook must never be inserted once project is up (one-hop guarantee)")
	}
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("second hit on live project: %d", rec2.Code)
	}
	super.mu.Lock()
	totalStarts := len(super.started)
	super.mu.Unlock()
	if totalStarts != 1 {
		t.Fatalf("second request must be direct (no extra start): starts=%d", totalStarts)
	}
}

func TestWakeTimeoutBounded(t *testing.T) {
	super := &stubSuper{states: map[string]ProcState{
		"stuck": {Name: "stuck", Status: StateStopped},
	}}
	mgr, _ := newTestManager(t, super)
	mgr.wakeWait = 300 * time.Millisecond
	handler := mgr.WakeHandler()
	req := httptest.NewRequest(http.MethodPost, "/wake/stuck", nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("unready project must 504: %d", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("wake wait unbounded (I7): %s", elapsed)
	}
}

func TestIdleTrackerFiresExactlyAfterQuietWindow(t *testing.T) {
	base := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	clock := &steppingClock{cur: base}
	window := 10 * time.Minute
	idle := NewIdleTracker(clock, func() time.Duration { return window }, func() time.Duration { return 0 })
	idle.Arm("web")
	if idle.Due("web") {
		t.Fatal("fresh arm must not be due")
	}
	clock.Step(9 * time.Minute)
	if idle.Due("web") {
		t.Fatalf("due at %s < window %s", idle.QuietFor("web"), window)
	}
	clock.Step(1*time.Minute - time.Nanosecond)
	if idle.Due("web") {
		t.Fatal("must not fire one nanosecond early")
	}
	clock.Step(time.Nanosecond)
	if !idle.Due("web") {
		t.Fatalf("must fire exactly at window: quiet=%s", idle.QuietFor("web"))
	}
	idle.Touch("web")
	if idle.Due("web") {
		t.Fatal("touch resets the quiet window")
	}
	clock.Step(window + time.Minute)
	if !idle.Due("web") {
		t.Fatal("fires again after new quiet window")
	}
}

type steppingClock struct {
	cur time.Time
}

func (c *steppingClock) Step(d time.Duration) { c.cur = c.cur.Add(d) }
func (c *steppingClock) Now() time.Time       { return c.cur }
func (c *steppingClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.cur.Add(d)
	return ch
}

func TestBackoffExponentialCapped(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second},
		{20, 30 * time.Second},
	}
	for _, tt := range tests {
		if got := BackoffDelay(tt.attempt); got != tt.want {
			t.Fatalf("attempt %d: got %s want %s", tt.attempt, got, tt.want)
		}
	}
}

func TestClientAgainstStubServer(t *testing.T) {
	super := &stubSuper{states: map[string]ProcState{
		"api": {Name: "api", Status: StateRunning, IsRunning: true, IsReady: HealthReady, Pid: 99},
	}}
	mux := super.handler()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewTCPClient(srv.Listener.Addr().String(), "")
	ctx := context.Background()
	if err := c.Live(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetProcess(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StateRunning || !st.IsRunning || st.IsReady != HealthReady || st.Pid != 99 {
		t.Fatalf("state mismatch: %+v", st)
	}
	all, err := c.ListProcesses(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v %v", all, err)
	}
	if err := c.Start(ctx, "api"); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(ctx, "api"); err != nil {
		t.Fatal(err)
	}
}

func TestUDSClientAgainstUnixSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "pc.sock")
	super := &stubSuper{states: map[string]ProcState{}}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	srv := &http.Server{Handler: super.handler()}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	c := NewUDSClient(sock, "")
	if err := c.Live(context.Background()); err != nil {
		t.Fatalf("uds live: %v", err)
	}
}

func TestSweepStrays(t *testing.T) {
	reaped := 0
	prober := proberFunc(func(addr string) bool {
		return addr == "127.0.0.1:40001"
	})
	aliveFn := func(pid int) bool { return pid == 111 }
	records := []StrayRecord{
		{Project: "ghost", Port: 40001, Pid: 999, Killer: func(pid int) error {
			reaped++
			return nil
		}},
		{Project: "live-owner", Port: 40001, Pid: 111, Killer: func(pid int) error { return nil }},
		{Project: "gone-port", Port: 40002, Pid: 111, Killer: func(pid int) error { return nil }},
		{Project: "no-record", Port: 40003, Pid: 0, Killer: func(pid int) error { return nil }},
	}
	res := SweepStrays(records, prober, aliveFn)
	if !res[0].WasStray || !res[0].Reaped || reaped != 1 {
		t.Fatalf("dead pid on our port must be reaped: %+v reaped=%d", res[0], reaped)
	}
	if res[1].WasStray {
		t.Fatalf("live pid owning port is not a stray: %+v", res[1])
	}
	if res[2].WasStray || res[2].PortInUse {
		t.Fatalf("free port is not a stray: %+v", res[2])
	}
	if res[3].WasStray {
		t.Fatalf("zero pid record handling: %+v", res[3])
	}
}

type proberFunc func(addr string) bool

func (f proberFunc) Listening(addr string) bool { return f(addr) }

func TestAssertUnprivileged(t *testing.T) {
	if err := AssertUnprivileged(); err == nil {
		t.Skip("test running as root; I5 assert untestable here")
	} else if err != ErrRootSpawn {
		t.Fatalf("unexpected error: %v", err)
	}
}
