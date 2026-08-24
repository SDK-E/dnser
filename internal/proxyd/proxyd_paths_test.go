package proxyd

import (
	"testing"
)

func TestRouterPathPrefixLongestWinsWithFallback(t *testing.T) {
	r := NewRouter()
	r.Replace([]Route{
		{Host: "app.test", Paths: []string{"/api"}, Backends: []string{"api:1"}},
		{Host: "app.test", Paths: []string{"/api/v2"}, Backends: []string{"apiv2:1"}},
		{Host: "app.test", Backends: []string{"root:1"}},
	})

	cases := []struct {
		path    string
		backend string
	}{
		{"/", "root:1"},
		{"/index.html", "root:1"},
		{"/api", "api:1"},
		{"/api/keys", "api:1"},
		{"/api/v2/users", "apiv2:1"},
		{"/apiv2", "root:1"},
		{"/apix", "root:1"},
	}
	for _, c := range cases {
		got, ok := r.Lookup("app.test", c.path)
		if !ok || len(got.Backends) != 1 || got.Backends[0] != c.backend {
			t.Errorf("Lookup(app.test, %q) = %+v ok=%v, want backend %q", c.path, got, ok, c.backend)
		}
	}
}

func TestRouterWildcardHostWithPathFallback(t *testing.T) {
	r := NewRouter()
	r.Replace([]Route{
		{Host: "*.team.test", Paths: []string{"/v1"}, Backends: []string{"edge:1"}},
		{Host: "*.team.test", Backends: []string{"base:1"}},
	})
	if got, _ := r.Lookup("acme.team.test", "/v1/items"); got.Backends[0] != "edge:1" {
		t.Errorf("prefix under wildcard = %v", got)
	}
	if got, _ := r.Lookup("acme.team.test", "/other"); got.Backends[0] != "base:1" {
		t.Errorf("fallback under wildcard = %v", got)
	}
}

func TestRouterPathOnlyRouteDoesNotShadowOthersWhenNoMatch(t *testing.T) {
	r := NewRouter()
	r.Replace([]Route{
		{Host: "x.test", Paths: []string{"/only"}, Backends: []string{"p:1"}},
		{Host: "y.test", Backends: []string{"y:1"}},
	})
	if _, ok := r.Lookup("x.test", "/nomatch"); ok {
		t.Error("host with only path routes must not match unrelated paths")
	}
	if got, ok := r.Lookup("x.test", "/only"); !ok || got.Backends[0] != "p:1" {
		t.Errorf("/only = %v ok=%v", got, ok)
	}
}

func TestRouterMultipleBackendsRoundRobinPerHost(t *testing.T) {
	r := NewRouter()
	r.Replace([]Route{{Host: "pool.test", Backends: []string{"a:1", "b:1"}}})
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		rt, ok := r.Lookup("pool.test", "/")
		if !ok {
			t.Fatal("lookup failed")
		}
		b, _ := r.Pick("pool.test", rt.Backends)
		seen[b] = true
	}
	if len(seen) != 2 {
		t.Errorf("round-robin should rotate both backends, saw %v", seen)
	}
}
