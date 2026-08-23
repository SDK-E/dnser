//go:build unix

package runner

import (
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func shellBin() string { return "/bin/sh" }

func shellFlag() string { return "-c" }

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(p *os.Process) {
	if p == nil {
		return
	}
	_ = syscall.Kill(-p.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = p.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		<-done
	}
}

func netDialTimeout(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, 500*time.Millisecond)
}
