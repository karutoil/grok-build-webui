package handlers

import (
	"net/http"
	"time"

	"grok-build-webui/internal/version"
)

// Version reports the running binary's version relative to upstream releases.
// GET /api/settings/version           — cached upstream check
// GET /api/settings/version?refresh=1 — bypass the cache
func (h *SettingsHandler) Version(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		version.Refresh()
	}
	cur := h.cfg.Version
	latest := version.Latest()
	checkedAt := ""
	if latest != "" {
		if t := version.LastChecked(); !t.IsZero() {
			checkedAt = t.UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":      cur,
		"channel":      version.Channel(cur),
		"latest":       latest,
		"status":       version.Status(cur, latest),
		"releases_url": version.ReleasesURL(),
		"checked_at":   checkedAt,
	})
}
