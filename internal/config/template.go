package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

//go:embed templates/*.yaml
var embeddedTemplates embed.FS

type DetectionHints struct {
	Files       []string `yaml:"files,omitempty"`
	PortEnvKeys []string `yaml:"port_env_keys,omitempty"`
	DefaultPort int      `yaml:"default_port,omitempty"`
}

type Template struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description,omitempty"`
	Detect       DetectionHints    `yaml:"detect,omitempty"`
	Command      string            `yaml:"command,omitempty"`
	Shell        Shell             `yaml:"shell,omitempty"`
	Availability string            `yaml:"availability,omitempty"`
	Env          map[string]string `yaml:"env,omitempty"`
}

func DecodeTemplate(data []byte) (*Template, error) {
	t := &Template{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(t); err != nil {
		return nil, fmt.Errorf("decode template: %w", err)
	}
	if strings.TrimSpace(t.Name) == "" {
		return nil, fmt.Errorf("template missing name")
	}
	return t, nil
}

func RegistryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".dnser", "templates"), nil
}

func LoadRegistry() (map[string]*Template, error) {
	registry := map[string]*Template{}
	entries, err := embeddedTemplates.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := embeddedTemplates.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", e.Name(), err)
		}
		t, err := DecodeTemplate(data)
		if err != nil {
			return nil, fmt.Errorf("embedded template %s: %w", e.Name(), err)
		}
		registry[t.Name] = t
	}
	dir, err := RegistryDir()
	if err != nil {
		return registry, nil
	}
	userEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range userEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", e.Name(), err)
		}
		t, err := DecodeTemplate(data)
		if err != nil {
			return nil, fmt.Errorf("user template %s: %w", e.Name(), err)
		}
		registry[t.Name] = t
	}
	return registry, nil
}

func GetTemplate(name string) (*Template, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	t, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown type %q (known types: %s)", name, strings.Join(KnownTypes(registry), ", "))
	}
	return t, nil
}

func KnownTypes(registry map[string]*Template) []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
