//go:build windows

package runner

func captureLoginShellPath(shell string) (string, error) {
	return "", errNoShell
}

type pathCaptureError string

func (e pathCaptureError) Error() string { return string(e) }

const errNoShell = pathCaptureError("login shell PATH capture not supported on windows")
