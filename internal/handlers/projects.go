package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
	"grok-build-webui/internal/grok"
	"grok-build-webui/internal/paths"
	"grok-build-webui/internal/session"
)

type ProjectHandler struct {
	db      *db.DB
	manager *session.Manager
	cfg     *config.Config
}

func NewProjectHandler(d *db.DB) *ProjectHandler {
	return &ProjectHandler{db: d}
}

func NewProjectHandlerWithManager(d *db.DB, m *session.Manager, cfg *config.Config) *ProjectHandler {
	return &ProjectHandler{db: d, manager: m, cfg: cfg}
}

type projectOut struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	CreatedAt time.Time    `json:"created_at"`
	Layout    string       `json:"layout,omitempty"`
	ActiveTab string       `json:"active_tab,omitempty"`
	Git       grok.GitInfo `json:"git"`
}

func (h *ProjectHandler) allowRoot() string {
	if h.cfg != nil {
		return h.cfg.AllowRoot
	}
	return ""
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, name, path, created_at, layout, active_tab FROM projects ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	var out []projectOut
	for rows.Next() {
		var p projectOut
		var created, layout, active string
		if err := rows.Scan(&p.ID, &p.Name, &p.Path, &created, &layout, &active); err != nil {
			continue
		}
		p.CreatedAt = parseTime(created)
		p.Layout = layout
		p.ActiveTab = active
		p.Git = grok.GitStatus(p.Path)
		out = append(out, p)
	}
	if out == nil {
		out = []projectOut{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Path = strings.TrimSpace(req.Path)
	if req.Name == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "name and path required")
		return
	}
	abs, err := paths.NormalizeDir(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "path does not exist")
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}
	if !paths.Allowed(abs, h.allowRoot()) {
		writeError(w, http.StatusBadRequest, "path not allowed")
		return
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = h.db.Exec(`INSERT INTO projects(id,name,path,created_at) VALUES(?,?,?,?)`, id, req.Name, abs, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "path already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, projectOut{ID: id, Name: req.Name, Path: abs, CreatedAt: now, Git: grok.GitStatus(abs)})
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	p, err := h.load(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProjectHandler) load(id string) (projectOut, error) {
	var p projectOut
	var created, layout, active string
	err := h.db.QueryRow(`SELECT id, name, path, created_at, layout, active_tab FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Path, &created, &layout, &active)
	if err != nil {
		return p, err
	}
	p.CreatedAt = parseTime(created)
	p.Layout = layout
	p.ActiveTab = active
	p.Git = grok.GitStatus(p.Path)
	return p, nil
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	var req struct {
		Name      *string `json:"name"`
		Path      *string `json:"path"`
		Layout    *string `json:"layout"`
		ActiveTab *string `json:"active_tab"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	existing, err := h.load(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Path != nil && strings.TrimSpace(*req.Path) != "" {
		abs, err := paths.NormalizeDir(*req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		if !paths.Allowed(abs, h.allowRoot()) {
			writeError(w, http.StatusBadRequest, "path not allowed")
			return
		}
		existing.Path = abs
	}
	if req.Layout != nil {
		existing.Layout = *req.Layout
	}
	if req.ActiveTab != nil {
		existing.ActiveTab = *req.ActiveTab
	}
	_, err = h.db.Exec(`UPDATE projects SET name=?, path=?, layout=?, active_tab=? WHERE id=?`,
		existing.Name, existing.Path, existing.Layout, existing.ActiveTab, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	existing.Git = grok.GitStatus(existing.Path)
	writeJSON(w, http.StatusOK, existing)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	if h.manager != nil {
		for _, s := range h.manager.ListByProject(id) {
			_ = h.manager.Close(s.ID)
		}
	}
	res, err := h.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ProjectHandler) Conversations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	p, err := h.load(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	home := ""
	if h.cfg != nil {
		home = h.cfg.GrokHome
	}
	list := grok.ListConversations(home, p.Path)
	if list == nil {
		list = []grok.Conversation{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *ProjectHandler) Browse(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	if dir == "" {
		if h.cfg != nil && h.cfg.AllowRoot != "" {
			dir = h.cfg.AllowRoot
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = home
		} else {
			dir = "/"
		}
	}
	abs, err := paths.NormalizeDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if !paths.Allowed(abs, h.allowRoot()) {
		writeError(w, http.StatusBadRequest, "path not allowed")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not a directory")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read directory")
		return
	}
	var dirs []dirItem
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(abs, e.Name())
		if !paths.Allowed(child, h.allowRoot()) {
			continue
		}
		dirs = append(dirs, dirItem{Name: e.Name(), Path: child})
	}
	parent := filepath.Dir(abs)
	if parent == abs || !paths.Allowed(parent, h.allowRoot()) {
		parent = ""
	}
	resp := map[string]any{
		"path":   abs,
		"parent": parent,
		"dirs":   dirs,
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		resp["matches"] = searchDirs(abs, h.allowRoot(), strings.ToLower(q))
	}
	writeJSON(w, http.StatusOK, resp)
}

type dirItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// skipDirs are dependency/build output folders excluded from recursive
// directory search; they can contain tens of thousands of subdirectories.
var skipDirs = map[string]bool{
	"node_modules": true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"target":       true,
	"dist":         true,
	"build":        true,
	"vendor":       true,
}

const (
	searchMaxDepth  = 4
	searchMaxVisits = 4000
	searchMaxResult = 100
)

// searchDirs walks the tree under root looking for directories whose name
// contains query (case-insensitive), skipping hidden and dependency
// directories. The walk is capped so a huge tree can't stall the request.
func searchDirs(root, allowRoot, query string) []dirItem {
	query = strings.ToLower(strings.TrimSpace(query))
	type work struct {
		path  string
		depth int
	}
	var (
		matches []dirItem
		stack   = []work{{root, 0}}
		visits  int
	)
	for len(stack) > 0 && len(matches) < searchMaxResult && visits < searchMaxVisits {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := os.ReadDir(cur.path)
		if err != nil {
			continue
		}
		visits++
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || skipDirs[name] {
				continue
			}
			child := filepath.Join(cur.path, name)
			if !paths.Allowed(child, allowRoot) {
				continue
			}
			if strings.Contains(strings.ToLower(name), query) {
				matches = append(matches, dirItem{name, child})
				continue
			}
			if cur.depth+1 <= searchMaxDepth {
				stack = append(stack, work{child, cur.depth + 1})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	return time.Time{}
}
