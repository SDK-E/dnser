package dnscore

import (
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	maxCacheTTL    = 300 * time.Second
	minCacheTTL    = 5 * time.Second
	negCacheTTLMax = 60 * time.Second
	cacheKeySep    = "|"
)

type cacheEntry struct {
	msg     *dns.Msg
	expires time.Time
}

type inflight struct {
	done chan struct{}
	msg  *dns.Msg
	err  error
}

type Cache struct {
	mu       sync.Mutex
	entries  map[string]cacheEntry
	inflight map[string]*inflight
	now      func() time.Time
}

func NewCache() *Cache {
	return &Cache{
		entries:  make(map[string]cacheEntry),
		inflight: make(map[string]*inflight),
		now:      time.Now,
	}
}

func (c *Cache) key(name string, qtype uint16) string {
	return name + cacheKeySep + strconv.FormatUint(uint64(qtype), 10)
}

func (c *Cache) Get(name string, qtype uint16) (*dns.Msg, bool) {
	c.mu.Lock()
	e, ok := c.entries[c.key(name, qtype)]
	c.mu.Unlock()
	if !ok || c.now().After(e.expires) {
		return nil, false
	}
	return e.msg.Copy(), true
}

func (c *Cache) Put(name string, qtype uint16, resp *dns.Msg) {
	if resp == nil || len(resp.Question) == 0 {
		return
	}
	if resp.Rcode != dns.RcodeSuccess && resp.Rcode != dns.RcodeNameError {
		return
	}
	ttl := responseTTL(resp)
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[c.key(name, qtype)] = cacheEntry{
		msg:     resp.Copy(),
		expires: c.now().Add(ttl),
	}
}

func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

func (c *Cache) DoSingleFlight(name string, qtype uint16, fetch func() (*dns.Msg, error)) (*dns.Msg, error) {
	k := c.key(name, qtype)
	c.mu.Lock()
	if f, ok := c.inflight[k]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.msg, f.err
		case <-time.After(forwardTimeout * 2):
			return fetch()
		}
	}
	f := &inflight{done: make(chan struct{})}
	c.inflight[k] = f
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inflight, k)
		c.mu.Unlock()
		close(f.done)
	}()

	msg, err := fetch()
	f.msg = msg
	f.err = err
	return msg, err
}

func responseTTL(resp *dns.Msg) time.Duration {
	if resp.Rcode != dns.RcodeSuccess {
		if soa := findSOA(resp.Ns); soa != nil {
			neg := time.Duration(soa.Minttl) * time.Second
			return clampTTL(neg, negCacheTTLMax)
		}
		return 10 * time.Second
	}
	min := uint32(0)
	first := true
	for _, rr := range resp.Answer {
		h := rr.Header()
		switch h.Rrtype {
		case dns.TypeA, dns.TypeAAAA, dns.TypeCNAME, dns.TypeTXT,
			dns.TypeMX, dns.TypeSRV, dns.TypeNS:
		default:
			continue
		}
		if first || h.Ttl < min {
			min = h.Ttl
			first = false
		}
	}
	if first && min == 0 {
		return 0
	}
	return clampTTL(time.Duration(min)*time.Second, maxCacheTTL)
}

func clampTTL(d, cap time.Duration) time.Duration {
	if d < minCacheTTL {
		d = minCacheTTL
	}
	if d > cap {
		d = cap
	}
	return d
}

func findSOA(section []dns.RR) *dns.SOA {
	for _, rr := range section {
		if soa, ok := rr.(*dns.SOA); ok {
			return soa
		}
	}
	return nil
}
