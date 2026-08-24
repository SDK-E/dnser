package state

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type ProjectState struct {
	Port         int            `json:"port,omitempty"`
	ServicePorts map[string]int `json:"service_ports,omitempty"`
}

type Store struct {
	path string
	data Data
}

type Data struct {
	Version  int                       `json:"version"`
	Projects map[string]*ProjectState  `json:"projects"`
	Linked   map[string]*LinkedProject `json:"linked,omitempty"`
}

const currentVersion = 1

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".dnser", "state.json"), nil
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: Data{Version: currentVersion, Projects: map[string]*ProjectState{}}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.data.Projects == nil {
		s.data.Projects = map[string]*ProjectState{}
	}
	return s, nil
}

func (s *Store) project(key string) *ProjectState {
	p, ok := s.data.Projects[key]
	if !ok {
		p = &ProjectState{ServicePorts: map[string]int{}}
		s.data.Projects[key] = p
	}
	if p.ServicePorts == nil {
		p.ServicePorts = map[string]int{}
	}
	return p
}

func (s *Store) Port(projectKey string) (int, bool) {
	p, ok := s.data.Projects[projectKey]
	if !ok || p.Port == 0 {
		return 0, false
	}
	return p.Port, true
}

func (s *Store) ServicePort(projectKey, service string) (int, bool) {
	p, ok := s.data.Projects[projectKey]
	if !ok {
		return 0, false
	}
	port, ok := p.ServicePorts[service]
	return port, ok && port != 0
}

func (s *Store) AllocatePort(projectKey string, preferred int) (int, error) {
	p := s.project(projectKey)
	if p.Port != 0 && portFree(p.Port) {
		return p.Port, nil
	}
	port, err := pickFree(preferred, s.usedPorts())
	if err != nil {
		return 0, err
	}
	p.Port = port
	if err := s.Save(); err != nil {
		return 0, err
	}
	return port, nil
}

func (s *Store) AllocateServicePort(projectKey, service string, preferred int) (int, error) {
	p := s.project(projectKey)
	if existing, ok := p.ServicePorts[service]; ok && existing != 0 && portFree(existing) {
		return existing, nil
	}
	port, err := pickFree(preferred, s.usedPorts())
	if err != nil {
		return 0, err
	}
	p.ServicePorts[service] = port
	if err := s.Save(); err != nil {
		return 0, err
	}
	return port, nil
}

func (s *Store) usedPorts() []int {
	var used []int
	for _, p := range s.data.Projects {
		if p.Port != 0 {
			used = append(used, p.Port)
		}
		for _, sp := range p.ServicePorts {
			if sp != 0 {
				used = append(used, sp)
			}
		}
	}
	sort.Ints(used)
	return used
}

func (s *Store) RemoveProject(projectKey string) error {
	delete(s.data.Projects, projectKey)
	return s.Save()
}

func (s *Store) Save() error {
	s.data.Version = currentVersion
	out, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	out = append(out, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".state.json.tmp-*")
	if err != nil {
		return fmt.Errorf("temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

func portFree(port int) bool {
	conn, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func pickFree(preferred int, used []int) (int, error) {
	if preferred > 0 && preferred <= 65535 && portFree(preferred) && !containsInt(used, preferred) {
		return preferred, nil
	}
	base := 35000
	for port := base; port <= 65535; port++ {
		if containsInt(used, port) {
			continue
		}
		if portFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range %d-65535", base)
}

func containsInt(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
