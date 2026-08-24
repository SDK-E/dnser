package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllocatePortStableAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := s1.AllocatePort("/proj", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Port("/proj")
	if !ok || got != p1 {
		t.Fatalf("port not stable across reload: %d vs %d", got, p1)
	}
}

func TestAllocatePortPrefersPreferred(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	port, err := s.AllocatePort("/proj", 45999)
	if err != nil {
		t.Fatal(err)
	}
	if port != 45999 {
		t.Fatalf("preferred port should be honored when free, got %d", port)
	}
}

func TestServicePortsAllocatedOnceAndPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s1, _ := Open(path)
	sp1, err := s1.AllocateServicePort("/proj", "redis", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sp2, ok := s2.ServicePort("/proj", "redis")
	if !ok || sp2 != sp1 {
		t.Fatalf("service port unstable: %d vs %d", sp2, sp1)
	}
}

func TestUsedPortsNotReallocated(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	a, err := s.AllocatePort("/one", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AllocatePort("/two", a)
	if err != nil {
		t.Fatal(err)
	}
	if b == a {
		t.Fatalf("allocated same port to two projects: %d", b)
	}
}

func TestRemoveProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Open(path)
	if _, err := s.AllocatePort("/gone", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProject("/gone"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Port("/gone"); ok {
		t.Fatalf("project should be removed")
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("state file must be 0600, got %v", st.Mode().Perm())
	}
}

func TestOpenMissingFileStartsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Port("/anything"); ok {
		t.Fatalf("fresh store must be empty")
	}
}
