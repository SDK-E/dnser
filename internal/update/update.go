package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultReleasesAPI = "https://api.github.com/repos/SDK-E/dnser/releases/latest"

var releasesAPI = defaultReleasesAPI

func releasesAPISetter(u string) { releasesAPI = u }

type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

func Latest(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("update: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: fetch latest release: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: github api returned %s", res.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("update: decode release: %w", err)
	}
	return &Release{
		Version: strings.TrimPrefix(payload.TagName, "v"),
		URL:     payload.HTMLURL,
		Notes:   payload.Body,
	}, nil
}

func Check(ctx context.Context, current string) (*Release, error) {
	rel, err := Latest(ctx)
	if err != nil {
		return nil, err
	}
	if Compare(rel.Version, current) <= 0 {
		return nil, nil
	}
	return rel, nil
}

func Compare(a, b string) int {
	pa := parts(a)
	pb := parts(b)
	for i := range 3 {
		d := pa[i] - pb[i]
		if d != 0 {
			return d
		}
	}
	return 0
}

func parts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, seg := range strings.SplitN(v, ".", 3) {
		n := 0
		for _, r := range seg {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out
}
