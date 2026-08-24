package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Validator func(path string) error

func EmitFile(path string, data []byte, mode os.FileMode, validate Validator) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if validate != nil {
		if err := validate(tmpName); err != nil {
			return fmt.Errorf("validation failed; keeping last known good at %s: %w", path, err)
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("swap into place: %w", err)
	}
	return nil
}

func CommandValidator(ctx context.Context, bin string, args ...string) Validator {
	return func(path string) error {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmdArgs := append(append([]string{}, args...), path)
		out, err := runCommand(cctx, bin, cmdArgs...)
		if err != nil {
			return fmt.Errorf("%s rejected config: %w: %s", bin, err, tailLines(out, 10))
		}
		return nil
	}
}

func CaddyValidate(ctx context.Context, caddyBin string) Validator {
	return CommandValidator(ctx, caddyBin, "validate", "--adapter", "caddyfile", "--config")
}
