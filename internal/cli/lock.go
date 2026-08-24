package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type Lock struct {
	file *os.File
}

func lockFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".dnser", "dnser.lock"), nil
}

func AcquireLock() (*Lock, error) {
	path, err := lockFilePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create ~/.dnser: %w", err)
	}
	f, err := openLockFile(path)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, ErrLocked
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	flockRelease(l.file)
	_ = l.file.Close()
	l.file = nil
}
