package desktop

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/daemon"
)

type Options struct {
	Store   *config.Store
	Version string
}

type Status struct {
	Running      bool   `json:"running"`
	Version      string `json:"version"`
	TLD          string `json:"tld"`
	DNSPort      int    `json:"dns_port"`
	DashboardURL string `json:"dashboard_url"`
	Projects     int    `json:"projects"`
}

type Service struct {
	opts Options

	mu       sync.Mutex
	rt       *daemon.Runtime
	setupMu  sync.Mutex
	readyFn  func(rt *daemon.Runtime)
	changeFn func()
	upd      UpdateInfo
}

func New(opts Options) (*Service, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("desktop: store required")
	}
	return &Service{opts: opts}, nil
}

func (s *Service) SetReadyHook(fn func(rt *daemon.Runtime)) {
	s.mu.Lock()
	s.readyFn = fn
	rt := s.rt
	current := s.readyFn
	s.mu.Unlock()
	if rt != nil && current != nil {
		current(rt)
	}
}

func (s *Service) SetChangeHook(fn func()) {
	s.mu.Lock()
	s.changeFn = fn
	s.mu.Unlock()
}

func (s *Service) notifyChange() {
	s.mu.Lock()
	fn := s.changeFn
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (s *Service) Start() error {
	s.mu.Lock()
	if s.rt != nil {
		s.mu.Unlock()
		return fmt.Errorf("desktop: already running")
	}
	rt, err := daemon.New(daemon.Options{
		Store:   s.opts.Store,
		Version: s.opts.Version,
	})
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("desktop: start daemon: %w", err)
	}
	s.rt = rt
	ready := s.readyFn
	s.mu.Unlock()

	if ready != nil {
		ready(rt)
	}
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	rt := s.rt
	s.rt = nil
	s.mu.Unlock()
	if rt == nil {
		return nil
	}
	if err := rt.Shutdown(ctx); err != nil {
		return fmt.Errorf("desktop: stop daemon: %w", err)
	}
	return nil
}

func (s *Service) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt != nil
}

func (s *Service) Runtime() *daemon.Runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt
}

func (s *Service) APIHandler() http.Handler {
	rt := s.Runtime()
	if rt == nil {
		return nil
	}
	return rt.APIHandler()
}

func (s *Service) Status() Status {
	st := Status{
		Running:  s.Running(),
		Version:  s.opts.Version,
		TLD:      s.opts.Store.Settings().TLD,
		Projects: len(s.opts.Store.Projects()),
	}
	if rt := s.Runtime(); rt != nil {
		st.DNSPort = rt.DNSPort()
		st.DashboardURL = rt.DashboardURL()
	}
	return st
}
