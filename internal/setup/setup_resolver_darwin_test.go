//go:build darwin

package setup

import (
	"strings"
	"testing"
)

func TestWriteResolverDomainCommand(t *testing.T) {
	r := NewDryRunner("sudo")
	if err := WriteResolverDomain(r, "test", "127.0.0.1", 35410); err != nil {
		t.Fatalf("write resolver domain: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	if !strings.Contains(full, "osascript") {
		t.Fatalf("expected osascript elevation when sudo unavailable:\n%s", full)
	}
	for _, want := range []string{"mkdir -p /etc/resolver", "nameserver 127.0.0.1", "port 35410", "'/etc/resolver/test'"} {
		if !strings.Contains(full, want) {
			t.Errorf("command missing %q:\n%s", want, full)
		}
	}
}

func TestWriteResolverDomainSudoPath(t *testing.T) {
	r := NewDryRunner()
	if err := WriteResolverDomain(r, "test", "127.0.0.1", 53); err != nil {
		t.Fatalf("write resolver domain: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	if !strings.Contains(full, "sudo") || !strings.Contains(full, "/bin/sh") {
		t.Fatalf("expected sudo path when available:\n%s", full)
	}
}

func TestRemoveResolverFilesCommand(t *testing.T) {
	r := NewDryRunner("sudo")
	if err := RemoveResolverFiles(r, []string{"test", "dev.local"}); err != nil {
		t.Fatalf("remove resolver files: %v", err)
	}
	full := strings.Join(r.Commands(), "\n")
	for _, want := range []string{`rm -f '/etc/resolver/test'`, `rm -f '/etc/resolver/dev.local'`, "rmdir /etc/resolver"} {
		if !strings.Contains(full, want) {
			t.Errorf("commands missing %q:\n%s", want, full)
		}
	}
}
