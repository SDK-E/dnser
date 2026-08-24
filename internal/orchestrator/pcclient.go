package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type ProcState struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	IsRunning bool   `json:"is_running"`
	IsReady   string `json:"is_ready"`
	Pid       int    `json:"pid"`
	ExitCode  int    `json:"exit_code"`
	Age       int    `json:"age"`
}

const (
	StateRunning = "Running"
	StatePending = "Pending"
	StateStopped = "Stopped"
	StateDone    = "Completed"
	StateError   = "Error"

	HealthReady    = "Ready"
	HealthNotReady = "Not Ready"
)

const DefaultClientTimeout = 10 * time.Second

type Client struct {
	http *http.Client
	base string
	tok  string
}

func NewTCPClient(addr, token string) *Client {
	if !strings.Contains(addr, ":") {
		addr = "127.0.0.1:" + addr
	}
	return &Client{
		http: &http.Client{Timeout: DefaultClientTimeout},
		base: "http://" + addr,
		tok:  token,
	}
}

func NewUDSClient(socketPath, token string) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		http: &http.Client{Timeout: 10 * time.Second, Transport: tr},
		base: "http://pc",
		tok:  token,
	}
}

func (c *Client) do(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.tok != "" {
		req.Header.Set("X-PC-Token-Key", c.tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if rerr != nil {
			break
		}
		if len(body) > 1<<20 {
			break
		}
	}
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) Live(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/live")
	return err
}

func (c *Client) GetProcess(ctx context.Context, name string) (*ProcState, error) {
	b, err := c.do(ctx, http.MethodGet, "/process/"+name)
	if err != nil {
		return nil, err
	}
	var st ProcState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("parse process state: %w", err)
	}
	return &st, nil
}

func (c *Client) ListProcesses(ctx context.Context) (map[string]ProcState, error) {
	b, err := c.do(ctx, http.MethodGet, "/processes")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Processes []ProcState `json:"processes"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse process list: %w", err)
	}
	out := make(map[string]ProcState, len(raw.Processes))
	for _, p := range raw.Processes {
		out[p.Name] = p
	}
	return out, nil
}

func (c *Client) Start(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodPost, "/process/start/"+name)
	return err
}

func (c *Client) Stop(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodPost, "/process/stop/"+name)
	return err
}

func (c *Client) Restart(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodPost, "/process/restart/"+name)
	return err
}
