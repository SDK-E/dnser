package health

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

type Status struct {
	Up        bool      `json:"up"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	FailCount int       `json:"fail_count"`
}

type Probe struct {
	URL  string
	Dial bool
}

type Checker struct {
	mu       sync.RWMutex
	statuses map[string]Status
	targets  func() map[string]Probe
	client   *http.Client
	dialer   *net.Dialer
	interval time.Duration
	stop     chan struct{}
	once     sync.Once
}

func NewChecker(targets func() map[string]Probe, interval time.Duration) *Checker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Checker{
		statuses: make(map[string]Status),
		targets:  targets,
		client:   &http.Client{Timeout: 2 * time.Second},
		dialer:   &net.Dialer{Timeout: 2 * time.Second},
		interval: interval,
		stop:     make(chan struct{}),
	}
}

func (c *Checker) Start() {
	go c.loop()
	c.poll()
}

func (c *Checker) Stop() {
	c.once.Do(func() { close(c.stop) })
}

func (c *Checker) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.poll()
		}
	}
}

func (c *Checker) poll() {
	if c.targets == nil {
		return
	}
	all := c.targets()
	var wg sync.WaitGroup
	for backend, probe := range all {
		wg.Add(1)
		go func(backend string, probe Probe) {
			defer wg.Done()
			st := c.probe(probe)
			c.mu.Lock()
			prev, ok := c.statuses[backend]
			if st.Up {
				st.FailCount = 0
			} else if ok {
				st.FailCount = prev.FailCount + 1
			} else {
				st.FailCount = 1
			}
			c.statuses[backend] = st
			c.mu.Unlock()
		}(backend, probe)
	}
	wg.Wait()
}

func (c *Checker) probe(p Probe) Status {
	start := time.Now()
	var up bool
	if p.Dial {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		conn, err := c.dialer.DialContext(ctx, "tcp", p.URL)
		if err == nil {
			_ = conn.Close()
			up = true
		}
	} else {
		resp, err := c.client.Get(p.URL)
		if err == nil {
			_ = resp.Body.Close()
			up = resp.StatusCode < http.StatusInternalServerError
		}
	}
	latency := time.Since(start).Milliseconds()
	return Status{Up: up, LatencyMS: latency, CheckedAt: time.Now()}
}

func (c *Checker) Snapshot() map[string]Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]Status, len(c.statuses))
	for k, v := range c.statuses {
		out[k] = v
	}
	return out
}

func (c *Checker) Get(host string) (Status, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.statuses[host]
	return s, ok
}
