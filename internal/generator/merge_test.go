package generator

import (
	"reflect"
	"testing"
)

func TestDeepMergeRecursiveMaps(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"x": 1, "y": 2}, "keep": true}
	src := map[string]any{"a": map[string]any{"y": 20, "z": 30}}
	got, err := DeepMerge(dst, src)
	if err != nil {
		t.Fatal(err)
	}
	a := got["a"].(map[string]any)
	if a["x"] != 1 || a["y"] != 20 || a["z"] != 30 {
		t.Fatalf("recursive merge wrong: %#v", a)
	}
	if got["keep"] != true {
		t.Fatalf("dst keys lost")
	}
}

func TestDeepMergeArraysReplace(t *testing.T) {
	dst := map[string]any{"list": []any{1, 2, 3}, "env": []string{"A=1"}}
	src := map[string]any{"list": []any{9}}
	for _, tc := range []struct{ k string }{{"list"}, {"env"}} {
		_ = tc
	}
	got, _ := DeepMerge(dst, src)
	if !reflect.DeepEqual(got["list"], []any{9}) {
		t.Fatalf("arrays must replace: %#v", got["list"])
	}
}

func TestDeepMergeScalarsReplaceAndTypesWin(t *testing.T) {
	dst := map[string]any{"s": "old", "n": 1}
	src := map[string]any{"s": "new", "n": map[string]any{"deep": true}}
	got, _ := DeepMerge(dst, src)
	if got["s"] != "new" {
		t.Fatalf("scalar must replace")
	}
	if _, ok := got["n"].(map[string]any); !ok {
		t.Fatalf("map must replace scalar: %#v", got["n"])
	}
}

func TestDeepMergeDoesNotMutateInputs(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"x": 1}}
	src := map[string]any{"a": map[string]any{"x": 2}}
	_, _ = DeepMerge(dst, src)
	if dst["a"].(map[string]any)["x"] != 1 {
		t.Fatalf("dst mutated")
	}
	if src["a"].(map[string]any)["x"] != 2 {
		t.Fatalf("src mutated")
	}
}

func TestRootSuffix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"auth.mycompany.internal", "mycompany.internal"},
		{"*.preview.auth.mycompany.internal", "auth.mycompany.internal"},
		{"internal", "internal"},
		{"app.test", "test"},
		{"mycompany.internal", "internal"},
		{"MyCompany.Internal", "internal"},
	}
	for _, tt := range tests {
		if got := RootSuffix(tt.in); got != tt.want {
			t.Fatalf("RootSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
