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

type RootInstaller interface {
	InstallRoot(binaryPath string) error
	UninstallRoot() error
	HasRootService() bool
}

func ErrNotImplemented(op string) error {
	return fmt.Errorf("service %s not supported on this platform", op)
}
