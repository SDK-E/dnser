package logstream

import (
	"sync"
	"testing"
	"time"
)

func TestPublishRecentRing(t *testing.T) {
	s := New(4)
	for i := 0; i < 6; i++ {
		s.Publish(Event{Name: "n", Answer: string(rune('a' + i))})
	}
	recent := s.Recent(4)
	if len(recent) != 4 {
		t.Fatalf("recent len = %d, want 4", len(recent))
	}
	if recent[0].Answer != "c" || recent[3].Answer != "f" {
		t.Errorf("ring order wrong: %v", recent)
	}
	if s.Len() != 4 {
		t.Errorf("len = %d, want 4", s.Len())
	}
}

func TestSubscribeBroadcast(t *testing.T) {
	s := New(8)
	ch1, unsub1 := s.Subscribe(8)
	ch2, unsub2 := s.Subscribe(8)

	s.Publish(Event{Name: "x"})
	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Name != "x" {
				t.Errorf("wrong event %v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive event")
		}
	}

	unsub1()
	unsub1()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unsubscribe must be idempotent, got panic: %v", r)
			}
		}()
		unsub1()
	}()

	s.Publish(Event{Name: "y"})
	select {
	case ev := <-ch2:
		if ev.Name != "y" {
			t.Errorf("wrong event %v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("second subscriber did not receive event")
	}
	unsub2()
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	s := New(8)
	_, unsub := s.Subscribe(1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.Publish(Event{Name: "burst"})
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}
	unsub()
}
