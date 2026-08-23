//go:build desktop

package desktop

import (
	"context"
	"log/slog"
	"time"
)

const (
	updateStartupDelay = 30 * time.Second
	updateInterval     = 6 * time.Hour
)

func (s *Service) runUpdateLoop(ctx context.Context) {
	time.Sleep(updateStartupDelay)
	for {
		if err := s.CheckNow(ctx); err != nil {
			slog.Debug("update check failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(updateInterval):
		}
	}
}
