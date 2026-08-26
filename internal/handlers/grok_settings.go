package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"grok-build-webui/internal/grok"
	"grok-build-webui/internal/grokconfig"
)

var grokConfigMu sync.Mutex

func (h *SettingsHandler) grokConfigPath() string {
	return filepath.Join(grok.Home(h.cfg.GrokHome), "config.toml")
}

func (h *SettingsHandler) GetGrok(w http.ResponseWriter, r *http.Request) {
	grokConfigMu.Lock()
	defer grokConfigMu.Unlock()
	path := h.grokConfigPath()
	root, raw, mtime, exists, err := grokconfig.Load(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, grokconfig.Snapshot(path, root, raw, mtime, exists))
}

func (h *SettingsHandler) UpdateGrok(w http.ResponseWriter, r *http.Request) {
	var patch grokconfig.Patch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	grokConfigMu.Lock()
	defer grokConfigMu.Unlock()
	path := h.grokConfigPath()
	root, _, mtime, exists, err := grokconfig.Load(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if patch.IfMatchMTime != "" && exists {
		got := mtime.UTC().Format(time.RFC3339Nano)
		if got != patch.IfMatchMTime && !patch.Force {
			writeError(w, http.StatusConflict, "config.toml changed on disk; reload and try again")
			return
		}
	}
	if patch.Raw != nil {
		if err := grokconfig.SaveRaw(path, *patch.Raw); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if !patch.Empty() {
		if err := grokconfig.Apply(root, patch); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := grokconfig.Save(path, root); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	root, raw, mtime, exists, err := grokconfig.Load(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = os.Chmod(path, 0o600)
	writeJSON(w, http.StatusOK, grokconfig.Snapshot(path, root, raw, mtime, exists))
}
