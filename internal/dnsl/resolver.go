package dnsl

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrNotWritable = errors.New("resolver directory not writable; running in fallback mode without system DNS integration")

type Entry struct {
	Suffix string
	Addr   string
}

type ResolverWriter struct {
	Dir string
}

func resolverFileContent(e Entry) []byte {
	return []byte(fmt.Sprintf("nameserver %s\nport %d\n", hostOf(e.Addr), portOf(e.Addr)))
}

func ParseResolverFile(data []byte) (host, port string, ok bool) {
	host, port = "", ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "nameserver":
			host = fields[1]
		case "port":
			port = fields[1]
		}
	}
	return host, port, host != "" && port != ""
}

func (w ResolverWriter) Apply(entries []Entry) ([]Entry, error) {
	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWritable, err)
	}
	var applied []Entry
	for _, e := range entries {
		path := w.pathFor(e.Suffix)
		want := resolverFileContent(e)
		if current, err := os.ReadFile(path); err == nil && matchesEntry(current, e) {
			continue
		}
		tmp, err := os.CreateTemp(w.Dir, "."+e.Suffix+".tmp-*")
		if err != nil {
			return applied, fmt.Errorf("%w: %v", ErrNotWritable, err)
		}
		tmpName := tmp.Name()
		cleanup := func() { _ = os.Remove(tmpName) }
		if _, err := tmp.Write(want); err != nil {
			_ = tmp.Close()
			cleanup()
			return applied, fmt.Errorf("write %s: %w", path, err)
		}
		if err := tmp.Chmod(0o644); err != nil {
			_ = tmp.Close()
			cleanup()
			return applied, err
		}
		if err := tmp.Close(); err != nil {
			cleanup()
			return applied, err
		}
		if err := os.Rename(tmpName, path); err != nil {
			cleanup()
			return applied, fmt.Errorf("swap %s: %w", path, err)
		}
		applied = append(applied, e)
	}
	return applied, nil
}

func (w ResolverWriter) Verify(entries []Entry) (drifted []string, err error) {
	for _, e := range entries {
		path := w.pathFor(e.Suffix)
		current, readErr := os.ReadFile(path)
		if os.IsNotExist(readErr) {
			drifted = append(drifted, e.Suffix+" (missing)")
			continue
		}
		if readErr != nil {
			return drifted, fmt.Errorf("read %s: %w", path, readErr)
		}
		if !matchesEntry(current, e) {
			host, port, _ := ParseResolverFile(current)
			drifted = append(drifted, fmt.Sprintf("%s (has %s:%s, want %s)", e.Suffix, host, port, e.Addr))
		}
	}
	return drifted, nil
}

func (w ResolverWriter) Remove(suffixes []string) error {
	var firstErr error
	for _, sfx := range suffixes {
		path := w.pathFor(sfx)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return firstErr
}

func (w ResolverWriter) List() ([]string, error) {
	dirEntries, err := os.ReadDir(w.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, de := range dirEntries {
		if !de.IsDir() && !strings.HasPrefix(de.Name(), ".") {
			out = append(out, de.Name())
		}
	}
	return out, nil
}

func (w ResolverWriter) pathFor(suffix string) string {
	return filepath.Join(w.Dir, suffixSanitize(suffix))
}

func suffixSanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "*.")
	s = strings.ReplaceAll(s, "..", ".")
	if strings.Contains(s, "/") || strings.Contains(s, "\\") || strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

func matchesEntry(data []byte, e Entry) bool {
	host, port, ok := ParseResolverFile(data)
	if !ok {
		return false
	}
	return host == hostOf(e.Addr) && port == strconv.Itoa(portOf(e.Addr))
}

func hostOf(addr string) string {
	h, _, err := splitAddr(addr)
	if err != nil {
		return addr
	}
	return h
}

func portOf(addr string) int {
	_, p, err := splitAddr(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

func splitAddr(addr string) (string, string, error) {
	return net.SplitHostPort(addr)
}
