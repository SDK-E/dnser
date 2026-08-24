package dnsl

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

var DefaultPortChain = []int{53, 5353, 35353}

type Config struct {
	Upstreams    []string
	Suffixes     []string
	Answers      []Answer
	PortChain    []int
	CacheEnabled bool
}

type Listener struct {
	cfg   Config
	shim  *Shim
	proxy *proxy.Proxy
	port  int
}

func New(cfg Config) *Listener {
	if len(cfg.PortChain) == 0 {
		cfg.PortChain = DefaultPortChain
	}
	if len(cfg.Upstreams) == 0 {
		cfg.Upstreams = []string{"1.1.1.1:53"}
	}
	return &Listener{cfg: cfg}
}

func (l *Listener) Start(ctx context.Context) error {
	upstreamAddr := normalizeUpstream(l.cfg.Upstreams[0])
	l.shim = NewShim(l.cfg.Answers, l.cfg.Suffixes, upstreamAddr)
	if err := l.shim.Start(0); err != nil {
		return fmt.Errorf("start answer shim: %w", err)
	}

	var lastErr error
	for _, candidate := range l.cfg.PortChain {
		p, err := newProxy(candidate, l.shim.Port(), l.cfg, upstreamAddr)
		if err != nil {
			lastErr = err
			continue
		}
		if err := p.Start(ctx); err != nil {
			lastErr = fmt.Errorf("bind %d: %w", candidate, err)
			continue
		}
		l.proxy = p
		l.port = candidate
		if candidate == 0 {
			if ap, ok := p.Addr(proxy.ProtoUDP).(*net.UDPAddr); ok && ap != nil {
				l.port = ap.Port
			}
		}
		return nil
	}
	_ = l.shim.Stop()
	if lastErr == nil {
		lastErr = fmt.Errorf("no ports available")
	}
	return fmt.Errorf("dns listener start failed (chain %v): %w", l.cfg.PortChain, lastErr)
}

func newProxy(port, shimPort int, cfg Config, upstreamAddr string) (*proxy.Proxy, error) {
	lines := []string{}
	for _, u := range cfg.Upstreams {
		lines = append(lines, normalizeUpstream(u))
	}
	for _, sfx := range normalizeSuffixes(cfg.Suffixes) {
		lines = append(lines, fmt.Sprintf("[/%s/]%s", sfx, net.JoinHostPort(loopbackIP, strconv.Itoa(shimPort))))
	}
	lines = append(lines, fmt.Sprintf("[/dnser.internal/]%s", net.JoinHostPort(loopbackIP, strconv.Itoa(shimPort))))
	opts := &upstream.Options{
		Logger: slog.Default(),
	}
	upsConf, err := proxy.ParseUpstreamsConfig(lines, opts)
	if err != nil {
		return nil, fmt.Errorf("parse upstream config: %w", err)
	}
	udpAddr := &net.UDPAddr{IP: net.ParseIP(loopbackIP), Port: port}
	tcpAddr := &net.TCPAddr{IP: net.ParseIP(loopbackIP), Port: port}
	return proxy.New(&proxy.Config{
		Logger:         slog.Default(),
		UDPListenAddr:  []*net.UDPAddr{udpAddr},
		TCPListenAddr:  []*net.TCPAddr{tcpAddr},
		UpstreamConfig: upsConf,
		CacheEnabled:   cfg.CacheEnabled,
	})
}

func (l *Listener) Stop(ctx context.Context) error {
	var firstErr error
	if l.proxy != nil {
		if err := l.proxy.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		l.proxy = nil
	}
	if l.shim != nil {
		if err := l.shim.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		l.shim = nil
	}
	l.port = 0
	return firstErr
}

func (l *Listener) Port() int {
	return l.port
}

func (l *Listener) ShimPort() int {
	if l.shim == nil {
		return 0
	}
	return l.shim.Port()
}

func (l *Listener) Probe() error {
	if l.port == 0 || l.shim == nil {
		return fmt.Errorf("listener not started")
	}
	client := &dns.Client{Net: "udp", Timeout: probeTimeout}
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(HealthName), dns.TypeA)
	resp, _, err := client.Exchange(q, net.JoinHostPort(loopbackIP, strconv.Itoa(l.port)))
	if err != nil {
		return fmt.Errorf("probe %d: %w", l.port, err)
	}
	if resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) == 0 {
		return fmt.Errorf("probe %d: unhealthy response", l.port)
	}
	return nil
}
