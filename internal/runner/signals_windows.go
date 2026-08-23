//go:build windows

package runner

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func setSysProcAttr(cmd *exec.Cmd) {
}

func shellBin() string { return "cmd" }

func shellFlag() string { return "/C" }

func killTree(p *os.Process) {
	if p == nil {
		return
	}
	kill := exec.Command("taskkill", "/PID", strconv.Itoa(p.Pid), "/T", "/F")
	_ = kill.Run()
}

func netDialTimeout(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, 500*time.Millisecond)
}
