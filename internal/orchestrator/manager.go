package orchestrator

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	mu         sync.Mutex
	lifecycles map[string]*Lifecycle
	super      *Client
	idle       *IdleTracker
	wakeWait   time.Duration
}

func NewManager(super *Client, idle *IdleTracker, wakeWait time.Duration) *Manager {
	if wakeWait <= 0 {
		wakeWait = 30 * time.Second
	}
	return &Manager{
		lifecycles: map[string]*Lifecycle{},
		super:      super,
		idle:       idle,
		wakeWait:   wakeWait,
	}
}

func (m *Manager) lifecycle(project string) *Lifecycle {
	lc, ok := m.lifecycles[project]
	if !ok {
		lc = NewLifecycle(project)
		m.lifecycles[project] = lc
	}
	return lc
}

func (m *Manager) Phase(project string) Phase {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lifecycle(project).Phase
}

func (m *Manager) Start(ctx context.Context, project string, now time.Time) error {
	m.mu.Lock()
	lc := m.lifecycle(project)
	act, err := lc.Send(EventStartRequested, now)
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("start %s: %w", project, err)
	}
	if act == ActionSupervisorStart {
		if err := m.super.Start(ctx, project); err != nil {
			return fmt.Errorf("supervisor start %s: %w", project, err)
		}
	}
	if m.idle != nil {
		m.idle.Arm(project)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context, project string, now time.Time) error {
	m.mu.Lock()
	lc := m.lifecycle(project)
	act, err := lc.Send(EventStopRequested, now)
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("stop %s: %w", project, err)
	}
	if act == ActionSupervisorStop {
		if err := m.super.Stop(ctx, project); err != nil {
			return err
		}
	}
	if m.idle != nil {
		m.idle.Disarm(project)
	}
	return nil
}

func (m *Manager) Observe(project string, ev Event, now time.Time) (Action, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lifecycle(project).Send(ev, now)
}

func (m *Manager) SyncFromSupervisor(ctx context.Context) error {
	states, err := m.super.ListProcesses(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for name, st := range states {
		lc := m.lifecycle(name)
		switch {
		case st.IsRunning && st.IsReady == HealthReady && lc.Phase == PhaseStarting:
			_, _ = lc.Send(EventSuperReady, now)
		case st.IsRunning && st.IsReady != HealthReady && lc.Phase == PhaseStarting:
			_, _ = lc.Send(EventSuperRunning, now)
		case !st.IsRunning && (lc.Phase == PhaseRunning || lc.Phase == PhaseReady || lc.Phase == PhaseStopping || lc.Phase == PhaseStarting):
			_, _ = lc.Send(EventSuperExited, now)
		}
	}
	return nil
}

func (m *Manager) WakeAndHold(ctx context.Context, project string, now time.Time) error {
	if err := m.Start(ctx, project, now); err != nil {
		return err
	}
	deadline := time.Now().Add(m.wakeWait)
	for {
		st, err := m.super.GetProcess(ctx, project)
		if err == nil && st.IsReady == HealthReady {
			if _, oerr := m.Observe(project, EventSuperReady, time.Now()); oerr != nil {
				return fmt.Errorf("wake %s: %w", project, oerr)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wake %s: not ready within %s", project, m.wakeWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (m *Manager) WakeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/wake/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "project name required", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		phase := m.lifecycle(name).Phase
		m.mu.Unlock()
		if !WakeHookAllowed(phase) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), m.wakeWait+5*time.Second)
		defer cancel()
		if err := m.WakeAndHold(ctx, name, time.Now()); err != nil {
			http.Error(w, "wake failed: "+err.Error(), http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("X-DNSer-Woke", name)
		w.Header().Set("Retry-After", strconv.Itoa(0))
		w.WriteHeader(http.StatusNoContent)
	})
}
