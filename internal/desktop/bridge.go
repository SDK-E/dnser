//go:build desktop

package desktop

type Bridge struct {
	svc *Service
}

func NewBridge(svc *Service) *Bridge {
	return &Bridge{svc: svc}
}

func (b *Bridge) Status() Status {
	return b.svc.Status()
}

func (b *Bridge) IsRunning() bool {
	return b.svc.Running()
}

func (b *Bridge) SetupStatus() SetupStatusView {
	return b.svc.SetupStatus()
}

func (b *Bridge) RunSetup() []SetupStep {
	var steps []SetupStep
	err := b.svc.RunSetup(func(step SetupStep) {
		steps = append(steps, step)
	})
	if err != nil && len(steps) == 0 {
		steps = append(steps, SetupStep{Name: "setup", Err: err.Error()})
	}
	return steps
}

func (b *Bridge) RevertSetup() error {
	return b.svc.RevertSetup()
}

func (b *Bridge) SetAutostart(enabled bool) error {
	return b.svc.SetAutostart(enabled)
}

func (b *Bridge) OpenDashboardExternal() bool {
	st := b.svc.Status()
	if st.DashboardURL == "" {
		return false
	}
	return openInBrowser(st.DashboardURL)
}
