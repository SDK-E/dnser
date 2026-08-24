package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SDK-E/dnser/internal/journal"
)

func TestChaosKillDuringLinkMutation(t *testing.T) {
	if os.Getenv("DNSER_CHAOS_CHILD") == "" {
		return
	}
	root := os.Getenv("DNSER_CHAOS_ROOT")
	delayMS := 0
	_, _ = fmt.Sscanf(os.Getenv("DNSER_CHAOS_DELAY"), "%d", &delayMS)
	writes := []MutationWrite{
		{Path: filepath.Join(root, "generated", "Caddyfile"), Content: []byte(strings.Repeat("caddy-line\n", 400)), Mode: 0o644},
		{Path: filepath.Join(root, "generated", "process-compose.yaml"), Content: []byte(strings.Repeat("pc-line\n", 400)), Mode: 0o644},
		{Path: os.Getenv("DNSER_CHAOS_TARGET"), Content: []byte("nameserver 127.0.0.1\nport 35353\n"), Mode: 0o644},
	}
	if delayMS > 0 {
		go func() {
			time.Sleep(time.Duration(delayMS) * time.Millisecond)
			os.Exit(137)
		}()
	}
	_, err := applyMutation(context.Background(), "link:chaos-child", writes)
	if err != nil {
		fmt.Println(err)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestChaosRandomizedKillsPreserveInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(20260824))
	oldContent := strings.Repeat("old\n", 500)
	newContent := strings.Repeat("new\n", 500)

	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("iter%d", i), func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "resolver", "chaos.test")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(oldContent), 0o644); err != nil {
				t.Fatal(err)
			}

			delay := rng.Intn(12)
			cmd := exec.Command(os.Args[0], "-test.run=TestChaosKillDuringLinkMutation", "-test.v")
			cmd.Env = append(os.Environ(),
				"DNSER_CHAOS_CHILD=1",
				fmt.Sprintf("DNSER_CHAOS_DELAY=%d", delay),
				fmt.Sprintf("DNSER_CHAOS_ROOT=%s", dir),
				"DNSER_CHAOS_TARGET="+target,
				"HOME="+dir,
			)
			var out strings.Builder
			cmd.Stdout, cmd.Stderr = &out, &out
			startDone := make(chan error, 1)
			waitDone := make(chan error, 1)
			go func() { startDone <- cmd.Start() }()
			if err := <-startDone; err != nil {
				t.Fatal(err)
			}
			go func() { waitDone <- cmd.Wait() }()
			time.Sleep(time.Duration(delay) * time.Millisecond)
			select {
			case err := <-waitDone:
				_ = err
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("chaos child hung")
			}

			data, rerr := os.ReadFile(target)
			if rerr != nil {
				t.Fatalf("I1 violation: target unreadable after crash: %v", rerr)
			}
			got := string(data)
			if got != oldContent && got != newContent && got != "nameserver 127.0.0.1\nport 35353\n" {
				t.Fatalf("torn write detected (browsing would hang): %d bytes of garbage", len(got))
			}

			store, serr := journal.OpenStore(dir)
			if serr != nil {
				t.Fatal(serr)
			}
			plans, lerr := store.List()
			if lerr != nil {
				t.Fatalf("journal unreadable after crash: %v", lerr)
			}
			if len(plans) == 0 {
				t.Logf("iteration %d: child died before first write-ahead; nothing to resume (valid)", i)
				return
			}
			p := plans[0]
			if p.Status != journal.StatusApplied && !journal.HasInterrupted(p) {
				t.Fatalf("journal state neither applied nor resumable: %+v", p.Steps)
			}

			recovered, ferr := journal.Finish(context.Background(), store, p, journal.NewFSRegistry())
			if ferr != nil || p.Status != journal.StatusApplied {
				t.Fatalf("finish did not converge: err=%v status=%s reports=%v", ferr, p.Status, recovered)
			}
			final, _ := os.ReadFile(target)
			if string(final) != "nameserver 127.0.0.1\nport 35353\n" {
				t.Fatalf("converged plan must leave the new content: %d bytes", len(final))
			}

			back, _ := store.Load(p.ID)
			if _, rerr2 := journal.Revert(context.Background(), store, back, journal.NewFSRegistry()); rerr2 != nil {
				t.Fatalf("revert after convergence: %v", rerr2)
			}
			restored, _ := os.ReadFile(target)
			if string(restored) != oldContent {
				t.Fatalf("revert must restore pre-state byte-for-byte (I2): got %d bytes want %d", len(restored), len(oldContent))
			}
		})
	}
}

func TestUpdateChecksumVerification(t *testing.T) {
	payload := "#!/bin/sh\necho new-binary-v2\n"
	sum := sha256.Sum256([]byte(payload))
	checksums := hex.EncodeToString(sum[:]) + "  dnser_darwin_arm64\n"

	fake := &stubFetcher{
		responses: map[string][]byte{
			"https://fake/releases/latest/download/checksums.txt":      []byte(checksums),
			"https://fake/releases/latest/download/dnser_darwin_arm64": []byte(payload),
		},
	}
	result, err := RunUpdate(context.Background(), "/tmp/dnser-self", "https://fake/releases/latest/download", false, fake)
	if err != nil {
		t.Fatal(err)
	}
	if result["checksum"] != "ok" || result["applied"] != false {
		t.Fatalf("dry run wrong: %+v", result)
	}

	bad := &stubFetcher{responses: map[string][]byte{
		"https://bad/releases/latest/download/checksums.txt":      []byte("deadbeef  dnser_darwin_arm64\n"),
		"https://bad/releases/latest/download/dnser_darwin_arm64": []byte(payload),
	}}
	if _, err := RunUpdate(context.Background(), "/tmp/x", "https://bad/releases/latest/download", false, bad); err == nil {
		t.Fatal("checksum mismatch must refuse install")
	} else if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("wrong error: %v", err)
	}

	noEntry := &stubFetcher{responses: map[string][]byte{
		"https://none/releases/latest/download/checksums.txt":      []byte("abc123  other-file\n"),
		"https://none/releases/latest/download/dnser_darwin_arm64": []byte(payload),
	}}
	if _, err := RunUpdate(context.Background(), "/tmp/x", "https://none/releases/latest/download", false, noEntry); err == nil {
		t.Fatal("missing checksum entry must refuse (R10)")
	}

	dir := t.TempDir()
	self := filepath.Join(dir, "bin", "dnser")
	if err := os.MkdirAll(filepath.Dir(self), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self, []byte("#!/bin/sh\necho v1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	good, uerr := RunUpdate(context.Background(), self, "https://fake/releases/latest/download", true, fake)
	if uerr != nil {
		t.Fatal(uerr)
	}
	if good["path"] != self {
		t.Fatalf("manual/script installs replace self at %s: %+v", self, good)
	}
	after, rerr := os.ReadFile(self)
	if rerr != nil || string(after) != payload {
		t.Fatalf("binary not replaced atomically: %v", rerr)
	}
	info, _ := os.Stat(self)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("exec perms lost on replace: %v", info.Mode().Perm())
	}
}

type stubFetcher struct{ responses map[string][]byte }

func (s *stubFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	if data, ok := s.responses[url]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("no stub for %s", url)
}
