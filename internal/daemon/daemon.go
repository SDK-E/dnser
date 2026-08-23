package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SDK-E/dnser/internal/api"
	"github.com/SDK-E/dnser/internal/certs"
	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/dnscore"
	"github.com/SDK-E/dnser/internal/health"
	"github.com/SDK-E/dnser/internal/logstream"
	"github.com/SDK-E/dnser/internal/proxyd"
)

type Runtime struct {
	mu      sync.Mutex
	store   *config.Store
	stream  *logstream.Stream
	ca      *certs.CA
	manager *certs.Manager
	router  *proxyd.Router
	dns     *dnscore.Server
	proxy   *proxyd.Server
	ui      *api.Server
	checker *health.Checker

	dnsPort   int
	uiPort    int
	stopWatch func()
	reloaded  chan struct{}
}

type Options struct {
	Store         *config.Store
	CertsDir      string
	DNSBindPort   int
	DNSFallbacks  []int
	Version       string
	SkipListeners bool
}

func New(opts Options) (*Runtime, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("daemon: store required")
	}
	if opts.CertsDir == "" {
		opts.CertsDir = filepath.Dir(opts.Store.Path()) + string(os.PathSeparator) + "certs"
	}
	dnsFallbacks := opts.DNSFallbacks
	if len(dnsFallbacks) == 0 {
		dnsFallbacks = []int{5353, 35353}
	}

	rt := &Runtime{
		store:    opts.Store,
		stream:   logstream.New(logstream.DefaultCapacity),
		reloaded: make(chan struct{}, 1),
	}

	cfg := rt.store.Get()

	ca, err := certs.NewCA(opts.CertsDir)
	if err != nil {
		return nil, fmt.Errorf("init CA: %w", err)
	}
	rt.ca = ca
	rt.manager = certs.NewManager(ca)
	rt.router = proxyd.NewRouter()

	engine := dnscore.NewEngine(cfg)
	forward, err := dnscore.NewForwarder(cfg.Settings.Upstreams)
	if err != nil {
		return nil, fmt.Errorf("init upstream forwarder: %w", err)
	}
	cache := dnscore.NewCache()
	dnsServer, err := dnscore.NewServer(dnscore.Options{
		Engine: engine, Forward: forward, Cache: cache, Stream: rt.stream,
	})
	if err != nil {
		return nil, err
	}
	rt.dns = dnsServer

	rt.proxy = proxyd.NewServer(rt.router, rt.manager)

	uiPort := cfg.Settings.Ports.UI
	if !opts.SkipListeners {
		picked, ok := dnscore.PickFreeTCPPort(cfg.Settings.Bind, uiPort)
		if !ok {
			return nil, fmt.Errorf("start dashboard: no available port on %s", cfg.Settings.Bind)
		}
		if picked != uiPort {
			slog.Warn(fmt.Sprintf("port %s:%d unavailable; dashboard listening on %s:%d instead", cfg.Settings.Bind, uiPort, cfg.Settings.Bind, picked))
		}
		uiPort = picked
	}
	rt.uiPort = uiPort

	rt.applyRoutes(cfg, uiPort)

	if !opts.SkipListeners {
		bind := cfg.Settings.Bind
		preferred := cfg.Settings.Ports.DNS
		if opts.DNSBindPort > 0 {
			preferred = opts.DNSBindPort
		}
		port, note, pickErr := pickDNSPort(bind, preferred, dnsFallbacks)
		if pickErr != nil {
			return nil, pickErr
		}
		if err := rt.dns.ListenAndServe(bind, port); err != nil {
			return nil, fmt.Errorf("start DNS server: %w", err)
		}
		rt.dnsPort = port
		if note != "" {
			slog.Warn(note)
		}

		httpAddr := fmt.Sprintf("%s:%d", bind, cfg.Settings.Ports.HTTP)
		httpsAddr := fmt.Sprintf("%s:%d", bind, cfg.Settings.Ports.HTTPS)
		if err := rt.proxy.Serve(httpAddr, httpsAddr); err != nil {
			return nil, fmt.Errorf("start proxy: %w", err)
		}

		rt.checker = health.NewChecker(rt.router.Targets, 5*time.Second)
		rt.checker.Start()
	}

	rt.ui = api.New(rt, opts.Version)
	if !opts.SkipListeners {
		if err := rt.ui.ListenAndServe(cfg.Settings.Bind, uiPort); err != nil {
			return nil, fmt.Errorf("start dashboard: %w", err)
		}
	}

	stop, err := rt.store.Watch(func(cfg config.Config) {
		if err := rt.Reload(cfg); err != nil {
			slog.Warn("daemon reload failed", "err", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("watch config: %w", err)
	}
	rt.stopWatch = stop
	return rt, nil
}

func pickDNSPort(bind string, preferred int, fallbacks []int) (int, string, error) {
	port, err := dnscore.PickPort(bind, preferred, fallbacks...)
	if err != nil {
		return 0, "", err
	}
	note := ""
	if port != preferred {
		note = fmt.Sprintf("port %d unavailable; DNS listening on %d instead", preferred, port)
	}
	return port, note, nil
}

func (rt *Runtime) applyRoutes(cfg config.Config, uiPort int) {
	var routes []proxyd.Route
	for _, p := range cfg.Projects {
		if p.Port <= 0 {
			continue
		}
		target := fmt.Sprintf("%s:%d", cfg.Settings.Bind, p.Port)
		routes = append(routes, proxyd.Route{
			Host: p.Domain, Target: target, HTTPS: p.HTTPS, ForceHTTPS: p.ForceHTTPS, Port: p.Port,
		})
		if p.Wildcard {
			routes = append(routes, proxyd.Route{
				Host: "*." + p.Domain, Target: target, HTTPS: p.HTTPS, ForceHTTPS: p.ForceHTTPS, Port: p.Port,
			})
		}
		for _, a := range p.Aliases {
			routes = append(routes, proxyd.Route{
				Host: a, Target: target, HTTPS: p.HTTPS, ForceHTTPS: p.ForceHTTPS, Port: p.Port,
			})
		}
	}
	dashTarget := fmt.Sprintf("%s:%d", cfg.Settings.Bind, uiPort)
	routes = append(routes,
		proxyd.Route{Host: config.DashboardDomain(cfg.Settings.TLD), Target: dashTarget, HTTPS: true, Port: uiPort},
	)
	rt.router.Replace(routes)
}

func (rt *Runtime) Reload(cfg config.Config) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	slog.Info("config changed; reloading zones and routes", "projects", len(cfg.Projects))
	rt.dns.UseEngine(dnscore.NewEngine(cfg))
	if rt.dns != nil && rt.dns.Cache() != nil {
		rt.dns.Cache().InvalidateAll()
	}
	rt.applyRoutes(cfg, rt.uiPort)
	select {
	case rt.reloaded <- struct{}{}:
	default:
	}
	return nil
}

func (rt *Runtime) NotifyReload() <-chan struct{} { return rt.reloaded }

func (rt *Runtime) Stream() *logstream.Stream { return rt.stream }

func (rt *Runtime) Store() *config.Store { return rt.store }

func (rt *Runtime) Manager() *certs.Manager { return rt.manager }

func (rt *Runtime) Router() *proxyd.Router { return rt.router }

func (rt *Runtime) Checker() *health.Checker { return rt.checker }

func (rt *Runtime) Proxy() *proxyd.Server { return rt.proxy }

func (rt *Runtime) DNSPort() int { return rt.dnsPort }

func (rt *Runtime) UIPort() int {
	if rt.uiPort > 0 {
		return rt.uiPort
	}
	return rt.store.Settings().Ports.UI
}

func (rt *Runtime) APIHandler() http.Handler {
	if rt.ui == nil {
		return nil
	}
	return rt.ui.Handler()
}

func (rt *Runtime) DashboardURL() string {
	st := rt.store.Settings()
	return fmt.Sprintf("https://%s:%d", config.DashboardDomain(st.TLD), st.Ports.HTTPS)
}

func (rt *Runtime) Shutdown(ctx context.Context) error {
	if rt.stopWatch != nil {
		rt.stopWatch()
	}
	if rt.checker != nil {
		rt.checker.Stop()
	}
	var firstErr error
	if rt.ui != nil {
		if err := rt.ui.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := rt.dns.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := rt.proxy.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
