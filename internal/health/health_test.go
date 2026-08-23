package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerUpAndDown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := down.URL
	down.Close()

	targets := map[string]string{
		"up.test":   up.URL,
		"down.test": url,
	}
	c := NewChecker(func() map[string]string { return targets }, time.Hour)
	c.poll()
	defer c.Stop()

	snap := c.Snapshot()
	if !snap["up.test"].Up {
		t.Errorf("up.test should be Up: %+v", snap["up.test"])
	}
	if snap["down.test"].Up {
		t.Errorf("down.test should be Down: %+v", snap["down.test"])
	}
	if snap["down.test"].FailCount != 1 {
		t.Errorf("fail count = %d, want 1", snap["down.test"].FailCount)
	}
}

func TestCheckerTracksFailCount(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	targets := map[string]string{"svc.test": ok.URL}
	c := NewChecker(func() map[string]string { return targets }, time.Hour)

	c.poll()
	ok.Close()
	c.poll()
	c.poll()
	defer c.Stop()

	s, _ := c.Get("svc.test")
	if s.Up || s.FailCount != 2 {
		t.Errorf("after close: up=%v fails=%d, want false/2", s.Up, s.FailCount)
	}
}
