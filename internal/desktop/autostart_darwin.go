package desktop

import (
	"os"
	"path/filepath"
)

func darwinPlistPath() (string, error) {
	home, err := userHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", darwinAutostartLabel+".plist"), nil
}

func setAutostart(exe string, enabled bool) error {
	path, err := darwinPlistPath()
	if err != nil {
		return err
	}
	if !enabled {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	}
	return writeAtomic(path, plistContent(darwinAutostartLabel, exe))
}

func autostartActive() bool {
	path, err := darwinPlistPath()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(path)
	return statErr == nil
}
