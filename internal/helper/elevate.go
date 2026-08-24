package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var ErrRefused = errors.New("elevation refused by user")

type Elevator interface {
	Elevate(ctx context.Context, self string, args []string) error
}

func ElevatorForOS() (Elevator, error) {
	switch runtime.GOOS {
	case "darwin":
		return osascriptElevator{}, nil
	case "linux":
		if _, err := exec.LookPath("pkexec"); err == nil {
			return pkexecElevator{}, nil
		}
		return nil, fmt.Errorf("pkexec not found: install polkit to elevate")
	default:
		return nil, fmt.Errorf("elevation unsupported on %s", runtime.GOOS)
	}
}

type osascriptElevator struct{}

func (osascriptElevator) Elevate(ctx context.Context, self string, args []string) error {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, shellQuote(self))
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	script := fmt.Sprintf("do shell script %s with administrator privileges with prompt \"dnser needs permission to change system DNS settings\"", shellQuote(strings.Join(quoted, " ")))
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "User canceled") || strings.Contains(stderr.String(), "-128") {
			return ErrRefused
		}
		return fmt.Errorf("osascript elevation: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type pkexecElevator struct{}

func (pkexecElevator) Elevate(ctx context.Context, self string, args []string) error {
	cmd := exec.CommandContext(ctx, "pkexec", append([]string{self}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 126 {
			return ErrRefused
		}
		if strings.Contains(stderr.String(), "dismissed") || strings.Contains(stderr.String(), "not authorized") {
			return ErrRefused
		}
		return fmt.Errorf("pkexec elevation: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func AmRoot() bool {
	return os.Geteuid() == 0
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\"'\"'`) + "'"
}

func RunSelfElevated(ctx context.Context, planPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	elev, err := ElevatorForOS()
	if err != nil {
		return err
	}
	return elev.Elevate(ctx, self, []string{"helper", "run", "--plan", planPath})
}

func RunDirect(ctx context.Context, planPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, self, "helper", "run", "--plan", planPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helper run: %w", err)
	}
	return nil
}
