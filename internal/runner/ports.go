package runner

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
)

func AllocateFreePort(exclude map[int]bool) (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("probe free port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return 0, err
	}
	if exclude[port] {
		for attempt := 0; attempt < 50; attempt++ {
			p2, err := allocateAny()
			if err != nil {
				return 0, err
			}
			if !exclude[p2] {
				return p2, nil
			}
		}
		return 0, fmt.Errorf("no free port found outside exclusion set")
	}
	return port, nil
}

func allocateAny() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

var placeholderRe = regexp.MustCompile(`\{port\}`)

var namedPortRe = regexp.MustCompile(`\{port:([A-Za-z0-9_-]+)\}`)

func SubstitutePort(argv []string, port int) []string {
	out := make([]string, len(argv))
	p := strconv.Itoa(port)
	for i, a := range argv {
		out[i] = placeholderRe.ReplaceAllString(a, p)
	}
	return out
}

func SubstitutePortMap(argv []string, port int, named map[string]int) []string {
	if len(named) == 0 {
		return SubstitutePort(argv, port)
	}
	out := make([]string, len(argv))
	p := strconv.Itoa(port)
	for i, a := range argv {
		a = placeholderRe.ReplaceAllString(a, p)
		a = namedPortRe.ReplaceAllStringFunc(a, func(m string) string {
			name := namedPortRe.FindStringSubmatch(m)[1]
			if v, ok := named[name]; ok && v > 0 {
				return strconv.Itoa(v)
			}
			return m
		})
		out[i] = a
	}
	return out
}

func SubstituteBackendStrings(backends []string, named map[string]int) []string {
	if len(named) == 0 {
		return backends
	}
	out := make([]string, len(backends))
	for i, b := range backends {
		out[i] = SubstitutePortMap([]string{b}, named[""], named)[0]
	}
	return out
}
