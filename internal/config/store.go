package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		cfg, migrated, err := loadConfig(data)
		if err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
		s.cfg = cfg
		fillDefaults(&s.cfg)
		if err := Validate(s.cfg); err != nil {
			return nil, fmt.Errorf("invalid config %s: %w", path, err)
		}
		if migrated {
			if err := s.saveLocked(); err != nil {
				return nil, fmt.Errorf("migrate config %s to v%d: %w", path, CurrentVersion, err)
			}
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
		p := &next.Projects[i]
		if p.Run != nil && p.Run.Command == "" && p.Run.Mode == "" && p.Run.Port == 0 {
			p.Run = nil
		}
		if p.Path != "" {
			p.Path = strings.TrimSpace(p.Path)
		}
		for j := range p.Routes {
			route := &p.Routes[j]
			host, err := NormalizeLabel(route.Host)
			if err != nil {
				return fmt.Errorf("project %d route %d host: %w", i, j, err)
			}
			route.Host = host
			backends := make([]string, 0, len(route.Backends))
			for _, b := range route.Backends {
				backends = append(backends, strings.TrimSpace(b))
			}
			route.Backends = backends
			paths := make([]string, 0, len(route.Paths))
			for _, pref := range route.Paths {
				if norm := NormalizePathPrefix(pref); norm != "" {
					paths = append(paths, norm)
				}
			}
			sort.Strings(paths)
			route.Paths = paths
		}
		for j := range p.Services {
			svc := &p.Services[j]
			name, err := NormalizeLabel(svc.Name)
			if err != nil {
				return fmt.Errorf("project %d service %d name: %w", i, j, err)
			}
			svc.Name = name
			svc.Type = strings.ToLower(strings.TrimSpace(svc.Type))
			svc.Command = strings.TrimSpace(svc.Command)
			svc.Host = strings.ToLower(strings.TrimSpace(svc.Host))
			svc.Transport = strings.ToLower(strings.TrimSpace(svc.Transport))
		}
		for j := range p.Records {
			name, err := NormalizeLabel(p.Records[j].Name)
			if err != nil {
				return fmt.Errorf("project %d record %d name: %w", i, j, err)
			}
			p.Records[j].Name = name
			p.Records[j].Value = strings.TrimSpace(p.Records[j].Value)
		}
		if p.CreatedAt.IsZero() {
			p.CreatedAt = now
		}
		p.UpdatedAt = now
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
	cfg, migrated, err := loadConfig(data)
	if err != nil {
		return fmt.Errorf("parse config %s: %w", s.path, err)
	}
	fillDefaults(&cfg)
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("invalid config %s: %w", s.path, err)
	}
	if migrated {
		s.mu.Lock()
		s.cfg = cfg
		err = s.saveLocked()
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

func (s *Store) snapshotLocked() Config {
	cfg := s.cfg
	cfg.Settings.Upstreams = append([]string(nil), s.cfg.Settings.Upstreams...)
	if len(s.cfg.Projects) > 0 {
		cfg.Projects = make([]Project, len(s.cfg.Projects))
		copy(cfg.Projects, s.cfg.Projects)
		for i := range cfg.Projects {
			p := &cfg.Projects[i]
			if len(p.Routes) > 0 {
				p.Routes = make([]Route, len(p.Routes))
				for j := range p.Routes {
					p.Routes[j] = s.cfg.Projects[i].Routes[j]
					p.Routes[j].Backends = append([]string(nil), s.cfg.Projects[i].Routes[j].Backends...)
				}
			}
			if p.Run != nil {
				run := *p.Run
				p.Run = &run
			}
			if len(p.Services) > 0 {
				p.Services = append([]Service(nil), s.cfg.Projects[i].Services...)
			}
			p.Records = append([]Record(nil), s.cfg.Projects[i].Records...)
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
