package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"grok-build-webui/internal/auth"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
	"grok-build-webui/internal/session"
)

type SettingsHandler struct {
	db      *db.DB
	cfg     *config.Config
	auth    *auth.Service
	manager *session.Manager
}

func NewSettingsHandler(d *db.DB, c *config.Config, a *auth.Service, m *session.Manager) *SettingsHandler {
	return &SettingsHandler{db: d, cfg: c, auth: a, manager: m}
}

func (h *SettingsHandler) envLocked() bool {
	return os.Getenv("GROK_WEBUI_PUBLIC_URL") != ""
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	publicURL := h.cfg.PublicURL
	if !h.envLocked() {
		if v, ok := h.db.GetSetting("public_url"); ok {
			publicURL = v
		}
	}
	prefs := map[string]any{}
	if v, ok := h.db.GetSetting("ui_prefs"); ok && v != "" {
		_ = json.Unmarshal([]byte(v), &prefs)
	}
	running := 0
	if h.manager != nil {
		running = h.manager.RunningCount()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"public_url":   publicURL,
		"rpid":         h.cfg.RPID(),
		"grok_bin":     h.cfg.GrokBin,
		"grok_home":    h.cfg.GrokHome,
		"allow_root":   h.cfg.AllowRoot,
		"max_sessions": h.cfg.MaxSessions,
		"running":      running,
		"locked":       h.envLocked(),
		"prefs":        prefs,
		"started_at":   h.cfg.StartedAt.UTC().Format(time.RFC3339Nano),
	})
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicURL *string         `json:"public_url"`
		Prefs     json.RawMessage `json:"prefs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.PublicURL != nil {
		v := strings.TrimSuffix(strings.TrimSpace(*req.PublicURL), "/")
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || u.Scheme == "" || u.Host == "" {
				writeError(w, http.StatusBadRequest, "invalid public_url, must be https://...")
				return
			}
			if u.Scheme != "https" && u.Scheme != "http" {
				writeError(w, http.StatusBadRequest, "public_url must be https:// or http://")
				return
			}
		}
		_ = h.db.SetSetting("public_url", v)
		if !h.envLocked() {
			h.cfg.PublicURL = v
		}
		_ = h.auth.ReloadWebAuthn()
	}
	if len(req.Prefs) > 0 && string(req.Prefs) != "null" {
		_ = h.db.SetSetting("ui_prefs", string(req.Prefs))
	}
	h.Get(w, r)
}
