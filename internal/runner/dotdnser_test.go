package runner

import (
	"reflect"
	"testing"
)

func TestParseDotDnserFullDocument(t *testing.T) {
	src := `
# project manifest
command: pnpm dev --port {port}

services:
  redis:
    type: redis
    command: redis-server --port {port}
    transport: tcp
  worker:
    command: node worker.js --queue {port:redis}
routes:
  - host: api
    paths: [/api, /v2]
    https: true
    force_https: true
    backends:
      - 127.0.0.1:{port}
      - 10.0.0.5:9000
  - host: dbx
    tcp: true
    listen: 55432
    backends: [127.0.0.1:{port:redis}]
`
	doc, err := ParseDotDnser(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Command != "pnpm dev --port {port}" {
		t.Errorf("command = %q", doc.Command)
	}
	if len(doc.Services) != 2 {
		t.Fatalf("services = %+v", doc.Services)
	}
	byName := map[string]DotService{}
	for _, s := range doc.Services {
		byName[s.Name] = s
	}
	if s := byName["redis"]; s.Type != "redis" || s.Command != "redis-server --port {port}" || s.Transport != "tcp" {
		t.Errorf("redis service = %+v", s)
	}
	if s := byName["worker"]; s.Command != "node worker.js --queue {port:redis}" {
		t.Errorf("worker service = %+v", s)
	}
	if len(doc.Routes) != 2 {
		t.Fatalf("routes = %+v", doc.Routes)
	}
	api := doc.Routes[0]
	if api.Host != "api" || !reflect.DeepEqual(api.Paths, []string{"/api", "/v2"}) || !api.HTTPS || !api.ForceHTTPS {
		t.Errorf("api route = %+v", api)
	}
	if !reflect.DeepEqual(api.Backends, []string{"127.0.0.1:{port}", "10.0.0.5:9000"}) {
		t.Errorf("api backends = %+v", api.Backends)
	}
	db := doc.Routes[1]
	if !db.TCP || db.Listen != 55432 || !reflect.DeepEqual(db.Backends, []string{"127.0.0.1:{port:redis}"}) {
		t.Errorf("db route = %+v", db)
	}
}

func TestParseDotDnserListStyleServices(t *testing.T) {
	src := `
services:
  - name: cache
    type: valkey
    host: cache.internal
    port: 6379
    dns: false
  - name: queue
    command: nats-server -p {port}
`
	doc, err := ParseDotDnser(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Services) != 2 {
		t.Fatalf("services = %+v", doc.Services)
	}
	cache := doc.Services[0]
	if cache.Name != "cache" || cache.Host != "cache.internal" || cache.Port != 6379 || cache.DNS == nil || *cache.DNS {
		t.Errorf("cache = %+v", cache)
	}
	queue := doc.Services[1]
	if queue.Command == "" || queue.DNS == nil || !*queue.DNS {
		t.Errorf("queue = %+v", queue)
	}
}

func TestParseDotDnserLegacyCommandOnly(t *testing.T) {
	override, ok := ReadLinkOverrideFromSource("command:\n  npm run dev\n")
	if !ok || override.Command != "npm run dev" {
		t.Fatalf("legacy block style = %+v ok=%v", override, ok)
	}
	inline, ok := ReadLinkOverrideFromSource(`command: "pnpm dev --port {port}" # comment
other: value`)
	if !ok || inline.Command != "pnpm dev --port {port}" {
		t.Fatalf("inline with comment = %+v", inline)
	}
}

func TestParseDotDnserErrors(t *testing.T) {
	if _, err := ParseDotDnser("\tcommand: x"); err == nil {
		t.Error("tab indentation should error")
	}
	doc, err := ParseDotDnser("# only a comment")
	if err != nil || doc == nil || doc.Command != "" {
		t.Errorf("comment-only file should parse empty, got %v %v", doc, err)
	}
}
