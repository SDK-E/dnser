//go:build desktop

package desktop

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/SDK-E/dnser/internal/runner"
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
	if !st.Running {
		t.tray.SetLabel("")
		t.tray.SetTooltip("DNSer · daemon stopped")
		return
	}
	upCount := 0
	for _, appSum := range st.Apps {
		if appSum.State == "up" {
			upCount++
		}
	}
	label := ""
	if upCount > 0 {
		label = fmt.Sprint(upCount)
	}
	t.tray.SetLabel(label)
	tooltip := fmt.Sprintf("DNSer · DNS :%d · %d project(s)", st.DNSPort, st.Projects)
	if len(st.Apps) > 0 {
		tooltip += fmt.Sprintf(" · %d app(s) running", upCount)
	}
	t.tray.SetTooltip(tooltip)
}

func (t *trayController) buildMenu() *application.Menu {
	root := application.NewMenu()

	root.Add("Dashboard").OnClick(func(*application.Context) {
		showMainWindow(t.app)
	})
	root.Add("Open in Browser").OnClick(func(*application.Context) {
		if url := t.svc.Status().DashboardURL; url != "" {
			openInBrowser(url)
		}
	})

	st := t.svc.Status()
	if len(st.Apps) > 0 {
		projectMenu := root.AddSubmenu("Projects")
		for _, appSum := range st.Apps {
			appSum := appSum
			stateMark := map[string]string{
				"up": "●", "starting": "◐", "crash-looping": "✗",
				"stopped": "○", "failed": "✗", "deps-missing": "◌",
			}[appSum.State]
			if stateMark == "" {
				stateMark = "○"
			}
			label := fmt.Sprintf("%s %s", stateMark, appSum.Domain)
			if appSum.Port > 0 && appSum.State == "up" {
				label += fmt.Sprintf(" :%d", appSum.Port)
			}
			projectMenu.Add(label).OnClick(func(*application.Context) {
				openInBrowser(fmt.Sprintf("https://%s", appSum.Domain))
			})
		}
		projectMenu.AddSeparator()
		projectMenu.Add("Restart All").OnClick(func(*application.Context) {
			go func() {
				if sup := t.runnerSupervisor(); sup != nil {
					domains := make([]string, 0, 8)
					for domain := range sup.Info() {
						domains = append(domains, domain)
					}
					sort.Strings(domains)
					for _, domain := range domains {
						if err := sup.Restart(domain); err != nil {
							slog.Warn("restart failed", "project", domain, "err", err)
						}
					}
				}
				t.refresh()
			}()
		})
		projectMenu.Add("Stop All").OnClick(func(*application.Context) {
			go func() {
				if sup := t.runnerSupervisor(); sup != nil {
					for domain := range sup.Info() {
						sup.Stop(domain)
					}
				}
				t.refresh()
			}()
		})
	}

	if upd := t.svc.Update(); upd.Available {
		label := "Download Update"
		if upd.Version != "" {
			label = fmt.Sprintf("Download Update %s", upd.Version)
		}
		url := upd.URL
		root.AddSeparator()
		root.Add(label).OnClick(func(*application.Context) {
			openInBrowser(url)
		})
	}

	root.AddSeparator()
	root.AddCheckbox("Launch at Login", autostartActive()).OnClick(func(*application.Context) {
		target := !autostartActive()
		if err := t.svc.SetAutostart(target); err != nil {
			slog.Warn("toggle autostart failed", "err", err)
		}
		t.refresh()
	})

	setupLabel := "Setup System Integration…"
	if setupSt := t.svc.SetupStatus(); setupSt.Routed {
		setupLabel = "Re-run System Integration"
	}
	root.Add(setupLabel).OnClick(func(*application.Context) {
		go func() {
			if err := t.svc.RunSetup(nil); err != nil {
				slog.Warn("setup from tray failed", "err", err)
			}
			t.refresh()
		}()
	})
	root.Add("Restore System Settings").OnClick(func(*application.Context) {
		go func() {
			if err := t.svc.RevertSetup(); err != nil {
				slog.Warn("revert from tray failed", "err", err)
			}
			t.refresh()
		}()
	})

	root.AddSeparator()
	root.Add("Quit DNSer").OnClick(func(*application.Context) {
		t.app.Quit()
	})
	return root
}

func (t *trayController) runnerSupervisor() *runner.Supervisor {
	rt := t.svc.Runtime()
	if rt == nil {
		return nil
	}
	return rt.Runner()
}
