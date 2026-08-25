package config

import "strings"

func ExpandRecordName(name, primary string) string {
	switch {
	case name == "" || name == "@":
		return primary
	case strings.Contains(name, "."):
		return name
	default:
		return name + "." + primary
	}
}

func (m *Manifest) HasExplicitUpstream() bool {
	for _, r := range m.Routes {
		if r.Port != nil || r.Backend != "" || len(r.Backends) > 0 {
			return true
		}
	}
	return false
}

func (m *Manifest) ServesStaticFiles() bool {
	return m.Type == "static"
}
