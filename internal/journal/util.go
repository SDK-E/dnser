package journal

import (
	"path/filepath"
)

func parentDir(p string) string {
	return filepath.Dir(p)
}
