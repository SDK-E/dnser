package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type State struct {
	DNSServices      map[string][]string `json:"dns_services,omitempty"`
	DNSApplied       bool                `json:"dns_applied,omitempty"`
	ResolvBackup     string              `json:"resolv_backup,omitempty"`
	ServiceMode      string              `json:"service_mode,omitempty"`
	CATrustMode      string              `json:"ca_trust_mode,omitempty"`
	CATrusted        bool                `json:"ca_trusted,omitempty"`
	CAInstallPath    string              `json:"ca_install_path,omitempty"`
	ServiceInstalled bool                `json:"service_installed,omitempty"`
}

func LoadState(dir string) (*State, error) {
	path := filepath.Join(dir, "setup-state.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read setup state %s: %w", path, err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse setup state %s: %w", path, err)
	}
	return &st, nil
}

func SaveState(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal setup state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".setup-state-*.tmp")
	if err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, "setup-state.json")); err != nil {
		return fmt.Errorf("replace setup state: %w", err)
	}
	tmpName = ""
	return nil
}

func ClearState(dir string) error {
	err := os.Remove(filepath.Join(dir, "setup-state.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

const (
	TrustModeUser  = "user"
	TrustModeAdmin = "admin"
)

type Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

const trustCommandTimeout = 20 * time.Second

func runWithTimeout(r Runner, name string, args ...string) ([]byte, error) {
	if _, ok := r.(execRunner); ok {
		cmd := exec.Command(name, args...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			return buf.Bytes(), err
		case <-time.After(trustCommandTimeout):
			_ = cmd.Process.Kill()
			return buf.Bytes(), fmt.Errorf("%s timed out after %s (locked keychain or missing GUI session)", name, trustCommandTimeout)
		}
	}
	return r.CombinedOutput(name, args...)
}

type execRunner struct{}

func (execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func SystemRunner() Runner { return execRunner{} }

type dryRunner struct {
	commands []string
	output   map[string]string
	failOn   map[string]error
}

func (d *dryRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	full := name + " " + joinArgs(args)
	d.commands = append(d.commands, full)
	if err, ok := d.failOn[name]; ok {
		return []byte("simulated failure"), err
	}
	if out, ok := d.output[full]; ok {
		return []byte(out), nil
	}
	return nil, nil
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

func NewDryRunner(failOn ...string) *dryRunner {
	fails := make(map[string]error)
	for _, name := range failOn {
		fails[name] = fmt.Errorf("dry-run failure: %s", name)
	}
	return &dryRunner{failOn: fails}
}

func (d *dryRunner) Commands() []string { return d.commands }

func WriteTempFile(dir, pattern string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return name, nil
}
