package logstream

import (
	"sync"
	"time"
)

type Source string

const (
	SourceLocal   Source = "local"
	SourceForward Source = "forward"
	SourceCache   Source = "cache"
	SourceError   Source = "error"
)

type Event struct {
	Time    time.Time     `json:"time"`
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Source  Source        `json:"source"`
	Answer  string        `json:"answer"`
	Latency time.Duration `json:"latency_ns"`
}

const DefaultCapacity = 2048

type Stream struct {
	mu      sync.Mutex
	buf     []Event
	head    int
	count   int
	subs    map[chan Event]struct{}
	dropped uint64
}

func New(capacity int) *Stream {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Stream{
		buf:  make([]Event, capacity),
		subs: make(map[chan Event]struct{}),
	}
}

func (s *Stream) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	s.mu.Lock()
	s.buf[s.head] = e
	s.head = (s.head + 1) % len(s.buf)
	if s.count < len(s.buf) {
		s.count++
	}
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
			s.dropped++
		}
	}
	s.mu.Unlock()
}

func (s *Stream) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 64
	}
	ch := make(chan Event, buffer)
	var once sync.Once
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	unsub := func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, ch)
			s.mu.Unlock()
			close(ch)
		})
	}
	return ch, unsub
}

func (s *Stream) Recent(n int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > s.count {
		n = s.count
	}
	out := make([]Event, n)
	start := (s.head - n + len(s.buf)) % len(s.buf)
	for i := 0; i < n; i++ {
		out[i] = s.buf[(start+i)%len(s.buf)]
	}
	return out
}

func (s *Stream) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}
