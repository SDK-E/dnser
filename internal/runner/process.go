package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Spec struct {
	Domain    string
	Dir       string
	Framework string
	Port      int
	Command   []string
	UseShell  bool
	PortEnv   bool
	Env       []string
}

func (s *Supervisor) supervise(app *managedApp) {
	defer close(app.done)
	for {
		exitErr := s.launch(app)

		app.procMu.Lock()
		app.proc = nil
		stopping := app.stopping
		if exitErr != nil {
			app.lastErr = exitErr.Error()
		}
		app.procMu.Unlock()

		if stopping {
			return
		}

		now := s.clock()
		app.procMu.Lock()
		app.backoff *= 2
		if app.backoff == 0 {
			app.backoff = time.Second
		}
		if app.backoff > 30*time.Second {
			app.backoff = 30 * time.Second
		}
		if now.Sub(app.stableAt) > time.Minute {
			app.backoff = time.Second
		}
		app.state = StateCrashLooping
		restarts := app.restarts
		app.restarts++
		app.procMu.Unlock()

		s.logs.emit(app.domain, fmt.Sprintf("process exited (%v); restarting in %s (restart #%d)", exitErr, app.backoff, restarts+1))

		if !s.sleepBackoff(app) {
			return
		}
	}
}

func (s *Supervisor) sleepBackoff(app *managedApp) bool {
	app.procMu.Lock()
	d := app.backoff
	app.procMu.Unlock()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-app.stopCh:
		return false
	}
}

func (s *Supervisor) launch(app *managedApp) error {
	cmd, err := buildCmd(app)
	if err != nil {
		app.procMu.Lock()
		app.state = StateFailed
		app.lastErr = err.Error()
		app.procMu.Unlock()
		return err
	}

	cmd.Dir = app.dir
	cmd.Env = os.Environ()
	if len(app.env) > 0 {
		cmd.Env = append(cmd.Env, app.env...)
	}
	if app.portEnv {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", app.port))
	}
	cmd.Stdout = s.logs.writerFor(app.domain)
	cmd.Stderr = s.logs.writerFor(app.domain)

	setSysProcAttr(cmd)

	app.procMu.Lock()
	if err := cmd.Start(); err != nil {
		app.procMu.Unlock()
		return fmt.Errorf("start %s: %w", strings.Join(cmd.Args, " "), err)
	}
	app.proc = cmd.Process
	app.state = StateStarting
	app.startedAt = s.clock()
	app.stableAt = app.startedAt
	pid := app.proc.Pid
	app.procMu.Unlock()

	s.logs.emit(app.domain, fmt.Sprintf("started pid %d: %s (port %d)", pid, strings.Join(app.command, " "), app.port))

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	upDone := false
	timeout := time.NewTimer(90 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()

	var exitErr error
waiting:
	for {
		select {
		case err := <-waitCh:
			exitErr = err
			break waiting
		case <-app.stopCh:
			exitErr = <-waitCh
			break waiting
		case <-timeout.C:
			if !upDone {
				s.logs.emit(app.domain, "no listener detected on port yet — leaving it running")
				upDone = true
			}
		case <-tick.C:
			if !upDone && dialPort(app.port) {
				upDone = true
				app.procMu.Lock()
				if app.state == StateStarting {
					app.state = StateUp
					app.stableAt = s.clock()
				}
				app.procMu.Unlock()
				s.logs.emit(app.domain, "listening — project is up")
			}
		}
	}
	return exitErr
}

func (a *managedApp) stop() {
	a.procMu.Lock()
	a.stopping = true
	proc := a.proc
	state := a.state
	a.procMu.Unlock()

	if proc != nil && state != StateStopped {
		killTree(proc)
	}
	a.stopOnce.Do(func() { close(a.stopCh) })
}

func buildCmd(app *managedApp) (*exec.Cmd, error) {
	argv := SubstitutePort(app.command, app.port)
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if app.useShell {
		joined := strings.Join(argv, " ")
		return exec.Command(shellBin(), shellFlag(), joined), nil
	}
	bin := argv[0]
	if !strings.ContainsRune(bin, os.PathSeparator) && !strings.ContainsRune(bin, '/') {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("%q not found in PATH — install it first", bin)
		}
	} else if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("command %q does not exist", bin)
	}
	return exec.Command(bin, argv[1:]...), nil
}

func dialPort(port int) bool {
	conn, err := netDialTimeout(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
