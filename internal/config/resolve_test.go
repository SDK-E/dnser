package config

import (
	"reflect"
	"testing"
)

func TestPlaceholderSubstitution(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		ctx     PlaceholderCtx
		want    string
		wantErr bool
	}{
		{
			name: "primary port",
			in:   "npm run dev -- --port {port}",
			ctx:  PlaceholderCtx{Port: 3000, Domain: "a.test"},
			want: "npm run dev -- --port 3000",
		},
		{
			name: "service port",
			in:   "redis-server --port {port:redis}",
			ctx:  PlaceholderCtx{Port: 3000, Services: map[string]int{"redis": 16379}},
			want: "redis-server --port 16379",
		},
		{
			name: "domain and logs dir",
			in:   "serve --host {domain} --log {logs_dir}/x.log",
			ctx:  PlaceholderCtx{Port: 1, Domain: "a.test", LogsDir: "/logs"},
			want: "serve --host a.test --log /logs/x.log",
		},
		{
			name:    "missing service",
			in:      "{port:db}",
			ctx:     PlaceholderCtx{Port: 3000, Services: map[string]int{"redis": 1}},
			wantErr: true,
		},
		{
			name:    "unresolved primary port",
			in:      "{port}",
			ctx:     PlaceholderCtx{},
			wantErr: true,
		},
		{
			name:    "unknown token",
			in:      "{wtf}",
			ctx:     PlaceholderCtx{Port: 3000},
			wantErr: true,
		},
		{
			name:    "unbalanced braces",
			in:      "cmd {port",
			ctx:     PlaceholderCtx{Port: 3000},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ctx.substitute(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestSubstituteStringsNested(t *testing.T) {
	ctx := PlaceholderCtx{Port: 3000, Domain: "a.test", LogsDir: "/l"}
	in := map[string]any{
		"log": map[string]any{"output": "file", "file": "{logs_dir}/access.log"},
		"arr": []any{"{domain}", 42, true},
	}
	out, err := SubstituteStrings(in, ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["log"].(map[string]any)["file"] != "/l/access.log" {
		t.Fatalf("nested map not substituted: %#v", m)
	}
	arr := m["arr"].([]any)
	if arr[0] != "a.test" || arr[1] != 42 || arr[2] != true {
		t.Fatalf("array handling wrong: %#v", arr)
	}
}

func TestPrecedenceFlagBeatsManifestBeatsTemplate(t *testing.T) {
	port := 7777
	manifest := &Manifest{
		Type:    "nodejs",
		Domain:  "manifest.test",
		Port:    intPtr(3000),
		Command: "from-manifest",
		Env:     map[string]string{"NODE_OPTIONS": "user-set", "ONLY_MANIFEST": "yes"},
		Shell:   Shell{Enabled: false, Set: true},
	}
	tmpl := &Template{
		Name:         "nodejs",
		Command:      "from-template",
		Availability: "on_request",
		Shell:        Shell{Enabled: true, Set: true},
		Env:          map[string]string{"NODE_OPTIONS": "template-default", "ONLY_TEMPLATE": "yes"},
	}
	flags := FlagOverrides{}

	eff, err := ResolveEffective(manifest, tmpl, flags)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Command.Value != "from-manifest" || eff.Command.Source != SourceManifest {
		t.Fatalf("command precedence wrong: %+v", eff.Command)
	}
	if eff.Port.Value != 3000 || eff.Port.Source != SourceManifest {
		t.Fatalf("port precedence wrong: %+v", eff.Port)
	}
	if eff.Availability.Value != "on_request" || eff.Availability.Source != SourceTemplate {
		t.Fatalf("availability should come from template: %+v", eff.Availability)
	}
	if eff.EnvValues["NODE_OPTIONS"] != "user-set" || eff.EnvSources["NODE_OPTIONS"] != SourceManifest.String() {
		t.Fatalf("env precedence wrong: %+v", eff.EnvSources)
	}
	if eff.EnvValues["ONLY_TEMPLATE"] != "yes" || eff.EnvValues["ONLY_MANIFEST"] != "yes" {
		t.Fatalf("merge incomplete: %+v", eff.EnvValues)
	}
	if eff.Shell.Enabled {
		t.Fatalf("explicit manifest shell:false must win over template")
	}

	flags = FlagOverrides{Port: &port, Command: "from-flag", Type: "go"}
	eff, err = ResolveEffective(manifest, tmpl, flags)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Port.Value != 7777 || eff.Port.Source != SourceFlag {
		t.Fatalf("flag port must win: %+v", eff.Port)
	}
	if eff.Command.Value != "from-flag" {
		t.Fatalf("flag command must win")
	}
	if eff.Type.Value != "go" || eff.Type.Source != SourceFlag {
		t.Fatalf("flag type must win")
	}

	eff, err = ResolveEffective(&Manifest{}, tmpl, FlagOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Shell.Enabled != true {
		t.Fatalf("default shell=true expected, got %+v", eff.Shell)
	}
}

func TestInjectRuntimeEnv(t *testing.T) {
	svcPort := 5555
	eff := &EffectiveConfig{
		Domain:     ResolvedValue[string]{Value: "app.test"},
		Services:   map[string]Service{"db": {Host: "", Port: &svcPort}},
		EnvValues:  map[string]string{},
		EnvSources: map[string]string{},
	}
	eff.InjectRuntimeEnv(4321)
	if eff.EnvValues["PORT"] != "4321" {
		t.Fatalf("PORT not injected: %+v", eff.EnvValues)
	}
	if eff.EnvValues["DNSER_DOMAIN"] != "app.test" {
		t.Fatalf("DNSER_DOMAIN not injected")
	}
	if eff.EnvValues["DNSER_SERVICES_DB"] != "127.0.0.1:5555" {
		t.Fatalf("DNSER_SERVICES_DB wrong: %q", eff.EnvValues["DNSER_SERVICES_DB"])
	}

	userPort := "9999"
	eff2 := &EffectiveConfig{
		EnvValues:  map[string]string{"PORT": userPort},
		EnvSources: map[string]string{},
	}
	eff2.InjectRuntimeEnv(4321)
	if eff2.EnvValues["PORT"] != userPort {
		t.Fatalf("user-set PORT must not be overridden")
	}
}

func TestPrimaryDomainFallbacks(t *testing.T) {
	tests := []struct {
		m       Manifest
		dirname string
		want    string
	}{
		{Manifest{Domain: "d.test"}, "proj", "d.test"},
		{Manifest{Domains: []string{"one.test", "two.test"}}, "proj", "one.test"},
		{Manifest{}, "my_app", "my-app.test"},
	}
	for _, tt := range tests {
		if got := tt.m.PrimaryDomain(tt.dirname); got != tt.want {
			t.Fatalf("PrimaryDomain(%q) = %q, want %q", tt.dirname, got, tt.want)
		}
	}
}

func TestEffectiveNamesDedupesAndOrders(t *testing.T) {
	m := Manifest{
		Domain:  "root.test",
		Domains: []string{"extra.test", "root.test"},
		Aliases: []string{"alias.test", "extra.test"},
	}
	got := m.EffectiveNames()
	want := []string{"root.test", "extra.test", "alias.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func intPtr(i int) *int {
	return &i
}
