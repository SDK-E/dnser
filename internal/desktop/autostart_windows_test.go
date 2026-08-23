//go:build windows

package desktop

import (
	"strings"
	"testing"
)

func TestWindowsRegCommands(t *testing.T) {
	r := &recordingRunner{}
	if err := setAutostartWindows(r, `C:\Apps\dnser-desktop.exe`, true); err != nil {
		t.Fatalf("enable: %v", r.calls)
	}
	if len(r.calls) != 1 || !strings.Contains(r.calls[0], "reg add") || !strings.Contains(r.calls[0], `C:\Apps\dnser-desktop.exe`) {
		t.Fatalf("unexpected enable calls: %v", r.calls)
	}
	r2 := &recordingRunner{failOn: "reg delete"}
	if err := setAutostartWindows(r2, "", false); err != nil && strings.Contains(err.Error(), "remove") {
		t.Fatalf("delete of missing key should be tolerated: %v", err)
	}
	if !autostartActiveWindows(&recordingRunner{output: []byte("DNSer    REG_SZ    C:\\x\\y.exe")}) {
		t.Fatal("expected active when query returns value")
	}
	if autostartActiveWindows(&recordingRunner{}) {
		t.Fatal("expected inactive on empty query")
	}
}

type recordingRunner struct {
	calls  []string
	failOn string
	output []byte
}

func (r *recordingRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	full := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, full)
	if r.failOn != "" && strings.HasPrefix(full, r.failOn) {
		return []byte("unable to find the specified registry key or value"), &exitErrStub{}
	}
	return r.output, nil
}

type exitErrStub struct{}

func (*exitErrStub) Error() string { return "exit status 1" }
