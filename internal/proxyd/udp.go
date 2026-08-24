package proxyd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const udpSessionIdle = 30 * time.Second

type UDPRoute struct {
	Listen   int
	Host     string
	Backends []string
}

type udpForwarder struct {
	route  UDPRoute
	conn   *net.UDPConn
	closed atomic.Bool
}

type udpSession struct {
	backend string
	conn    *net.UDPConn
	last    atomic.Int64
}

type UDPManager struct {
	mu         sync.Mutex
	forwarders map[int]*udpForwarder
	sessions   map[string]*udpSession
	router     *Router
	stopOnce   sync.Once
	done       chan struct{}
	janitor    *time.Ticker
}

func NewUDPManager(router *Router) *UDPManager {
	m := &UDPManager{
		forwarders: make(map[int]*udpForwarder),
		sessions:   make(map[string]*udpSession),
		router:     router,
		done:       make(chan struct{}),
		janitor:    time.NewTicker(10 * time.Second),
	}
	go m.reapLoop()
	return m
}

func (m *UDPManager) Apply(routes []UDPRoute) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := make(map[int]UDPRoute, len(routes))
	for _, r := range routes {
		want[r.Listen] = r
	}
	for port, fwd := range m.forwarders {
		if _, keep := want[port]; !keep {
			fwd.closed.Store(true)
			_ = fwd.conn.Close()
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
			slog.Error("udp forwarder bind failed", "port", port, "err", err)
			continue
		}
		m.forwarders[port] = fwd
	}
	return firstErr
}

func (m *UDPManager) listen(route UDPRoute) (*udpForwarder, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", route.Listen)
	lnAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve udp %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", lnAddr)
	if err != nil {
		return nil, fmt.Errorf("bind udp %s: %w", addr, err)
	}
	fwd := &udpForwarder{route: route, conn: conn}
	go fwd.readLoop(m)
	slog.Info("udp forwarder listening", "addr", addr, "host", route.Host)
	return fwd, nil
}

func (f *udpForwarder) readLoop(m *UDPManager) {
	buf := make([]byte, 65535)
	for {
		n, client, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			if !f.closed.Load() {
				slog.Debug("udp accept stopped", "host", f.route.Host, "err", err)
			}
			return
		}
		m.relay(f, client, buf[:n])
	}
}

func (m *UDPManager) relay(f *udpForwarder, client *net.UDPAddr, payload []byte) {
	key := client.String()
	m.mu.Lock()
	sess, ok := m.sessions[key]
	m.mu.Unlock()
	if ok {
		if _, err := sess.conn.Write(payload); err != nil {
			m.dropSession(key, sess)
		} else {
			sess.last.Store(time.Now().UnixNano())
			return
		}
	}
	backend, ok := m.router.Pick(f.route.Host, f.route.Backends)
	if !ok {
		return
	}
	raddr, err := net.ResolveUDPAddr("udp", backend)
	if err != nil {
		slog.Debug("udp backend resolve failed", "host", f.route.Host, "backend", backend, "err", err)
		return
	}
	up, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		slog.Debug("udp backend dial failed", "host", f.route.Host, "backend", backend, "err", err)
		return
	}
	sess = &udpSession{backend: backend, conn: up}
	sess.last.Store(time.Now().UnixNano())
	m.mu.Lock()
	if old, exists := m.sessions[key]; exists {
		m.mu.Unlock()
		_ = up.Close()
		if _, err := old.conn.Write(payload); err != nil {
			m.dropSession(key, old)
		} else {
			old.last.Store(time.Now().UnixNano())
		}
		return
	}
	m.sessions[key] = sess
	m.mu.Unlock()
	go sess.pump(f, client)
	_, _ = sess.conn.Write(payload)
}

func (s *udpSession) pump(f *udpForwarder, client *net.UDPAddr) {
	buf := make([]byte, 65535)
	for {
		n, err := s.conn.Read(buf)
		if err != nil {
			return
		}
		if f.closed.Load() {
			return
		}
		if _, err := f.conn.WriteToUDP(buf[:n], client); err != nil {
			return
		}
		s.last.Store(time.Now().UnixNano())
	}
}

func (m *UDPManager) dropSession(key string, sess *udpSession) {
	m.mu.Lock()
	if cur, ok := m.sessions[key]; ok && cur == sess {
		delete(m.sessions, key)
	} else {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	_ = sess.conn.Close()
}

func (m *UDPManager) reapLoop() {
	for {
		select {
		case <-m.done:
			return
		case <-m.janitor.C:
			cutoff := time.Now().Add(-udpSessionIdle).UnixNano()
			m.mu.Lock()
			var stale []*udpSession
			keys := make([]string, 0, 4)
			for key, sess := range m.sessions {
				if sess.last.Load() < cutoff {
					stale = append(stale, sess)
					keys = append(keys, key)
					delete(m.sessions, key)
				}
			}
			m.mu.Unlock()
			for i, sess := range stale {
				_ = sess.conn.Close()
				slog.Debug("udp session reaped", "key", keys[i])
			}
		}
	}
}

func (m *UDPManager) Backends() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, f := range m.forwarders {
		for _, b := range f.route.Backends {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
	}
	return out
}

func (m *UDPManager) ActivePorts() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]int, 0, len(m.forwarders))
	for port := range m.forwarders {
		out = append(out, port)
	}
	return out
}

func (m *UDPManager) Shutdown(ctx context.Context) error {
	m.stopOnce.Do(func() { close(m.done); m.janitor.Stop() })
	m.mu.Lock()
	for _, fwd := range m.forwarders {
		fwd.closed.Store(true)
		_ = fwd.conn.Close()
	}
	sessions := m.sessions
	m.forwarders = make(map[int]*udpForwarder)
	m.sessions = make(map[string]*udpSession)
	m.mu.Unlock()
	for _, sess := range sessions {
		_ = sess.conn.Close()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
