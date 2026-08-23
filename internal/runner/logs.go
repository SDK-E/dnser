package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SDK-E/dnser/internal/logstream"
)

const (
	maxLogSize = 10 << 20
	logBackups = 2
)

type logTee struct {
	mu     sync.Mutex
	dir    string
	stream *logstream.Stream
	files  map[string]*os.File
	sizes  map[string]int64
}

func newLogTee(dir string, stream *logstream.Stream) *logTee {
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return &logTee{dir: dir, stream: stream, files: map[string]*os.File{}, sizes: map[string]int64{}}
}

func (t *logTee) writerFor(domain string) io.Writer {
	return &teeWriter{tee: t, domain: domain}
}

func (t *logTee) emit(domain, line string) {
	t.writeLine(domain, line)
}

func (t *logTee) writeLine(domain, line string) {
	stamped := fmt.Sprintf("%s %s\n", time.Now().UTC().Format("15:04:05"), strings.TrimRight(line, "\n"))
	if t.stream != nil {
		t.stream.Publish(logstream.Event{
			Time:   time.Now(),
			Name:   domain,
			Type:   "app",
			Source: logstream.Source("project"),
			Answer: strings.TrimRight(line, "\n"),
		})
	}
	if t.dir == "" {
		return
	}
	f := t.fileFor(domain)
	if f == nil {
		return
	}
	n, _ := f.WriteString(stamped)
	t.mu.Lock()
	t.sizes[domain] += int64(n)
	rotate := t.sizes[domain] > maxLogSize
	t.mu.Unlock()
	if rotate {
		t.rotate(domain)
	}
}

func (t *logTee) fileFor(domain string) *os.File {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f, ok := t.files[domain]; ok {
		return f
	}
	path := filepath.Join(t.dir, domain+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	if info, err := f.Stat(); err == nil {
		t.sizes[domain] = info.Size()
	}
	t.files[domain] = f
	return f
}

func (t *logTee) rotate(domain string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, ok := t.files[domain]
	if !ok {
		return
	}
	_ = f.Close()
	delete(t.files, domain)
	t.sizes[domain] = 0
	base := filepath.Join(t.dir, domain+".log")
	for i := logBackups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", base, i), fmt.Sprintf("%s.%d", base, i+1))
	}
	_ = os.Rename(base, base+".1")
}

func (t *logTee) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, f := range t.files {
		_ = f.Close()
	}
	t.files = map[string]*os.File{}
}

type teeWriter struct {
	tee    *logTee
	domain string
	buf    []byte
}

func (w *teeWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.tee.writeLine(w.domain, sanitize(line))
		w.buf = w.buf[idx+1:]
	}
	if len(w.buf) > 64<<10 {
		w.tee.writeLine(w.domain, sanitize(string(w.buf)))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func sanitize(s string) string {
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\x1b' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
