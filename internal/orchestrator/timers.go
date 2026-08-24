package orchestrator

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func RealClock() Clock { return realClock{} }

type idleEntry struct {
	lastActivity time.Time
}

type IdleTracker struct {
	mu        sync.Mutex
	clock     Clock
	window    func() time.Duration
	minUptime func() time.Duration
	idle      map[string]*idleEntry
}

func NewIdleTracker(clock Clock, window, minUptime func() time.Duration) *IdleTracker {
	return &IdleTracker{
		clock:     clock,
		window:    window,
		minUptime: minUptime,
		idle:      map[string]*idleEntry{},
	}
}

func (t *IdleTracker) Arm(project string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.idle[project] = &idleEntry{lastActivity: t.clock.Now()}
}

func (t *IdleTracker) Touch(project string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.idle[project]; ok {
		e.lastActivity = t.clock.Now()
	}
}

func (t *IdleTracker) Disarm(project string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.idle, project)
}

func (t *IdleTracker) QuietFor(project string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.idle[project]
	if !ok {
		return 0
	}
	return t.clock.Now().Sub(e.lastActivity)
}

func (t *IdleTracker) Due(project string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.idle[project]
	if !ok {
		return false
	}
	return t.clock.Now().Sub(e.lastActivity) >= t.window()
}
