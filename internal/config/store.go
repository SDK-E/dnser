package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func DefaultDir() (string, error) {
	if override := os.Getenv("DNSER_HOME"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".dnser"), nil
}

func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dnser.json"), nil
}

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		s.cfg = Default()
		if err := s.saveLocked(); err != nil {
			return nil, fmt.Errorf("create default config at %s: %w", path, err)
		}
	case err != nil:
		return nil, fmt.Errorf("read config %s: %w", path, err)
	default:
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
		fillDefaults(&s.cfg)
		if err := Validate(s.cfg); err != nil {
			return nil, fmt.Errorf("invalid config %s: %w", path, err)
		}
	}
	return s, nil
}

func OpenDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

func fillDefaults(c *Config) {
	def := DefaultSettings()
	if c.Settings.TLD == "" {
		c.Settings.TLD = def.TLD
	}
	if c.Settings.Bind == "" {
		c.Settings.Bind = def.Bind
	}
	if len(c.Settings.Upstreams) == 0 {
		c.Settings.Upstreams = append([]string(nil), def.Upstreams...)
	}
	if c.Settings.Ports == (Ports{}) {
		c.Settings.Ports = def.Ports
	} else {
		if c.Settings.Ports.DNS == 0 {
			c.Settings.Ports.DNS = def.Ports.DNS
		}
		if c.Settings.Ports.HTTP == 0 {
			c.Settings.Ports.HTTP = def.Ports.HTTP
		}
		if c.Settings.Ports.HTTPS == 0 {
			c.Settings.Ports.HTTPS = def.Ports.HTTPS
		}
		if c.Settings.Ports.UI == 0 {
			c.Settings.Ports.UI = def.Ports.UI
		}
	}
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Settings
}

func (s *Store) Projects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, len(s.cfg.Projects))
	copy(out, s.cfg.Projects)
	return out
}

func (s *Store) FindProject(domain string) (Project, bool) {
	norm, err := NormalizeDomain(domain)
	if err != nil {
		return Project{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.cfg.Projects {
		if p.Domain == norm {
			return p, true
		}
	}
	return Project{}, false
}

func (s *Store) Update(mutate func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.snapshotLocked()
	mutate(&next)
	now := time.Now().UTC().Truncate(time.Second)
	for i := range next.Projects {
		domain, err := NormalizeDomain(next.Projects[i].Domain)
		if err != nil {
			return fmt.Errorf("project %d domain: %w", i, err)
		}
		next.Projects[i].Domain = domain
		aliases := next.Projects[i].Aliases[:0:0]
		for _, a := range next.Projects[i].Aliases {
			norm, err := NormalizeDomain(a)
			if err != nil {
				return fmt.Errorf("project %d alias %q: %w", i, a, err)
			}
			aliases = append(aliases, norm)
		}
		next.Projects[i].Aliases = aliases
		for j := range next.Projects[i].Records {
			name, err := NormalizeLabel(next.Projects[i].Records[j].Name)
			if err != nil {
				return fmt.Errorf("project %d record %d name: %w", i, j, err)
			}
			next.Projects[i].Records[j].Name = name
			next.Projects[i].Records[j].Value = strings.TrimSpace(next.Projects[i].Records[j].Value)
		}
		if next.Projects[i].CreatedAt.IsZero() {
			next.Projects[i].CreatedAt = now
		}
		next.Projects[i].UpdatedAt = now
	}
	if err := Validate(next); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	s.cfg = next
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Reload() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", s.path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", s.path, err)
	}
	fillDefaults(&cfg)
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("invalid config %s: %w", s.path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	return nil
}

func (s *Store) snapshotLocked() Config {
	cfg := s.cfg
	cfg.Settings.Upstreams = append([]string(nil), s.cfg.Settings.Upstreams...)
	if len(s.cfg.Projects) > 0 {
		cfg.Projects = make([]Project, len(s.cfg.Projects))
		copy(cfg.Projects, s.cfg.Projects)
		for i := range cfg.Projects {
			cfg.Projects[i].Aliases = append([]string(nil), s.cfg.Projects[i].Aliases...)
			cfg.Projects[i].Records = append([]Record(nil), s.cfg.Projects[i].Records...)
		}
	}
	return cfg
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".dnser-*.json.tmp")
	if err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(s.path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace config %s: %w", s.path, err)
	}
	tmpName = ""
	return nil
}
