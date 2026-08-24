package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Spec struct {
	Domain     string
	Dir        string
	Framework  string
	Port       int
	Command    []string
	UseShell   bool
	PortEnv    bool
	Env        []string
	NamedPorts map[string]int
}

func (s *Supervisor) supervise(app *managedApp) {
	defer close(app.done)
	for {
		exitErr := s.launch(app)

		app.procMu.Lock()
		app.proc = nil
		stopping := app.stopping
		if exitErr != nil {
			app.lastErr = annotateExitErr(exitErr, app.command, app.useShell).Error()
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
	cmd, err := s.buildCmd(app)
	if err != nil {
		app.procMu.Lock()
		app.state = StateFailed
		app.lastErr = err.Error()
		app.procMu.Unlock()
		return err
	}

	cmd.Dir = app.dir
	cmd.Env = s.childEnv(app)
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

func buildCmd(app *managedApp, dirs []string) (*exec.Cmd, error) {
	argv := SubstitutePortMap(app.command, app.port, app.namedPorts)
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if app.useShell {
		joined := strings.Join(argv, " ")
		return exec.Command(shellBin(), shellFlag(), joined), nil
	}
	bin := argv[0]
	if !strings.ContainsRune(bin, os.PathSeparator) && !strings.ContainsRune(bin, '/') {
		if _, err := lookPathIn(dirs, bin); err != nil {
			return nil, fmt.Errorf("%q not found in daemon PATH — install it or extend PATH; run 'dnser doctor'", bin)
		}
	} else if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("command %q does not exist", bin)
	}
	return exec.Command(bin, argv[1:]...), nil
}

func (s *Supervisor) buildCmd(app *managedApp) (*exec.Cmd, error) {
	var dirs []string
	if s != nil && s.paths != nil {
		dirs = s.paths.Dirs()
	}
	return buildCmd(app, dirs)
}

func (s *Supervisor) childEnv(app *managedApp) []string {
	augmented := os.Getenv("PATH")
	if s != nil && s.paths != nil {
		augmented = s.paths.String()
	}
	return childEnv(app, augmented)
}

func lookPathIn(dirs []string, bin string) (string, error) {
	for _, dir := range dirs {
		candidate := filepath.Join(dir, bin)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && isExecutable(info) {
			return candidate, nil
		}
	}
	if strings.ContainsRune(bin, os.PathSeparator) || filepath.IsAbs(bin) {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}
	return "", fmt.Errorf("%q not found in %d PATH directories", bin, len(dirs))
}

func ResolveCommandPath(dirs []string, bin string) (string, error) {
	if bin == "" {
		return "", errors.New("empty command")
	}
	if strings.ContainsRune(bin, os.PathSeparator) || strings.ContainsRune(bin, '/') || filepath.IsAbs(bin) {
		info, err := os.Stat(bin)
		if err != nil {
			return "", fmt.Errorf("%q does not exist", bin)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%q is a directory", bin)
		}
		return bin, nil
	}
	return lookPathIn(dirs, bin)
}

func CommandBinary(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	segment := command
	if i := strings.LastIndexAny(segment, ";&|"); i >= 0 {
		segment = segment[i+1:]
	}
	fields := tokenize(segment)
	for len(fields) > 1 && isEnvAssignment(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	return unquoteShell(fields[0])
}

func tokenize(s string) []string {
	var out []string
	cur := strings.Builder{}
	inSingle, inDouble := false, false
	for _, r := range s {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			cur.WriteRune(r)
		case r == '"' && !inSingle:
			inDouble = !inDouble
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isEnvAssignment(field string) bool {
	key, _, ok := strings.Cut(field, "=")
	return ok && key != "" && !strings.ContainsAny(key, "\"'$/(") && isNameLike(key)
}

func isNameLike(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func unquoteShell(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func childEnv(app *managedApp, augmentedPATH string) []string {
	env := make([]string, 0, len(os.Environ())+len(app.env)+2)
	pathKey := "PATH"
	if runtime.GOOS == "windows" {
		pathKey = "Path"
	}
	replaced := false
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(key, pathKey) {
			env = append(env, pathKey+"="+augmentedPATH)
			replaced = true
			continue
		}
		env = append(env, kv)
	}
	if !replaced && augmentedPATH != "" {
		env = append(env, pathKey+"="+augmentedPATH)
	}
	env = append(env, app.env...)
	if app.portEnv {
		env = append(env, fmt.Sprintf("PORT=%d", app.port))
	}
	return env
}

func annotateExitErr(err error, command []string, useShell bool) error {
	if err == nil {
		return err
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || runtime.GOOS == "windows" {
		return err
	}
	if exitErr.ExitCode() != 127 || !useShell || len(command) == 0 {
		return err
	}
	bin := SubstitutePort(command[:1], 0)[0]
	return fmt.Errorf("%w — %q not found in daemon PATH; run 'dnser doctor' for details", err, bin)
}

func dialPort(port int) bool {
	conn, err := netDialTimeout(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
