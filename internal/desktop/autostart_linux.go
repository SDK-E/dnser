package desktop

import (
	"os"
	"path/filepath"
)

func linuxAutostartPath() (string, error) {
	home, err := userHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", "dnser-desktop.desktop"), nil
}

func setAutostart(exe string, enabled bool) error {
	path, err := linuxAutostartPath()
	if err != nil {
		return err
	}
	if !enabled {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return writeAtomic(path, desktopEntryContent(exe))
}

func autostartActive() bool {
	path, err := linuxAutostartPath()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(path)
	return statErr == nil
}
