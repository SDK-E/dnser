package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.3", "1.2.2", 1},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "10.0.0", -8},
		{"v1.2.3", "1.2.4", -1},
		{"1.2.3-rc1", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
	}
	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheckWithMockServer(t *testing.T) {
	old := releasesAPI
	t.Cleanup(func() { setAPI(old) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v9.8.7","html_url":"https://x/release","body":"notes"}`)
	}))
	defer srv.Close()
	setAPI(srv.URL)

	rel, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "9.8.7" || rel.URL != "https://x/release" {
		t.Fatalf("unexpected release: %+v", rel)
	}

	upToDate, err := Check(context.Background(), "9.8.7")
	if err != nil || upToDate != nil {
		t.Fatalf("expected nil release when current, got %v err %v", upToDate, err)
	}

	srv.Close()
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer errServer.Close()
	setAPI(errServer.URL)
	if _, err := Check(context.Background(), "1.0.0"); !errors.Is(err, nil) && err == nil {
		t.Fatal("expected error from failing server")
	} else if err == nil {
		t.Fatal("expected error from failing server")
	}
}

func setAPI(u string) { releasesAPISetter(u) }
