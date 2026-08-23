package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/SDK-E/dnser/internal/certs"
	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/setup"
)

type SetupStep struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Err    string `json:"err,omitempty"`
}

type SetupStatusView struct {
	CATrusted       bool     `json:"ca_trusted"`
	CATrustMode     string   `json:"ca_trust_mode,omitempty"`
	Routed          bool     `json:"routed"`
	RoutingMode     string   `json:"routing_mode"`
	ResolverDomains []string `json:"resolver_domains,omitempty"`
	DNSPort         int      `json:"dns_port"`
	NeedsPort53     bool     `json:"needs_port_53"`
}

func (s *Service) RunSetup(progress func(SetupStep)) error {
	if !s.setupMu.TryLock() {
		return fmt.Errorf("desktop: setup already running")
	}
	defer s.setupMu.Unlock()

	report := func(step SetupStep) {
		if progress != nil {
			progress(step)
		}
		if step.Err != "" {
			slog.Warn("setup step failed", "step", step.Name, "err", step.Err)
		} else {
			slog.Info("setup step done", "step", step.Name, "detail", step.Detail)
		}
	}

	dir := s.homeDir()
	cfg := s.opts.Store.Settings()
	state, err := setup.LoadState(dir)
	if err != nil {
		return err
	}

	if err := s.ensureRunning(); err != nil {
		return err
	}

	caPEM, caErr := s.ensureCA(dir)
	stepCA := SetupStep{Name: "certificate-authority"}
	if caErr != nil {
		stepCA.Err = caErr.Error()
		report(stepCA)
		return fmt.Errorf("init CA: %w", caErr)
	}
	stepCA.Detail = "local root CA ready"
	report(stepCA)

	if !state.CATrusted {
		path, mode, terr := setup.TrustCA(setup.SystemRunner(), caPEM, dir)
		stepTrust := SetupStep{Name: "trust-ca"}
		if terr != nil {
			stepTrust.Err = terr.Error()
			report(stepTrust)
		} else {
			state.CATrusted = true
			state.CAInstallPath = path
			state.CATrustMode = mode
			stepTrust.Detail = "trusted via " + mode
			report(stepTrust)
		}
	} else {
		report(SetupStep{Name: "trust-ca", Detail: "already trusted"})
	}

	rt := s.Runtime()
	exePath, _ := os.Executable()

	if !routedAlready(cfg.TLD, state) {
		runRouting(s, cfg, rt.DNSPort(), exePath, state, report)
	} else {
		report(SetupStep{Name: "route-dns", Detail: routingMode(state)})
	}

	if err := setup.SaveState(dir, state); err != nil {
		return fmt.Errorf("save setup state: %w", err)
	}
	s.notifyChange()
	return nil
}

func runRouting(s *Service, cfg config.Settings, dnsPort int, exePath string, state *setup.State, report func(SetupStep)) {
	step := SetupStep{Name: "route-dns"}
	done := func(err error) {
		if err != nil {
			step.Err = err.Error()
		} else {
			step.Detail = routingMode(state)
		}
		report(step)
	}
	if !elevateAvailable() {
		done(fmt.Errorf("no elevation mechanism available on %s", runtime.GOOS))
		return
	}
	if dnsPort == 53 || !routingNeedsPort53() {
		done(commitRouting(cfg.TLD, cfg.Bind, dnsPort, state))
		return
	}
	if err := prepareRouting(exePath, state); err != nil {
		done(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()
	if err := s.restart(ctx); err != nil {
		done(err)
		return
	}
	rt := s.Runtime()
	if rt.DNSPort() != 53 {
		done(fmt.Errorf("port 53 still unavailable after elevation; another service may own it"))
		return
	}
	done(commitRouting(cfg.TLD, cfg.Bind, rt.DNSPort(), state))
}

func (s *Service) ensureRunning() error {
	if s.Running() {
		return nil
	}
	return s.Start()
}

func (s *Service) restart(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return fmt.Errorf("restart daemon: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("restart daemon: %w", err)
	}
	return nil
}

func (s *Service) ensureCA(dir string) ([]byte, error) {
	ca, err := certs.NewCA(filepath.Join(dir, "certs"))
	if err != nil {
		return nil, err
	}
	return ca.CertificatePEM(), nil
}

func (s *Service) homeDir() string {
	return filepath.Dir(s.opts.Store.Path())
}

func (s *Service) SetupStatus() SetupStatusView {
	dir := s.homeDir()
	view := SetupStatusView{RoutingMode: "none", NeedsPort53: routingNeedsPort53()}
	state, err := setup.LoadState(dir)
	if err == nil {
		view.CATrusted = state.CATrusted
		view.CATrustMode = state.CATrustMode
		view.ResolverDomains = state.ResolverDomains
		view.RoutingMode = routingMode(state)
		view.Routed = view.RoutingMode != "none"
	}
	if rt := s.Runtime(); rt != nil {
		view.DNSPort = rt.DNSPort()
	}
	return view
}

func (s *Service) RevertSetup() error {
	dir := s.homeDir()
	state, err := setup.LoadState(dir)
	if err != nil {
		return fmt.Errorf("load setup state: %w", err)
	}
	reverted := false
	if state.DNSApplied {
		if err := setup.RestoreDNS(newElevatedRunner(), state.DNSServices); err != nil {
			return fmt.Errorf("restore system resolver: %w", err)
		}
		state.DNSApplied = false
		state.DNSServices = nil
		reverted = true
		slog.Info("restored system resolver settings")
	}
	if len(state.ResolverDomains) > 0 || state.CapGranted {
		if err := setup.RevertDesktopState(setup.SystemRunner(), state); err != nil {
			return fmt.Errorf("remove desktop routing: %w", err)
		}
		state.ResolverDomains = nil
		state.CapGranted = false
		reverted = true
		slog.Info("removed desktop resolver routing")
	}
	if !reverted {
		return nil
	}
	s.notifyChange()
	return setup.SaveState(dir, state)
}

func (s *Service) SetAutostart(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	if err := setAutostart(exe, enabled); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

func routingMode(st *setup.State) string {
	switch {
	case len(st.ResolverDomains) > 0:
		return "resolver-files"
	case st.DNSApplied:
		return "system-resolver"
	default:
		return "none"
	}
}

const restartTimeout = 15 * time.Second
