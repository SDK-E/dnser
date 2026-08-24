//go:build windows

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func pathErr(err error) error {
	return fmt.Errorf("resolve home: %w", err)
}

func openLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}

func flockExclusive(f *os.File) error {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	return windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
}

func flockRelease(f *os.File) {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
}
