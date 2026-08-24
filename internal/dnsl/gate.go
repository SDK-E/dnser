package dnsl

import (
	"context"
	"fmt"
)

func EnsureResolvers(ctx context.Context, l *Listener, w ResolverWriter, entries []Entry) ([]Entry, []string, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	if err := l.Probe(); err != nil {
		return nil, nil, fmt.Errorf("resolver write gated: listener not answering (I1): %w", err)
	}
	applied, err := w.Apply(entries)
	if err != nil {
		return applied, nil, err
	}
	drifted, err := w.Verify(entries)
	if err != nil {
		return applied, drifted, err
	}
	return applied, drifted, nil
}
