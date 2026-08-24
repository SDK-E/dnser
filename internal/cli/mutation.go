package cli

import (
	"context"
	"fmt"

	"github.com/SDK-E/dnser/internal/journal"
)

type MutationWrite struct {
	Path    string
	Content []byte
	Mode    uint32
}

func applyMutation(ctx context.Context, intent string, writes []MutationWrite) (*journal.Plan, error) {
	if len(writes) == 0 {
		return nil, fmt.Errorf("empty mutation: nothing to write")
	}
	store, err := openUserStore()
	if err != nil {
		return nil, err
	}
	plan := journal.NewPlan(intent)
	for _, w := range writes {
		plan.Steps = append(plan.Steps, &journal.Step{
			ID:   "write-" + baseOf(w.Path),
			Kind: journal.KindFileWrite,
			Params: map[string]any{
				"path":    w.Path,
				"content": string(w.Content),
				"mode":    w.Mode,
			},
			Status: journal.StatusPending,
		})
	}
	ap := &journal.Applier{Registry: journal.NewFSRegistry()}
	reports, err := ap.Apply(ctx, store, plan)
	for _, r := range reports {
		if r.Err != nil {
			return plan, fmt.Errorf("mutation step %s failed: %w (resume with: dnser journal finish %s || dnser journal revert %s)", r.StepID, r.Err, plan.ID, plan.ID)
		}
	}
	return plan, err
}

func baseOf(p string) string {
	last := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			last = i + 1
		}
	}
	return p[last:]
}
