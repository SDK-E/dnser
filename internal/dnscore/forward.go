package dnscore

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

const forwardTimeout = 3 * time.Second

type Forwarder struct {
	addrs  []string
	next   atomic.Uint64
	client *dns.Client
	tcpCli *dns.Client
}

func NewForwarder(upstreams []string) (*Forwarder, error) {
	if len(upstreams) == 0 {
		return nil, fmt.Errorf("forwarder: no upstream resolvers configured")
	}
	addrs := make([]string, 0, len(upstreams))
	for _, u := range upstreams {
		addr, err := UpstreamAddr(u)
		if err != nil {
			return nil, fmt.Errorf("upstream %q: %w", u, err)
		}
		addrs = append(addrs, addr)
	}
	udp := &dns.Client{Net: "udp", Timeout: forwardTimeout, UDPSize: 1232}
	tcp := &dns.Client{Net: "tcp", Timeout: forwardTimeout}
	return &Forwarder{
		addrs:  addrs,
		client: udp,
		tcpCli: tcp,
	}, nil
}

func UpstreamAddr(u string) (string, error) {
	u = strings.TrimSpace(u)
	if u == "" {
		return "", fmt.Errorf("empty")
	}
	host, port, err := net.SplitHostPort(u)
	if err != nil {
		return net.JoinHostPort(strings.Trim(u, "[]"), "53"), nil
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("bad port %q", port)
	}
	return net.JoinHostPort(host, port), nil
}

func (f *Forwarder) Forward(query *dns.Msg) (*dns.Msg, error) {
	start := f.next.Add(1)
	offset := int(start-1) % len(f.addrs)
	var lastErr error
	for i := 0; i < len(f.addrs); i++ {
		addr := f.addrs[(offset+i)%len(f.addrs)]
		resp, err := f.tryExchange(query, addr)
		if err == nil {
			resp.RecursionAvailable = true
			return resp, nil
		}
		lastErr = err
		slog.Debug("upstream exchange failed", "addr", addr, "err", err)
	}
	return nil, fmt.Errorf("all %d upstreams failed: %w", len(f.addrs), lastErr)
}

func (f *Forwarder) tryExchange(query *dns.Msg, addr string) (*dns.Msg, error) {
	req := query.Copy()
	req.Id = dns.Id()
	resp, _, err := f.client.Exchange(req, addr)
	if err == nil && resp.Truncated {
		resp, _, err = f.tcpCli.Exchange(req, addr)
	}
	return resp, err
}

func QueryFor(req *dns.Msg) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(req.Question[0].Name, req.Question[0].Qtype)
	q.RecursionDesired = true
	q.Question[0].Qclass = dns.ClassINET
	return q
}
