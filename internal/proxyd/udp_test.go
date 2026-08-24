package proxyd

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func startEchoUDP(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(buf[:n], addr)
		}
	}()
	t.Cleanup(func() { _ = conn.Close() })
	return conn.LocalAddr().String()
}

func TestUDPManagerRelaysPackets(t *testing.T) {
	router := NewRouter()
	mgr := NewUDPManager(router)
	defer func() { _ = mgr.Shutdown(t.Context()) }()

	echoAddr := startEchoUDP(t)
	listen := portOf(echoAddr) + 1

	if err := mgr.Apply([]UDPRoute{{Listen: listen, Host: "dns.x.test", Backends: []string{echoAddr}}}); err != nil {
		t.Fatal(err)
	}

	client, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", listen), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	msg := []byte("ping-through-dnser-udp")
	if _, err := client.Write(msg); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, len(msg))
	if _, err := client.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(msg) {
		t.Errorf("echo = %q, want %q", buf, msg)
	}
}

func TestUDPManagerApplyRemovesStaleListeners(t *testing.T) {
	router := NewRouter()
	mgr := NewUDPManager(router)
	defer func() { _ = mgr.Shutdown(t.Context()) }()

	a := startEchoUDP(t)
	b := startEchoUDP(t)

	if err := mgr.Apply([]UDPRoute{{Listen: portOf(a) + 2, Host: "a.test", Backends: []string{a}}}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Apply([]UDPRoute{{Listen: portOf(b) + 2, Host: "b.test", Backends: []string{b}}}); err != nil {
		t.Fatal(err)
	}
	ports := mgr.ActivePorts()
	if len(ports) != 1 || ports[0] != portOf(b)+2 {
		t.Fatalf("stale listener not removed: %v", ports)
	}
}
