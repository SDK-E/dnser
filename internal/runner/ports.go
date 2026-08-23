package runner

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

func SubstitutePort(argv []string, port int) []string {
	out := make([]string, len(argv))
	p := strconv.Itoa(port)
	for i, a := range argv {
		out[i] = placeholderRe.ReplaceAllString(a, p)
	}
	return out
}

const dotDnserFile = ".dnser.yaml"

type LinkOverride struct {
	Command string
}

func ReadLinkOverride(dir string) (LinkOverride, bool) {
	data, err := os.ReadFile(filepath.Join(dir, dotDnserFile))
	if err != nil {
		return LinkOverride{}, false
	}
	var cmd LinkOverride
	inCommand := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if strings.HasPrefix(line, "command:") && !strings.Contains(line[:7], "#") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "command:"))
			value = strings.Trim(value, `"'`)
			if value != "" && !strings.HasPrefix(value, "#") {
				cmd.Command = value
				return cmd, true
			}
			inCommand = true
			continue
		}
		if inCommand {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if line[0] == ' ' || line[0] == '\t' || line[0] == '-' {
				cmd.Command = strings.Trim(trimmed, `"'`)
				return cmd, true
			}
			inCommand = false
		}
	}
	return LinkOverride{}, false
}
