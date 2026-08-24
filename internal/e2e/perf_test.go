package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type perfSample struct {
	RSSKB uint64
	CPUS  float64
}

func sampleProcess(pid int) (perfSample, error) {
	out, err := exec.Command("ps", "-o", "rss=,cputime=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return perfSample{}, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return perfSample{}, fmt.Errorf("ps output short: %q", out)
	}
	rss, perr := strconv.ParseUint(fields[0], 10, 64)
	if perr != nil {
		return perfSample{}, perr
	}
	cpuParts := strings.Split(fields[1], ":")
	var cpuS float64
	mult := 1.0
	for i := len(cpuParts) - 1; i >= 0; i-- {
		var v float64
		if _, err := fmt.Sscanf(cpuParts[i], "%f", &v); err == nil {
			cpuS += v * mult
		}
		mult *= 60
	}
	return perfSample{RSSKB: rss, CPUS: cpuS}, nil
}

func TestPerfBudgetIdleDashboard(t *testing.T) {
	if testing.Short() {
		t.Skip("perf budget: skipped in -short")
	}
	s := newSandbox(t)
	var childOut strings.Builder
	cmd := exec.Command(s.Bin, "dashboard", "--port", strconv.Itoa(freePort(t)))
	cmd.Env = append(os.Environ(), "HOME="+s.Home)
	cmd.Stdout = &childOut
	cmd.Stderr = &childOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	time.Sleep(1500 * time.Millisecond)

	first, err := sampleProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("cannot sample process: %v; child said: %s", err, childOut.String())
	}
	time.Sleep(2 * time.Second)
	last, err := sampleProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("process exited early; child said: %s", childOut.String())
	}

	const orchestratorBudgetMB = 40
	rssMB := last.RSSKB / 1024
	cpuDelta := last.CPUS - first.CPUS
	if rssMB > orchestratorBudgetMB {
		t.Fatalf("PERF BUDGET VIOLATED (component reopen per RFC 001 §11.2): dnser idle RSS %d MB > %d MB", rssMB, orchestratorBudgetMB)
	}
	if cpuDelta > 0.2 {
		t.Fatalf("PERF BUDGET VIOLATED: idle CPU %.2fs over 2s window (>0.2s)", cpuDelta)
	}
	t.Logf("perf budget OK: RSS=%dMB cpuΔ=%.3fs (budget ≤%dMB ≈0%%)", rssMB, cpuDelta, orchestratorBudgetMB)
}

func TestDeletionDayLedgerFinalLOC(t *testing.T) {
	root := repoRoot(t)
	var total int
	perPkg := map[string]int{}
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		n := strings.Count(string(data), "\n")
		total += n
		pkg := filepath.Base(filepath.Dir(path))
		perPkg[pkg] += n
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"proxyd", "certs", "dnscore", "runner", "logstream"} {
		if _, exists := perPkg[banned]; exists {
			t.Fatalf("DELETION DAY VIOLATION: v1 package %q resurrected", banned)
		}
	}
	if _, exists := perPkg["service"]; exists {
		t.Fatalf("DELETION DAY VIOLATION: v1 service renderers resurrected")
	}
	const target = 3000
	if total > target*2 {
		t.Logf("ledger note: non-test Go LOC=%d exceeds RFC target ~%d (informational, not a gate)", total, target)
	} else {
		t.Logf("ledger: internal/ non-test Go LOC=%d (RFC 001 §11.1 target ~%d)", total, target)
	}
}
