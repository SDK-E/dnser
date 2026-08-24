package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Lock struct {
	file *os.File
}

func AcquireLock() (*Lock, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".dnser"), 0o700); err != nil {
		return nil, fmt.Errorf("create ~/.dnser: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(home, ".dnser", "dnser.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, ErrLocked
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}
