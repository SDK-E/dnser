//go:build desktop

package desktop

import (
	_ "embed"
)

//go:embed assets/tray.png
var trayIcon []byte
