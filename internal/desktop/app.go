//go:build desktop

package desktop

import (
	"context"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/SDK-E/dnser/internal/daemon"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	appID           = "enterprises.sdk.dnser.desktop"
	shutdownTimeout = 10 * time.Second
)

func Run(opts Options) int {
	svc, err := New(opts)
	if err != nil {
		slog.Error("desktop init failed", "err", err)
		return 1
	}
	svc.SetReadyHook(func(rt *daemon.Runtime) { mountDesktopRoutes(svc, rt) })
	if err := svc.Start(); err != nil {
		slog.Error("daemon start failed", "err", err)
		return 1
	}

	var app *application.App
	app = application.New(application.Options{
		Name:        "DNSer",
		Description: "Local DNS management for development",
		Assets: application.AssetOptions{
			Handler: assetHandler(svc),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: appID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				showMainWindow(app)
			},
			ExitCode: 0,
		},
		LogLevel: slog.LevelWarn,
	})

	app.OnShutdown(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := svc.Stop(ctx); err != nil {
			slog.Warn("daemon shutdown incomplete", "err", err)
		}
	})

	win := app.Window.NewWithOptions(windowOptions())
	registerCloseToHide(win)

	tray := newTray(app, svc)
	svc.SetChangeHook(tray.refresh)

	go svc.runUpdateLoop(context.Background())

	app.RegisterService(application.NewService(NewBridge(svc)))

	if err := app.Run(); err != nil {
		slog.Error("app loop failed", "err", err)
		return 1
	}
	return 0
}

func mountDesktopRoutes(svc *Service, rt *daemon.Runtime) {
	mux, ok := rt.APIHandler().(*http.ServeMux)
	if !ok {
		return
	}
	mux.Handle("/api/v1/desktop/", svc.APIRoutes())
}

func windowOptions() application.WebviewWindowOptions {
	opts := application.WebviewWindowOptions{
		Title:            "DNS.er",
		Width:            1120,
		Height:           740,
		MinWidth:         880,
		MinHeight:        560,
		URL:              "/",
		BackgroundType:   application.BackgroundTypeSolid,
		BackgroundColour: application.NewRGBA(8, 32, 3, 255),
	}
	mapping, _ := closeToHideMapping()
	switch runtime.GOOS {
	case "darwin":
		opts.Mac.EventMapping = mapping
	case "windows":
		opts.Windows.EventMapping = mapping
	}
	return opts
}

func closeToHideMapping() (map[events.WindowEventType]events.WindowEventType, events.WindowEventType) {
	mapping := events.DefaultWindowEventMapping()
	var closing events.WindowEventType
	switch runtime.GOOS {
	case "darwin":
		closing = events.Mac.WindowShouldClose
	case "windows":
		closing = events.Windows.WindowClosing
	default:
		closing = events.Linux.WindowDeleteEvent
	}
	delete(mapping, closing)
	return mapping, closing
}

func registerCloseToHide(win *application.WebviewWindow) {
	_, closing := closeToHideMapping()
	win.OnWindowEvent(closing, func(*application.WindowEvent) {
		win.Hide()
	})
}

func showMainWindow(app *application.App) {
	if win := app.Window.Current(); win != nil {
		win.Show()
		win.Focus()
	}
}

func assetHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := svc.APIHandler()
		if handler == nil {
			http.Error(w, "DNSer daemon is not running", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
