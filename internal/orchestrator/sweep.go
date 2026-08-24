package orchestrator

import (
	"fmt"
	"net"
	"os"
	"time"
)

var ErrRootSpawn = fmt.Errorf("refusing to spawn project process as root (I5)")

func AssertUnprivileged() error {
	if os.Geteuid() == 0 {
		return ErrRootSpawn
	}
	return nil
}

type PortProber interface {
	Listening(addr string) bool
}

type dialProber struct {
	timeout time.Duration
}

func NewDialProber(timeout time.Duration) *dialProber {
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	}
	return &dialProber{timeout: timeout}
}

func (d *dialProber) Listening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, d.timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

type StrayRecord struct {
	Project string
	Port    int
	Pid     int
	Killer  func(pid int) error
}

type SweepResult struct {
	Project     string
	Port        int
	WasStray    bool
	Reaped      bool
	PortInUse   bool
	RegistryPid int
	Live        bool
}

func SweepStrays(records []StrayRecord, prober PortProber, alive func(int) bool) []SweepResult {
	results := make([]SweepResult, 0, len(records))
	for _, rec := range records {
		res := SweepResult{Project: rec.Project, Port: rec.Port, RegistryPid: rec.Pid}
		addr := net.JoinHostPort("127.0.0.1", fmt.Sprint(rec.Port))
		res.PortInUse = prober.Listening(addr)
		res.Live = rec.Pid > 0 && alive(rec.Pid)
		res.WasStray = res.PortInUse && !res.Live
		if res.WasStray && rec.Killer != nil && rec.Pid > 0 {
			if err := rec.Killer(rec.Pid); err == nil {
				res.Reaped = true
			}
		}
		results = append(results, res)
	}
	return results
}
