package dnsl

import (
	"context"
	"fmt"
	"time"
)

type Watchdog struct {
	Interval      time.Duration
	FailThreshold int
	Probe         func() error
	OnDead        func()
}

func (w *Watchdog) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	threshold := w.FailThreshold
	if threshold <= 0 {
		threshold = 3
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Probe(); err != nil {
				failures++
				if failures == threshold {
					w.OnDead()
				}
				continue
			}
			failures = 0
		}
	}
}

func (w *Watchdog) String() string {
	return fmt.Sprintf("watchdog(interval=%s,threshold=%d)", w.Interval, w.FailThreshold)
}
