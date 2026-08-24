package journal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

var ErrNotSupported = fmt.Errorf("not supported on this platform")

func NewSystemRegistry(runner CommandRunner) Registry {
	reg := NewFSRegistry()
	nic := &NICExecutor{Runner: runner}
	reg[KindNICDNSSet] = nic
	return reg
}

func NewFullRegistry(runner CommandRunner, trust TrustOperator, svc ServiceOperator) Registry {
	reg := NewSystemRegistry(runner)
	if trust != nil {
		t := &CATrustExecutor{Runner: runner, Ops: trust}
		reg[KindCATrust] = t
		reg[KindCAUntrust] = t
	}
	if svc != nil {
		sv := &ServiceExecutor{Ops: svc}
		reg[KindServiceInstal] = sv
		reg[KindServiceRemove] = sv
	}
	return reg
}
