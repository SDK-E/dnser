package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type FS struct{}

func NewFSRegistry() Registry {
	fs := &FS{}
	return Registry{
		KindFileWrite:  fs,
		KindFileRemove: fs,
	}
}

func (f *FS) Capture(ctx context.Context, s *Step) (*Capture, error) {
	path, err := s.paramStr("path")
	if err != nil {
		return nil, err
	}
	cap := &Capture{File: &FileCapture{Path: path}}
	info, statErr := os.Lstat(path)
	switch {
	case os.IsNotExist(statErr):
		cap.File.Existed = false
		return cap, nil
	case statErr != nil:
		return nil, fmt.Errorf("lstat %s: %w", path, statErr)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("refusing to capture non-regular file %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cap.File.Existed = true
	cap.File.Perm = uint32(info.Mode().Perm())
	cap.File.Content = string(data)
	return cap, nil
}

func (f *FS) Apply(ctx context.Context, s *Step) error {
	path, perr := s.paramStr("path")
	if perr != nil {
		return perr
	}
	switch s.Kind {
	case KindFileWrite:
		content, err := s.paramStr("content")
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if raw, ok := s.Params["mode"].(float64); ok {
			mode = os.FileMode(raw)
		} else if raw, ok := s.Params["mode"].(int); ok {
			mode = os.FileMode(raw)
		}
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
		tmp, err := os.CreateTemp(dir, ".jwrite-*")
		if err != nil {
			return fmt.Errorf("temp in %s: %w", dir, err)
		}
		tmpName := tmp.Name()
		defer func() { _ = os.Remove(tmpName) }()
		if _, err := tmp.WriteString(content); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("write %s: %w", tmpName, err)
		}
		if err := tmp.Chmod(mode); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("chmod %s: %w", tmpName, err)
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("fsync %s: %w", tmpName, err)
		}
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("close %s: %w", tmpName, err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			return fmt.Errorf("swap into %s: %w", path, err)
		}
		return nil
	case KindFileRemove:
		err := os.Remove(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	default:
		return fmt.Errorf("FS executor cannot apply kind %q", s.Kind)
	}
}

func (f *FS) Invert(ctx context.Context, s *Step) error {
	if s.Capture == nil || s.Capture.File == nil {
		return fmt.Errorf("step %s: no file capture; refusing to invert blind", s.ID)
	}
	fc := s.Capture.File
	if !fc.Existed {
		err := os.Remove(fc.Path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", fc.Path, err)
		}
		return nil
	}
	dir := filepath.Dir(fc.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(fc.Path, []byte(fc.Content), os.FileMode(fc.Perm)); err != nil {
		return fmt.Errorf("restore %s: %w", fc.Path, err)
	}
	if err := os.Chmod(fc.Path, os.FileMode(fc.Perm)); err != nil {
		return fmt.Errorf("restore perms %s: %w", fc.Path, err)
	}
	return nil
}

func (f *FS) Verify(ctx context.Context, s *Step) error {
	path, err := s.paramStr("path")
	if err != nil {
		return err
	}
	switch s.Kind {
	case KindFileWrite:
		data, statErr := os.ReadFile(path)
		if statErr != nil {
			return fmt.Errorf("file %s absent after write", path)
		}
		if want, ok := s.Params["content"].(string); ok && string(data) != want {
			return fmt.Errorf("content drift at %s: %d bytes on disk != %d expected", path, len(data), len(want))
		}
	case KindFileRemove:
		if _, statErr := os.Lstat(path); statErr == nil {
			return fmt.Errorf("file %s still present after remove", path)
		}
	}
	return nil
}
