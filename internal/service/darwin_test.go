//go:build darwin

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderRootPlistPassesDnserHomeNotUserHome(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin", "dnser")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	plist := string(renderRootPlist(bin, filepath.Join(home, ".dnser")))
	marker := "<string>--home</string>"
	idx := strings.Index(plist, marker)
	if idx < 0 {
		t.Fatal("plist must pass --home explicitly")
	}
	rest := plist[idx+len(marker):]
	start := strings.Index(rest, "<string>") + len("<string>")
	end := strings.Index(rest, "</string>")
	got := rest[start:end]
	want := filepath.Join(home, ".dnser")
	if got != want {
		t.Fatalf("daemon home = %q, want %q (a user-home here makes the root daemon mint its own CA)", got, want)
	}
}
