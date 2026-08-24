package api

import (
	"embed"
	"net/http"

	"github.com/SDK-E/dnser/internal/config"
)

//go:embed schemas/*.json
var schemaFS embed.FS

type settingsView struct {
	TLD             string       `json:"tld"`
	Bind            string       `json:"bind"`
	Upstreams       []string     `json:"upstreams"`
	Autostart       bool         `json:"autostart"`
	Ports           config.Ports `json:"ports"`
	ForceHTTPS      bool         `json:"force_https"`
	PathRefreshMins int          `json:"path_refresh_minutes"`
}

func settingsToView(s config.Settings) settingsView {
	return settingsView{
		TLD:             s.TLD,
		Bind:            s.Bind,
		Upstreams:       s.Upstreams,
		Autostart:       s.Autostart,
		Ports:           s.Ports,
		ForceHTTPS:      s.ForceHTTPS,
		PathRefreshMins: s.PathRefresh(),
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, settingsToView(s.rt.Store().Settings()))
}

type updateSettingsReq struct {
	TLD             *string       `json:"tld,omitempty"`
	Bind            *string       `json:"bind,omitempty"`
	Upstreams       *[]string     `json:"upstreams,omitempty"`
	Autostart       *bool         `json:"autostart,omitempty"`
	Ports           *patchedPorts `json:"ports,omitempty"`
	ForceHTTPS      *bool         `json:"force_https,omitempty"`
	PathRefreshMins *int          `json:"path_refresh_minutes,omitempty"`
}

type patchedPorts struct {
	DNS   *int `json:"dns,omitempty"`
	HTTP  *int `json:"http,omitempty"`
	HTTPS *int `json:"https,omitempty"`
	UI    *int `json:"ui,omitempty"`
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsReq
	if !decodeJSON(w, r, &req) {
		return
	}
	store := s.rt.Store()
	err := store.Update(func(c *config.Config) {
		if req.TLD != nil {
			c.Settings.TLD = *req.TLD
		}
		if req.Bind != nil {
			c.Settings.Bind = *req.Bind
		}
		if req.Upstreams != nil {
			c.Settings.Upstreams = *req.Upstreams
		}
		if req.Autostart != nil {
			c.Settings.Autostart = *req.Autostart
		}
		if req.Ports != nil {
			if req.Ports.DNS != nil {
				c.Settings.Ports.DNS = *req.Ports.DNS
			}
			if req.Ports.HTTP != nil {
				c.Settings.Ports.HTTP = *req.Ports.HTTP
			}
			if req.Ports.HTTPS != nil {
				c.Settings.Ports.HTTPS = *req.Ports.HTTPS
			}
			if req.Ports.UI != nil {
				c.Settings.Ports.UI = *req.Ports.UI
			}
		}
		if req.ForceHTTPS != nil {
			c.Settings.ForceHTTPS = *req.ForceHTTPS
		}
		if req.PathRefreshMins != nil {
			c.Settings.PathRefreshMins = *req.PathRefreshMins
		}
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settingsToView(store.Settings()))
}

func serveSchema(w http.ResponseWriter, name string) {
	data, err := SchemaFile(name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "schema missing")
		return
	}
	w.Header().Set("Content-Type", "application/schema+json; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleSchemaConfig(w http.ResponseWriter, r *http.Request) {
	serveSchema(w, ConfigSchemaFile)
}

func (s *Server) handleSchemaProject(w http.ResponseWriter, r *http.Request) {
	serveSchema(w, ProjectSchemaFile)
}

const (
	ConfigSchemaFile  = "dnser.config.schema.json"
	ProjectSchemaFile = "dnser.project.schema.json"
)

func SchemaFile(name string) ([]byte, error) {
	return schemaFS.ReadFile("schemas/" + name)
}
