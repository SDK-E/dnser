package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (s *Store) Watch(onChange func(Config)) (func(), error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	dir := dirOf(s.Path())
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("watch config dir %s: %w", dir, err)
	}

	s.mu.RLock()
	last := hashConfig(s.cfg)
	s.mu.RUnlock()
	lastMod := modTime(s.Path())

	var (
		stopOnce sync.Once
		done     = make(chan struct{})
		kick     = make(chan struct{}, 1)
	)

	go func() {
		cfgName := filepath.Base(s.Path())
		for {
			select {
			case <-done:
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				base := filepath.Base(ev.Name)
				if base != cfgName && !strings.HasPrefix(base, ".dnser-") {
					continue
				}
				select {
				case kick <- struct{}{}:
				default:
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("config watcher error", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		fallback := time.NewTicker(750 * time.Millisecond)
		defer fallback.Stop()
		pending := false
		for {
			select {
			case <-done:
				return
			case <-kick:
				pending = true
			case <-fallback.C:
				if stat, err := os.Stat(s.Path()); err == nil && !stat.ModTime().Equal(lastMod) {
					lastMod = stat.ModTime()
					pending = true
				}
			case <-ticker.C:
				if !pending {
					continue
				}
				pending = false
				time.Sleep(100 * time.Millisecond)
				select {
				case <-done:
					return
				default:
				}
				if err := s.Reload(); err != nil {
					slog.Warn("config reload failed; keeping previous state", "path", s.Path(), "err", err)
					continue
				}
				cfg := s.Get()
				h := hashConfig(cfg)
				if h == last {
					continue
				}
				last = h
				if onChange != nil {
					onChange(cfg)
				}
			}
		}
	}()

	stop := func() {
		stopOnce.Do(func() {
			close(done)
			_ = watcher.Close()
		})
	}
	return stop, nil
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func hashConfig(c Config) [32]byte {
	data, err := json.Marshal(c)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(data)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}
