package health

import (
	"net/http"
	"sync"
	"time"
)

type Status struct {
	Up        bool          `json:"up"`
	LatencyMS int64         `json:"latency_ms"`
	CheckedAt time.Time     `json:"checked_at"`
	FailCount int           `json:"fail_count"`
}

type Checker struct {
	mu       sync.RWMutex
	statuses map[string]Status
	targets  func() map[string]string
	client   *http.Client
	interval time.Duration
	stop     chan struct{}
	once     sync.Once
}

func NewChecker(targets func() map[string]string, interval time.Duration) *Checker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Checker{
		statuses: make(map[string]Status),
		targets:  targets,
		client:   &http.Client{Timeout: 2 * time.Second},
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
	for host, target := range all {
		wg.Add(1)
		go func(host, target string) {
			defer wg.Done()
			st := c.probe(target)
			c.mu.Lock()
			prev, ok := c.statuses[host]
			if st.Up {
				st.FailCount = 0
			} else if ok {
				st.FailCount = prev.FailCount + 1
			} else {
				st.FailCount = 1
			}
			c.statuses[host] = st
			c.mu.Unlock()
		}(host, target)
	}
	wg.Wait()
}

func (c *Checker) probe(target string) Status {
	start := time.Now()
	resp, err := c.client.Get(target)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return Status{Up: false, LatencyMS: latency, CheckedAt: time.Now()}
	}
	defer func() { _ = resp.Body.Close() }()
	up := resp.StatusCode < http.StatusInternalServerError
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
