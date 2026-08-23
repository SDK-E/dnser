package desktop

import (
	"context"
	"log/slog"

	"github.com/SDK-E/dnser/internal/update"
)

type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	URL       string `json:"url,omitempty"`
}

func (s *Service) SetUpdateInfo(info UpdateInfo) {
	s.mu.Lock()
	s.upd = info
	s.mu.Unlock()
	s.notifyChange()
}

func (s *Service) Update() UpdateInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upd
}

func (s *Service) CheckNow(ctx context.Context) error {
	rel, err := update.Check(ctx, s.opts.Version)
	if err != nil {
		return err
	}
	if rel == nil {
		s.SetUpdateInfo(UpdateInfo{})
		return nil
	}
	slog.Info("update available", "current", s.opts.Version, "latest", rel.Version, "url", rel.URL)
	s.SetUpdateInfo(UpdateInfo{Available: true, Version: rel.Version, URL: rel.URL})
	return nil
}
