package config

import (
	"fmt"
	"strconv"
	"strings"
)

type PlaceholderCtx struct {
	Domain   string
	Port     int
	Services map[string]int
	LogsDir  string
}

func (c PlaceholderCtx) substitute(s string) (string, error) {
	out := s
	for {
		open := strings.Index(out, "{")
		if open < 0 {
			return out, nil
		}
		closeIdx := strings.Index(out[open:], "}")
		if closeIdx < 0 {
			return "", fmt.Errorf("unbalanced '{' in %q", s)
		}
		token := out[open+1 : open+closeIdx]
		replacement, err := c.resolve(token, s)
		if err != nil {
			return "", err
		}
		out = out[:open] + replacement + out[open+closeIdx+1:]
	}
}

func (c PlaceholderCtx) resolve(token, original string) (string, error) {
	switch {
	case token == "port":
		if c.Port == 0 {
			return "", fmt.Errorf("placeholder {port} used but no port resolved in %q", original)
		}
		return strconv.Itoa(c.Port), nil
	case strings.HasPrefix(token, "port:"):
		name := strings.TrimPrefix(token, "port:")
		port, ok := c.Services[name]
		if !ok || port == 0 {
			return "", fmt.Errorf("placeholder {port:%s} references unknown service in %q", name, original)
		}
		return strconv.Itoa(port), nil
	case token == "domain":
		if c.Domain == "" {
			return "", fmt.Errorf("placeholder {domain} used but no domain declared in %q", original)
		}
		return c.Domain, nil
	case token == "logs_dir":
		if c.LogsDir == "" {
			return "", fmt.Errorf("placeholder {logs_dir} used before logs dir known in %q", original)
		}
		return c.LogsDir, nil
	default:
		return "", fmt.Errorf("unknown placeholder {%s} in %q (allowed: port, port:name, domain, logs_dir)", token, original)
	}
}

func SubstituteStrings(v any, ctx PlaceholderCtx) (any, error) {
	switch t := v.(type) {
	case string:
		return ctx.substitute(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			sub, err := SubstituteStrings(val, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = sub
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			sub, err := SubstituteStrings(val, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = sub
		}
		return out, nil
	default:
		return v, nil
	}
}

func SubstituteRaw(raw *RawMap, ctx PlaceholderCtx) (*RawMap, error) {
	if raw == nil {
		return nil, nil
	}
	sub, err := SubstituteStrings(raw.Value, ctx)
	if err != nil {
		return nil, err
	}
	m, ok := sub.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("raw merge must produce a mapping")
	}
	return &RawMap{Value: m}, nil
}
