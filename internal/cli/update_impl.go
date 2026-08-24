package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releaseBaseURL = "https://github.com/SDK-E/dnser/releases/latest/download"

type ReleaseFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

type httpFetcher struct {
	client *http.Client
}

func NewHTTPFetcher() *httpFetcher {
	return &httpFetcher{client: &http.Client{Timeout: 60 * time.Second}}
}

func (h *httpFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

type UpdatePlan struct {
	AssetName    string
	ChecksumsURL string
	AssetURL     string
	Self         string
}

func BuildUpdatePlan(self, baseURL string) UpdatePlan {
	name := fmt.Sprintf("dnser_%s_%s", runtime.GOOS, runtime.GOARCH)
	base := baseURL
	if base == "" {
		base = releaseBaseURL
	}
	return UpdatePlan{
		AssetName:    name,
		ChecksumsURL: base + "/checksums.txt",
		AssetURL:     base + "/" + name,
		Self:         self,
	}
}

func VerifyChecksum(data []byte, checksums []byte, assetName string) error {
	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s; refusing to install (R10)", assetName)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s; refusing to install", assetName, got, want)
	}
	return nil
}

func ReplaceBinary(self string, newData []byte) error {
	dir := filepath.Dir(self)
	info, statErr := os.Stat(self)
	mode := os.FileMode(0o755)
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".dnser-update-*")
	if err != nil {
		return fmt.Errorf("temp beside binary: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(newData); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, self); err != nil {
		return fmt.Errorf("atomic replace of %s: %w", self, err)
	}
	return nil
}

func RunUpdate(ctx context.Context, self, baseURL string, apply bool, fetcher ReleaseFetcher) (map[string]any, error) {
	selfPath, err := os.Executable()
	if err == nil {
		selfPath = self
	}
	plan := BuildUpdatePlan(selfPath, baseURL)
	checksums, cerr := fetcher.Fetch(ctx, plan.ChecksumsURL)
	if cerr != nil {
		return nil, fmt.Errorf("--check: %w", cerr)
	}
	asset, aerr := fetcher.Fetch(ctx, plan.AssetURL)
	if aerr != nil {
		return nil, fmt.Errorf("download: %w", aerr)
	}
	if verr := VerifyChecksum(asset, checksums, plan.AssetName); verr != nil {
		return nil, verr
	}
	result := map[string]any{
		"asset":      plan.AssetName,
		"bytes":      len(asset),
		"checksum":   "ok",
		"applied":    false,
		"classified": classifyInstall(selfPath)[0],
	}
	source := classifyInstall(selfPath)[0]
	if source != "manual" && source != "script" {
		return result, fmt.Errorf("refusing to overwrite %s-managed install; use: brew upgrade sdk-e/tap/dnser", source)
	}
	if !apply {
		result["note"] = "dry run passed; re-run with --yes to install"
		return result, nil
	}
	if rerr := ReplaceBinary(selfPath, asset); rerr != nil {
		return nil, rerr
	}
	result["applied"] = true
	result["path"] = selfPath
	return result, nil
}

func classifyInstall(self string) []string {
	switch {
	case hasPrefixAny(self, "/opt/homebrew/", "/usr/local/Homebrew/", "/home/linuxbrew/"):
		return []string{"brew"}
	case hasPrefixAny(self, "/usr/local/bin/", "/usr/bin/"):
		return []string{"manual"}
	default:
		return []string{"script"}
	}
}
