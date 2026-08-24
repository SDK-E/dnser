package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SDK-E/dnser/internal/logstream"
)

type State string

const (
	StateStarting     State = "starting"
	StateUp           State = "up"
	StateCrashLooping State = "crash-looping"
	StateStopped      State = "stopped"
	StateDepsMissing  State = "deps-missing"
	StateFailed       State = "failed"
)

type AppInfo struct {
	Domain    string    `json:"domain"`
	Path      string    `json:"path"`
	Framework string    `json:"framework"`
	State     State     `json:"state"`
	Port      int       `json:"port"`
	PID       int       `json:"pid,omitempty"`
	Command   []string  `json:"command,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	Restarts  int       `json:"restarts"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

type Supervisor struct {
	mu    sync.RWMutex
	apps  map[string]*managedApp
	logs  *logTee
	clock func() time.Time
	paths *PathResolver
}

type managedApp struct {
	domain     string
	dir        string
	framework  string
	port       int
	command    []string
	useShell   bool
	portEnv    bool
	namedPorts map[string]int

	proc      *os.Process
	procMu    sync.Mutex
	state     State
	lastErr   string
	restarts  int
	startedAt time.Time
	stableAt  time.Time
	backoff   time.Duration
	stopping  bool
	done      chan struct{}
	stopCh    chan struct{}
	stopOnce  sync.Once
	env       []string
}

type Options struct {
	LogsDir     string
	Stream      *logstream.Stream
	Clock       func() time.Time
	UserHome    string
	DnserHome   string
	PathRefresh time.Duration
}

func NewSupervisor(opts Options) *Supervisor {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &Supervisor{
		apps:  make(map[string]*managedApp),
		logs:  newLogTee(opts.LogsDir, opts.Stream),
		clock: opts.Clock,
		paths: NewPathResolver(pathOptionsFor(opts)),
	}
}

func pathOptionsFor(opts Options) PathOptions {
	po := PathOptions{UserHome: opts.UserHome, TTL: opts.PathRefresh, Clock: opts.Clock}
	switch {
	case opts.DnserHome != "":
		po.DnserHome = opts.DnserHome
	case opts.LogsDir != "":
		po.DnserHome = filepath.Dir(filepath.Clean(opts.LogsDir))
	}
	return po
}

func (s *Supervisor) Info() map[string]AppInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]AppInfo, len(s.apps))
	for domain, app := range s.apps {
		app.procMu.Lock()
		info := AppInfo{
			Domain:    app.domain,
			Path:      app.dir,
			Framework: app.framework,
			State:     app.state,
			Port:      app.port,
			LastError: app.lastErr,
			Restarts:  app.restarts,
			StartedAt: app.startedAt,
			Command:   app.command,
		}
		if p := app.proc; p != nil {
			info.PID = p.Pid
		}
		app.procMu.Unlock()
		out[domain] = info
	}
	return out
}

func (s *Supervisor) Get(domain string) (AppInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.apps[domain]
	if !ok {
		return AppInfo{}, false
	}
	app.procMu.Lock()
	defer app.procMu.Unlock()
	info := AppInfo{
		Domain:    app.domain,
		Path:      app.dir,
		Framework: app.framework,
		State:     app.state,
		Port:      app.port,
		LastError: app.lastErr,
		Restarts:  app.restarts,
		StartedAt: app.startedAt,
		Command:   app.command,
	}
	if p := app.proc; p != nil {
		info.PID = p.Pid
	}
	return info, true
}

func (s *Supervisor) Start(spec Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, exists := s.apps[spec.Domain]
	if exists {
		old.stop()
		delete(s.apps, spec.Domain)
	}
	app := &managedApp{
		domain:     spec.Domain,
		dir:        spec.Dir,
		framework:  spec.Framework,
		port:       spec.Port,
		command:    spec.Command,
		useShell:   spec.UseShell,
		portEnv:    spec.PortEnv,
		namedPorts: spec.NamedPorts,
		state:      StateStarting,
		done:       make(chan struct{}),
		stopCh:     make(chan struct{}),
		env:        spec.Env,
	}
	s.apps[app.domain] = app
	go s.supervise(app)
	return nil
}

func (s *Supervisor) Stop(domain string) bool {
	s.mu.RLock()
	app, ok := s.apps[domain]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	app.stop()
	s.mu.Lock()
	app.procMu.Lock()
	app.state = StateStopped
	app.lastErr = ""
	app.procMu.Unlock()
	s.mu.Unlock()
	return true
}

func (s *Supervisor) Restart(domain string) error {
	s.mu.Lock()
	app, ok := s.apps[domain]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("project %q is not managed", domain)
	}
	spec := Spec{
		Domain:     app.domain,
		Dir:        app.dir,
		Framework:  app.framework,
		Port:       app.port,
		Command:    app.command,
		UseShell:   app.useShell,
		PortEnv:    app.portEnv,
		NamedPorts: app.namedPorts,
	}
	return s.Start(spec)
}

func (s *Supervisor) Remove(domain string) {
	s.Stop(domain)
	s.mu.Lock()
	delete(s.apps, domain)
	s.mu.Unlock()
}

func (s *Supervisor) Shutdown() {
	var wg sync.WaitGroup
	for _, app := range s.snapshotApps() {
		wg.Add(1)
		go func(a *managedApp) {
			a.stop()
			wg.Done()
		}(app)
	}
	wg.Wait()
	s.logs.closeAll()
}

func (s *Supervisor) snapshotApps() []*managedApp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*managedApp, 0, len(s.apps))
	for _, a := range s.apps {
		out = append(out, a)
	}
	return out
}

func (s *Supervisor) EffectivePATHDirs() []string {
	if s.paths == nil {
		return nil
	}
	return append([]string(nil), s.paths.Dirs()...)
}
