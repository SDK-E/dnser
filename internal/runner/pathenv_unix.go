//go:build unix

package runner

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func captureLoginShellPath(shell string) (string, error) {
	if shell == "" {
		return "", errNoShell
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-l", "-c", "echo $PATH").Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if i := strings.LastIndexByte(path, '\n'); i >= 0 {
		path = strings.TrimSpace(path[i+1:])
	}
	if path == "" {
		return "", errEmptyShellPath
	}
	return path, nil
}

type pathCaptureError string

func (e pathCaptureError) Error() string { return string(e) }

const (
	errNoShell        = pathCaptureError("no login shell configured")
	errEmptyShellPath = pathCaptureError("login shell returned empty PATH")
)
