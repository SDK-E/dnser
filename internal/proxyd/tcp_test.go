package proxyd

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func startEchoTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func TestTCPManagerForwardsBytes(t *testing.T) {
	router := NewRouter()
	mgr := NewTCPManager(router)
	defer func() { _ = mgr.Shutdown(t.Context()) }()

	echoAddr := startEchoTCP(t)
	echoPort := portOf(echoAddr)

	if err := mgr.Apply([]TCPRoute{{Listen: echoPort + 1, Host: "smtp.x.test", Backends: []string{echoAddr}}}); err != nil {
		t.Fatal(err)
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", echoPort+1), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	msg := []byte("ping-through-dnser")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(msg) {
		t.Errorf("echo = %q, want %q", buf, msg)
	}

	if ports := mgr.ActivePorts(); len(ports) != 1 || ports[0] != echoPort+1 {
		t.Errorf("active ports = %v", ports)
	}
}

func TestTCPManagerApplyRemovesStaleListeners(t *testing.T) {
	router := NewRouter()
	mgr := NewTCPManager(router)
	defer func() { _ = mgr.Shutdown(t.Context()) }()

	a := startEchoTCP(t)
	b := startEchoTCP(t)

	if err := mgr.Apply([]TCPRoute{{Listen: portOf(a) + 2, Host: "a.test", Backends: []string{a}}}); err != nil {
		t.Fatal(err)
	}
	if len(mgr.ActivePorts()) != 1 {
		t.Fatalf("want 1 forwarder, got %v", mgr.ActivePorts())
	}
	if err := mgr.Apply([]TCPRoute{{Listen: portOf(b) + 2, Host: "b.test", Backends: []string{b}}}); err != nil {
		t.Fatal(err)
	}
	if ports := mgr.ActivePorts(); len(ports) != 1 || ports[0] != portOf(b)+2 {
		t.Fatalf("stale listener not removed: %v", mgr.ActivePorts())
	}
}

func TestTCPManagerBindCollisionFailsThatRouteOnly(t *testing.T) {
	router := NewRouter()
	mgr := NewTCPManager(router)
	defer func() { _ = mgr.Shutdown(t.Context()) }()

	held := startEchoTCP(t)
	free := startEchoTCP(t)

	err := mgr.Apply([]TCPRoute{
		{Listen: portOf(held), Host: "taken.test", Backends: []string{held}},
		{Listen: portOf(free) + 3, Host: "ok.test", Backends: []string{free}},
	})
	if err == nil {
		t.Error("expected bind error for occupied listen port")
	}
	if len(mgr.ActivePorts()) != 1 || mgr.ActivePorts()[0] != portOf(free)+3 {
		t.Errorf("healthy route should still be active: %v", mgr.ActivePorts())
	}
}

func portOf(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	port := 0
	for _, r := range portStr {
		port = port*10 + int(r-'0')
	}
	return port
}
