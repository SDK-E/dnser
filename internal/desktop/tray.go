//go:build desktop

package desktop

import (
	"fmt"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type trayController struct {
	app  *application.App
	svc  *Service
	tray *application.SystemTray
}

func newTray(app *application.App, svc *Service) *trayController {
	t := &trayController{app: app, svc: svc}
	t.tray = app.SystemTray.New()
	t.tray.SetIcon(trayIcon)
	t.refresh()
	return t
}

func (t *trayController) refresh() {
	t.tray.SetMenu(t.buildMenu())
	st := t.svc.Status()
	if st.Running {
		t.tray.SetTooltip(fmt.Sprintf("DNSer · DNS :%d · %d project(s)", st.DNSPort, st.Projects))
	} else {
		t.tray.SetTooltip("DNSer · daemon stopped")
	}
}

func (t *trayController) buildMenu() *application.Menu {
	dashboard := application.NewMenuItem("Dashboard")
	dashboard.OnClick(func(*application.Context) {
		showMainWindow(t.app)
	})

	browser := application.NewMenuItem("Open in Browser")
	browser.OnClick(func(*application.Context) {
		if url := t.svc.Status().DashboardURL; url != "" {
			openInBrowser(url)
		}
	})

	var items []*application.MenuItem
	items = append(items, dashboard, browser)

	if upd := t.svc.Update(); upd.Available {
		label := "Download Update"
		if upd.Version != "" {
			label = fmt.Sprintf("Download Update %s", upd.Version)
		}
		updItem := application.NewMenuItem(label)
		version := upd.URL
		updItem.OnClick(func(*application.Context) {
			openInBrowser(version)
		})
		items = append(items, application.NewMenuItemSeparator(), updItem)
	}

	autostartItem := application.NewMenuItemCheckbox("Launch at Login", autostartActive())
	autostartItem.OnClick(func(*application.Context) {
		target := !autostartActive()
		if err := t.svc.SetAutostart(target); err != nil {
			slog.Warn("toggle autostart failed", "err", err)
		}
		t.refresh()
	})

	setupLabel := "Setup System Integration…"
	if st := t.svc.SetupStatus(); st.Routed {
		setupLabel = "Re-run System Integration"
	}
	setupItem := application.NewMenuItem(setupLabel)
	setupItem.OnClick(func(*application.Context) {
		go func() {
			if err := t.svc.RunSetup(nil); err != nil {
				slog.Warn("setup from tray failed", "err", err)
			}
			t.refresh()
		}()
	})

	revertItem := application.NewMenuItem("Restore System Settings")
	revertItem.OnClick(func(*application.Context) {
		go func() {
			if err := t.svc.RevertSetup(); err != nil {
				slog.Warn("revert from tray failed", "err", err)
			}
			t.refresh()
		}()
	})

	quit := application.NewMenuItem("Quit DNSer")
	quit.OnClick(func(*application.Context) {
		t.app.Quit()
	})

	items = append(items,
		application.NewMenuItemSeparator(),
		autostartItem,
		setupItem,
		revertItem,
		application.NewMenuItemSeparator(),
		quit,
	)
	return application.NewMenuFromItems(items[0], items[1:]...)
}
