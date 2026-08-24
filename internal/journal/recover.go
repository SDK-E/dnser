package journal

import (
	"context"
	"fmt"
	"time"
)

func Revert(ctx context.Context, st *Store, p *Plan, reg Registry) ([]StepReport, error) {
	var reports []StepReport
	for i := len(p.Steps) - 1; i >= 0; i-- {
		s := p.Steps[i]
		switch s.Status {
		case StatusReversed:
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusReversed})
			continue
		case StatusPending, StatusInflight, StatusApplied, StatusFailed:
		}
		if s.Capture == nil {
			s.Status = StatusReversed
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusReversed})
			continue
		}
		exec, err := reg.Get(s.Kind)
		if err != nil {
			s.Status = StatusFailed
			s.Error = err.Error()
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: err})
			_ = st.Save(p)
			return reports, err
		}
		err = exec.Invert(ctx, s)
		done := time.Now().UTC()
		if err != nil {
			s.Status = StatusFailed
			s.Error = fmt.Sprintf("revert: %v", err)
			reports = append(reports, StepReport{StepID: s.ID, Status: StatusFailed, Err: err})
			_ = st.Save(p)
			return reports, fmt.Errorf("revert step %s (%s): %w", s.ID, s.Kind, err)
		}
		s.Status = StatusReversed
		s.DoneAt = &done
		s.Error = ""
		reports = append(reports, StepReport{StepID: s.ID, Status: StatusReversed})
		if err := st.Save(p); err != nil {
			return reports, fmt.Errorf("journal revert of %s: %w", s.ID, err)
		}
	}
	p.Normalize()
	return reports, nil
}

func Finish(ctx context.Context, st *Store, p *Plan, reg Registry) ([]StepReport, error) {
	a := &Applier{Registry: reg}
	return a.Apply(ctx, st, p)
}

func HasInterrupted(p *Plan) bool {
	for _, s := range p.Steps {
		if s.Status == StatusInflight || (s.Status == StatusFailed && s.Capture != nil) {
			return true
		}
	}
	return false
}
