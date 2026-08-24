package config

import (
	"strings"
	"testing"
)

func baseConfigWithProject(t *testing.T, mutate func(p *Project)) Config {
	t.Helper()
	cfg := Default()
	cfg.Settings.Ports = Ports{DNS: 15000, HTTP: 15001, HTTPS: 15002, UI: 15003}
	p := Project{
		Domain: "app.test",
		Run:    &RunConfig{Command: "npm run dev", Port: 3000},
		Routes: []Route{{Host: "@", Backends: []string{"localhost:3000"}, HTTPS: true}},
	}
	mutate(&p)
	cfg.Projects = append(cfg.Projects, p)
	return cfg
}

func TestValidateAcceptsServicesAndPlaceholders(t *testing.T) {
	cfg := baseConfigWithProject(t, func(p *Project) {
		p.Services = []Service{
			{Name: "redis", Type: "Redis", Command: "redis-server --port {port}", Port: 0},
			{Name: "extdb", Type: "postgres", Host: "db.internal", Port: 5432, Transport: "tcp"},
			{Name: "sysdns", Type: "dns", Command: "dnscache -p {port}", Transport: "udp", DNS: true},
		}
		p.Routes = append(p.Routes,
			Route{Host: "*", Backends: []string{"localhost:{port}"}},
			Route{Host: "relay", TCP: true, Listen: 40010, Backends: []string{"127.0.0.1:{port:redis}"}},
			Route{Host: "relayu", UDP: true, Listen: 40011, Backends: []string{"127.0.0.1:{port:sysdns}"}},
		)
	})
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateRejectsBadServices(t *testing.T) {
	cases := map[string]func(p *Project){
		"duplicate name": func(p *Project) {
			p.Services = []Service{
				{Name: "a", Command: "x"},
				{Name: "A", Host: "h", Port: 1},
			}
		},
		"command and host": func(p *Project) {
			p.Services = []Service{{Name: "a", Command: "x", Host: "y", Port: 1}}
		},
		"missing endpoint": func(p *Project) {
			p.Services = []Service{{Name: "a"}}
		},
		"external without port": func(p *Project) {
			p.Services = []Service{{Name: "a", Host: "example.com"}}
		},
		"bad transport": func(p *Project) {
			p.Services = []Service{{Name: "a", Command: "x", Transport: "quic"}}
		},
		"bad type label": func(p *Project) {
			p.Services = []Service{{Name: "a", Command: "x", Type: "has space"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(baseConfigWithProject(t, mutate)); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestValidateRoutePathRules(t *testing.T) {
	if err := Validate(baseConfigWithProject(t, func(p *Project) {
		p.Routes[0].Paths = []string{"api/", "/v2"}
	})); err != nil {
		t.Fatalf("normalizable paths should pass: %v", err)
	}

	err := Validate(baseConfigWithProject(t, func(p *Project) {
		p.Routes[0].Paths = []string{"   "}
	}))
	if err == nil || !strings.Contains(err.Error(), "path prefix") {
		t.Fatalf("empty path should fail, got %v", err)
	}
	err = Validate(baseConfigWithProject(t, func(p *Project) {
		p.Routes[0].Paths = []string{"/dup", "/dup"}
	}))
	if err == nil || !strings.Contains(err.Error(), "duplicate path") {
		t.Fatalf("duplicate normalized path should fail, got %v", err)
	}
}

func TestValidateUDPRequiresListen(t *testing.T) {
	err := Validate(baseConfigWithProject(t, func(p *Project) {
		p.Routes = append(p.Routes, Route{Host: "u", UDP: true, Backends: []string{"127.0.0.1:53"}})
	}))
	if err == nil || !strings.Contains(err.Error(), "udp listen port required") {
		t.Fatalf("udp route without listen should fail, got %v", err)
	}
}

func TestEffectiveForceHTTPS(t *testing.T) {
	r := Route{HTTPS: true}
	if r.EffectiveForceHTTPS(false) {
		t.Error("no global, no local → false")
	}
	if !r.EffectiveForceHTTPS(true) {
		t.Error("global default applies to https routes")
	}
	httpRoute := Route{}
	if httpRoute.EffectiveForceHTTPS(true) {
		t.Error("global default must not apply to non-https routes")
	}
}
