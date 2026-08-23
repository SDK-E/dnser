package service

import (
	"fmt"
)

type Manager interface {
	Name() string
	Install(binaryPath string) error
	Uninstall() error
	Start() error
	Stop() error
	IsRunning() (bool, error)
}

func ErrNotImplemented(op string) error {
	return fmt.Errorf("service %s not supported on this platform", op)
}
