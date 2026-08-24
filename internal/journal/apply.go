package journal

import (
	"context"
	"fmt"
	"time"
)

type Executor interface {
	Capture(ctx context.Context, s *Step) (*Capture, error)
	Apply(ctx context.Context, s *Step) error
	Invert(ctx context.Context, s *Step) error
	Verify(ctx context.Context, s *Step) error
}

type Registry map[StepKind]Executor

func (r Registry) Get(kind StepKind) (Executor, error) {
	e, ok := r[kind]
	if !ok {
		return nil, fmt.Errorf("no executor registered for kind %q", kind)
	}
	return e, nil
}

type StepReport struct {
	StepID string
	Status StepStatus
	Err    error
}

type Applier struct {
	Registry Registry
	Timeout  time.Duration
}

const DefaultStepTimeout = 30 * time.Second

func (a *Applier) Apply(ctx context.Context, st *Store, p *Plan) ([]StepReport, error) {
	var reports []StepReport
	for _, s := range p.Steps {
		if s.Status == StatusApplied {
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusApplied})
			continue
		}
		exec, err := a.Registry.Get(s.Kind)
		if err != nil {
			s.Status = StatusFailed
			s.Error = err.Error()
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: err})
			_ = st.Save(p)
			return reports, err
		}
		stepCtx, cancel := a.stepContext(ctx)
		cap, capErr := exec.Capture(stepCtx, s)
		cancel()
		if capErr != nil {
			s.Status = StatusFailed
			s.Error = fmt.Sprintf("capture: %v", capErr)
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: capErr})
			_ = st.Save(p)
			return reports, fmt.Errorf("capture %s: %w", s.ID, capErr)
		}
		s.Capture = cap
		s.Status = StatusInflight
		s.Error = ""
		if err := st.Save(p); err != nil {
			return reports, fmt.Errorf("write-ahead journal for %s: %w", s.ID, err)
		}
		stepCtx, cancel = a.stepContext(ctx)
		err = exec.Apply(stepCtx, s)
		done := time.Now().UTC()
		cancel()
		if err != nil {
			s.Status = StatusFailed
			s.Error = err.Error()
			s.DoneAt = &done
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: err})
			_ = st.Save(p)
			return reports, fmt.Errorf("apply step %s (%s): %w", s.ID, s.Kind, err)
		}
		vCtx, vCancel := a.stepContext(ctx)
		vErr := exec.Verify(vCtx, s)
		vCancel()
		if vErr != nil {
			s.Status = StatusFailed
			s.Error = fmt.Sprintf("verify: %v", vErr)
			s.DoneAt = &done
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: vErr})
			_ = st.Save(p)
			return reports, fmt.Errorf("verify step %s: %w", s.ID, vErr)
		}
		s.Status = StatusApplied
		s.DoneAt = &done
		reports = append(reports, StepReport{StepID: s.ID, Status: StatusApplied})
		if err := st.Save(p); err != nil {
			return reports, fmt.Errorf("journal applied step %s: %w", s.ID, err)
		}
	}
	return reports, nil
}

func (a *Applier) stepContext(parent context.Context) (context.Context, context.CancelFunc) {
	t := a.Timeout
	if t <= 0 {
		t = DefaultStepTimeout
	}
	return context.WithTimeout(parent, t)
}
