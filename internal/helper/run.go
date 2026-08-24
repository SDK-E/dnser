package helper

import (
	"context"
	"fmt"

	"github.com/SDK-E/dnser/internal/journal"
)

func ApplyPlanFile(ctx context.Context, planPath string) (*journal.Plan, []journal.StepReport, error) {
	p, err := journal.LoadPlanFrom(planPath)
	if err != nil {
		return nil, nil, err
	}
	reg := journal.NewFullRegistry(RealRunner(), CATrustOps(), ServiceOps())
	st, err := journal.OpenStore(".")
	if err != nil {
		return nil, nil, err
	}
	ap := &journal.Applier{Registry: reg}
	reports, applyErr := ap.Apply(ctx, st, p)
	return p, reports, applyErr
}

type VerifyResult struct {
	StepID string
	OK     bool
	Detail string
}

func RegistryForCLI() journal.Registry {
	return registryForVerify()
}

func registryForVerify() journal.Registry {
	reg := journal.NewFullRegistry(RealRunner(), CATrustOps(), ServiceOps())
	fsReg := journal.NewFSRegistry()
	merged := journal.Registry{}
	for kind, exec := range fsReg {
		merged[kind] = exec
	}
	for kind, exec := range reg {
		merged[kind] = exec
	}
	return merged
}

func VerifyPlan(ctx context.Context, p *journal.Plan) ([]VerifyResult, bool) {
	reg := registryForVerify()
	var out []VerifyResult
	allOK := true
	for _, s := range p.Steps {
		exec, gerr := reg.Get(s.Kind)
		if gerr != nil {
			out = append(out, VerifyResult{StepID: s.ID, OK: false, Detail: gerr.Error()})
			allOK = false
			continue
		}
		err := exec.Verify(ctx, s)
		ok := err == nil
		detail := "ok"
		if !ok {
			detail = err.Error()
			allOK = false
		}
		out = append(out, VerifyResult{StepID: s.ID, OK: ok, Detail: detail})
	}
	return out, allOK
}

func AlreadyApplied(ctx context.Context, p *journal.Plan) bool {
	_, ok := VerifyPlan(ctx, p)
	return ok
}

func RevertPlan(ctx context.Context, st *journal.Store, p *journal.Plan) ([]journal.StepReport, error) {
	return journal.Revert(ctx, st, p, registryForVerify())
}

func FormatReports(reports []journal.StepReport) string {
	out := ""
	for _, r := range reports {
		mark := "+"
		switch r.Status {
		case journal.StatusFailed:
			mark = "x"
		case journal.StatusReversed:
			mark = "-"
		case journal.StatusPending:
			mark = "…"
		}
		line := fmt.Sprintf("  %s %s", mark, r.StepID)
		if r.Err != nil {
			line += ": " + r.Err.Error()
		}
		out += line + "\n"
	}
	return out
}
