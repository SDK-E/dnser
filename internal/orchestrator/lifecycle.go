package orchestrator

import (
	"fmt"
	"time"
)

type Phase string

const (
	PhaseStopped  Phase = "stopped"
	PhaseStarting Phase = "starting"
	PhaseReady    Phase = "ready"
	PhaseRunning  Phase = "running"
	PhaseStopping Phase = "stopping"
	PhaseBackoff  Phase = "backoff"
)

type Event string

const (
	EventStartRequested Event = "start_requested"
	EventStopRequested  Event = "stop_requested"
	EventSuperRunning   Event = "super_running"
	EventSuperReady     Event = "super_ready"
	EventSuperExited    Event = "super_exited"
	EventIdleFired      Event = "idle_fired"
	EventBackoffElapsed Event = "backoff_elapsed"
)

type Action string

const (
	ActionSupervisorStart Action = "supervisor_start"
	ActionSupervisorStop  Action = "supervisor_stop"
	ActionNone            Action = ""
)

var ErrInvalidTransition = fmt.Errorf("invalid lifecycle transition")

type transition struct {
	next   Phase
	action Action
}

func transitions() map[Phase]map[Event]transition {
	return map[Phase]map[Event]transition{
		PhaseStopped: {
			EventStartRequested: {next: PhaseStarting, action: ActionSupervisorStart},
			EventBackoffElapsed: {next: PhaseStopped, action: ActionNone},
			EventStopRequested:  {next: PhaseStopped, action: ActionNone},
		},
		PhaseStarting: {
			EventSuperReady:    {next: PhaseReady, action: ActionNone},
			EventSuperRunning:  {next: PhaseRunning, action: ActionNone},
			EventStopRequested: {next: PhaseStopping, action: ActionSupervisorStop},
			EventSuperExited:   {next: PhaseStopped, action: ActionNone},
		},
		PhaseReady: {
			EventStopRequested: {next: PhaseStopping, action: ActionSupervisorStop},
			EventSuperExited:   {next: PhaseStopped, action: ActionNone},
			EventIdleFired:     {next: PhaseStopping, action: ActionSupervisorStop},
		},
		PhaseRunning: {
			EventStopRequested: {next: PhaseStopping, action: ActionSupervisorStop},
			EventSuperReady:    {next: PhaseReady, action: ActionNone},
			EventSuperExited:   {next: PhaseStopped, action: ActionNone},
			EventIdleFired:     {next: PhaseStopping, action: ActionSupervisorStop},
		},
		PhaseStopping: {
			EventSuperExited: {next: PhaseStopped, action: ActionNone},
		},
		PhaseBackoff: {
			EventBackoffElapsed: {next: PhaseStarting, action: ActionSupervisorStart},
			EventStopRequested:  {next: PhaseStopped, action: ActionNone},
		},
	}
}

type Lifecycle struct {
	Project   string
	Phase     Phase
	StartedAt time.Time
	Crashes   int
}

func NewLifecycle(project string) *Lifecycle {
	return &Lifecycle{Project: project, Phase: PhaseStopped}
}

func (l *Lifecycle) Send(ev Event, now time.Time) (Action, error) {
	table, ok := transitions()[l.Phase]
	if !ok {
		return ActionNone, fmt.Errorf("%w: no table for phase %s", ErrInvalidTransition, l.Phase)
	}
	t, ok := table[ev]
	if !ok {
		return ActionNone, fmt.Errorf("%w: %s × %s", ErrInvalidTransition, l.Phase, ev)
	}
	prev := l.Phase
	l.Phase = t.next
	switch {
	case prev == PhaseStopped && t.next == PhaseStarting:
		l.StartedAt = now
	case t.next == PhaseStopped:
		l.Crashes = 0
	}
	return t.action, nil
}

func (l *Lifecycle) Uptime(now time.Time) time.Duration {
	if l.StartedAt.IsZero() || l.Phase == PhaseStopped {
		return 0
	}
	return now.Sub(l.StartedAt)
}

func BackoffDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}

func WakeHookAllowed(phase Phase) bool {
	return phase == PhaseStopped || phase == PhaseBackoff
}
