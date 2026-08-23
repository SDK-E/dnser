package proxyd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type TCPRoute struct {
	Listen   int
	Host     string
	Backends []string
}

type tcpForwarder struct {
	route  TCPRoute
	ln     net.Listener
	conns  sync.WaitGroup
	closed atomic.Bool
}

type TCPManager struct {
	mu         sync.Mutex
	forwarders map[int]*tcpForwarder
	router     *Router
	dialer     net.Dialer
	stopOnce   sync.Once
	done       chan struct{}
}

func NewTCPManager(router *Router) *TCPManager {
	return &TCPManager{
		forwarders: make(map[int]*tcpForwarder),
		router:     router,
		done:       make(chan struct{}),
	}
}

func (m *TCPManager) Apply(routes []TCPRoute) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := make(map[int]TCPRoute, len(routes))
	for _, r := range routes {
		want[r.Listen] = r
	}

	for port, fwd := range m.forwarders {
		if _, keep := want[port]; !keep {
			fwd.closed.Store(true)
			_ = fwd.ln.Close()
			delete(m.forwarders, port)
		}
	}

	var firstErr error
	for port, route := range want {
		if _, exists := m.forwarders[port]; exists {
			continue
		}
		fwd, err := m.listen(route)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("tcp forwarder bind failed", "port", port, "err", err)
			continue
		}
		m.forwarders[port] = fwd
	}
	return firstErr
}

func (m *TCPManager) listen(route TCPRoute) (*tcpForwarder, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", route.Listen)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind tcp %s: %w", addr, err)
	}
	fwd := &tcpForwarder{route: route, ln: ln}
	fwd.conns.Add(1)
	go func() {
		defer fwd.conns.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				if !fwd.closed.Load() {
					slog.Debug("tcp accept stopped", "addr", addr, "err", err)
				}
				return
			}
			fwd.conns.Add(1)
			go func() {
				defer fwd.conns.Done()
				m.serveConn(fwd, conn)
			}()
		}
	}()
	slog.Info("tcp forwarder listening", "addr", addr, "host", route.Host)
	return fwd, nil
}

func (m *TCPManager) serveConn(fwd *tcpForwarder, inbound net.Conn) {
	defer func() { _ = inbound.Close() }()

	tried := map[string]bool{}
	for range fwd.route.Backends {
		select {
		case <-m.done:
			return
		default:
		}
		if fwd.closed.Load() {
			return
		}
		backend, ok := m.router.Pick(fwd.route.Host, fwd.route.Backends)
		if !ok || tried[backend] {
			continue
		}
		tried[backend] = true

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		outbound, err := m.dialer.DialContext(ctx, "tcp", backend)
		cancel()
		if err != nil {
			slog.Debug("tcp backend dial failed", "host", fwd.route.Host, "backend", backend, "err", err)
			continue
		}
		pipe(inbound, outbound)
		return
	}
}

func pipe(a, b net.Conn) {
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	done := make(chan struct{}, 2)
	copySide := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		_ = dst.SetDeadline(time.Now())
		done <- struct{}{}
	}
	go copySide(a, b)
	go copySide(b, a)
	<-done
	<-done
}

func (m *TCPManager) ActivePorts() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]int, 0, len(m.forwarders))
	for port := range m.forwarders {
		out = append(out, port)
	}
	return out
}

func (m *TCPManager) Shutdown(ctx context.Context) error {
	m.stopOnce.Do(func() { close(m.done) })
	m.mu.Lock()
	for _, fwd := range m.forwarders {
		fwd.closed.Store(true)
		_ = fwd.ln.Close()
	}
	m.forwarders = make(map[int]*tcpForwarder)
	m.mu.Unlock()

	waitDone := make(chan struct{})
	go func() {
		for _, f := range m.allForwarders() {
			f.conns.Wait()
		}
		close(waitDone)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waitDone:
		return nil
	}
}

func (m *TCPManager) allForwarders() []*tcpForwarder {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*tcpForwarder, 0, len(m.forwarders))
	for _, f := range m.forwarders {
		out = append(out, f)
	}
	return out
}
