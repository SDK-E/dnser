package config

import (
	"encoding/json"
	"fmt"
	"time"
)

type v1Project struct {
	Domain     string    `json:"domain"`
	Port       int       `json:"port"`
	Wildcard   bool      `json:"wildcard"`
	HTTPS      bool      `json:"https"`
	ForceHTTPS bool      `json:"force_https,omitempty"`
	Aliases    []string  `json:"aliases,omitempty"`
	Records    []Record  `json:"records,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type v1Config struct {
	Version  int         `json:"version"`
	Settings Settings    `json:"settings"`
	Projects []v1Project `json:"projects"`
}

func migrateV1(old v1Config) (Config, error) {
	cfg := Config{
		Version:  CurrentVersion,
		Settings: old.Settings,
	}
	for _, op := range old.Projects {
		p := Project{
			Domain:    op.Domain,
			Records:   op.Records,
			CreatedAt: op.CreatedAt,
			UpdatedAt: op.UpdatedAt,
		}
		if op.Port > 0 {
			backends := []string{fmt.Sprintf("localhost:%d", op.Port)}
			p.Routes = append(p.Routes, Route{
				Host:       "@",
				Backends:   append([]string(nil), backends...),
				HTTPS:      op.HTTPS,
				ForceHTTPS: op.ForceHTTPS,
			})
			if op.Wildcard {
				p.Routes = append(p.Routes, Route{
					Host:       "*",
					Backends:   append([]string(nil), backends...),
					HTTPS:      op.HTTPS,
					ForceHTTPS: op.ForceHTTPS,
				})
			}
		}
		for _, a := range op.Aliases {
			p.Routes = append(p.Routes, Route{
				Host:       a,
				Backends:   []string{fmt.Sprintf("localhost:%d", op.Port)},
				HTTPS:      op.HTTPS,
				ForceHTTPS: op.ForceHTTPS,
			})
		}
		cfg.Projects = append(cfg.Projects, p)
	}
	return cfg, nil
}

func loadConfig(data []byte) (Config, bool, error) {
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Config{}, false, err
	}
	switch probe.Version {
	case 1:
		var old v1Config
		if err := json.Unmarshal(data, &old); err != nil {
			return Config{}, false, err
		}
		cfg, err := migrateV1(old)
		return cfg, true, err
	default:
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, false, err
		}
		return cfg, false, nil
	}
}
