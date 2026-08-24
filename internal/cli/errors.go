package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func isUsageText(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"flag provided but not defined",
		"invalid argument",
		"accepts ",
		"requires at least",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

var (
	ErrUsage      = errors.New("usage error")
	ErrAborted    = errors.New("aborted by user")
	ErrLocked     = errors.New("another dnser command is running")
	ErrDaemonGone = errors.New("daemon not running")
)

type ConfirmRequiredError struct {
	Severity string
	Changes  []Change
	Hint     string
	Token    string
}

func (e *ConfirmRequiredError) Error() string {
	if e.Hint != "" {
		return e.Hint
	}
	return "confirmation required"
}

type ElevationRequiredError struct {
	Command string
}

func (e *ElevationRequiredError) Error() string {
	return "elevation required: " + e.Command
}

type DoctorIssuesError struct {
	Issues int
}

func (e *DoctorIssuesError) Error() string {
	return fmt.Sprintf("doctor found %d issues", e.Issues)
}

type CodedError struct {
	Code        int    `json:"code"`
	Kind        string `json:"kind"`
	ErrorText   string `json:"error"`
	Remediation string `json:"remediation,omitempty"`
}

func ErrorEnvelope(err error) (CodedError, int) {
	var confirm *ConfirmRequiredError
	var elev *ElevationRequiredError
	var doctor *DoctorIssuesError
	switch {
	case errors.As(err, &confirm):
		cmd := RebuildConfirmCommand(os.Args, confirm)
		env := CodedError{Code: 3, Kind: "confirmation_required", ErrorText: "mutation plan requires confirmation", Remediation: cmd}
		return env, 3

	case errors.As(err, &elev):
		return CodedError{Code: 4, Kind: "elevation_required", ErrorText: elev.Error(), Remediation: elev.Command}, 4
	case errors.As(err, &doctor):
		return CodedError{Code: 10, Kind: "issues_found", ErrorText: doctor.Error()}, 10
	case errors.Is(err, ErrUsage):
		return CodedError{Code: 2, Kind: "usage", ErrorText: err.Error()}, 2
	case isUsageText(err):
		return CodedError{Code: 2, Kind: "usage", ErrorText: err.Error()}, 2
	case errors.Is(err, ErrLocked):
		return CodedError{Code: 1, Kind: "locked", ErrorText: err.Error(), Remediation: "wait for the running dnser command to finish"}, 1
	case errors.Is(err, ErrAborted):
		return CodedError{Code: 130, Kind: "aborted", ErrorText: err.Error()}, 130
	default:
		return CodedError{Code: 1, Kind: "operational", ErrorText: err.Error()}, 1
	}
}

func RebuildConfirmCommand(args []string, c *ConfirmRequiredError) string {
	base := "dnser"
	if len(args) > 0 {
		base = args[0]
	}
	rest := make([]string, 0, len(args))
	skip := map[string]bool{"-y": true, "--yes": true, "--no-input": true}
	for i := 1; i < len(args); i++ {
		if skip[args[i]] {
			continue
		}
		rest = append(rest, args[i])
	}
	tail := ""
	if c.Severity == SeveritySevere && c.Token != "" {
		tail = " --confirm " + c.Token
	} else {
		tail = " --yes"
	}
	return base + " " + joinArgs(rest) + tail
}

func joinArgs(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
