package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SDK-E/dnser/internal/detect"
	"github.com/SDK-E/dnser/internal/runner"
)

type detectRequest struct {
	Path string `json:"path"`
}

type detectResponse struct {
	Path         string   `json:"path"`
	Framework    string   `json:"framework,omitempty"`
	Recipe       []string `json:"recipe,omitempty"`
	PortEnv      bool     `json:"port_env"`
	DepsMissing  string   `json:"deps_missing,omitempty"`
	SuggestedDNS string   `json:"suggested_domain"`
}

func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	var reqBody detectRequest
	if !decodeJSON(w, r, &reqBody) {
		return
	}
	dir := strings.TrimSpace(reqBody.Path)
	if dir == "" {
		writeErr(w, http.StatusBadRequest, "path is required")
		return
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
		}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "not a directory: "+dir)
		return
	}
	resp := detectResponse{
		Path:         dir,
		SuggestedDNS: strings.ToLower(filepath.Base(dir)),
	}
	result, recipe, err := detect.DetectStack(dir)
	if err == nil {
		resp.Framework = result.Framework
		resp.Recipe = recipe.Command
		resp.PortEnv = recipe.PortEnv
		if !detect.DepsInstalled(dir, result.Framework) {
			resp.DepsMissing = detect.InstallHint(result.Framework)
			if resp.DepsMissing == "" {
				resp.DepsMissing = "install project dependencies first"
			}
		}
	}
	if _, ok := runner.ReadLinkOverride(dir); ok {
		if override, _ := runner.ReadLinkOverride(dir); override.Command != "" {
			resp.Recipe = strings.Fields(override.Command)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
