package state

import (
	"fmt"
	"sort"
)

type LinkedProject struct {
	Name         string         `json:"name"`
	Dir          string         `json:"dir"`
	Domain       string         `json:"domain"`
	Port         int            `json:"port"`
	Availability string         `json:"availability"`
	ServicePorts map[string]int `json:"service_ports,omitempty"`
}

func (s *Store) Link(p LinkedProject) error {
	if p.Name == "" {
		return fmt.Errorf("project name required")
	}
	if s.data.Linked == nil {
		s.data.Linked = map[string]*LinkedProject{}
	}
	existing := s.data.Linked[p.Name]
	if existing == nil {
		existing = &LinkedProject{Name: p.Name}
		s.data.Linked[p.Name] = existing
	}
	*existing = p
	return s.Save()
}

func (s *Store) Unlink(name string) (bool, error) {
	if s.data.Linked == nil {
		return false, nil
	}
	if _, ok := s.data.Linked[name]; !ok {
		return false, nil
	}
	delete(s.data.Linked, name)
	return true, s.Save()
}

func (s *Store) Linked(name string) (LinkedProject, bool) {
	if s.data.Linked == nil {
		return LinkedProject{}, false
	}
	p, ok := s.data.Linked[name]
	if !ok || p == nil {
		return LinkedProject{}, false
	}
	return *p, true
}

func (s *Store) ListLinked() []LinkedProject {
	out := make([]LinkedProject, 0, len(s.data.Linked))
	for _, p := range s.data.Linked {
		if p != nil {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
