package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"time"

	"grok-build-webui/internal/db"
	"grok-build-webui/internal/session"
)

type SessionHandler struct {
	db      *db.DB
	manager *session.Manager
}

func NewSessionHandler(d *db.DB, m *session.Manager) *SessionHandler {
	return &SessionHandler{db: d, manager: m}
}

type sessionOut struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	Cols           int       `json:"cols"`
	Rows           int       `json:"rows"`
	CreatedAt      time.Time `json:"created_at"`
	LastActive     time.Time `json:"last_active"`
	Mode           string    `json:"mode"`
	ResumeID       string    `json:"resume_id"`
	GrokSessionID  string    `json:"grok_session_id"`
	Model          string    `json:"model"`
	PermissionMode string    `json:"permission_mode"`
	Sandbox        string    `json:"sandbox"`
	Yolo           bool      `json:"yolo"`
}

func sessionFromLive(s *session.Session) sessionOut {
	snap := s.Snapshot()
	out := sessionOut{
		ID:        s.ID,
		ProjectID: s.ProjectID,
	}
	if v, ok := snap["title"].(string); ok {
		out.Title = v
	}
	if v, ok := snap["status"].(string); ok {
		out.Status = v
	}
	if s.IsRunning() {
		out.Status = "running"
	}
	if v, ok := snap["cols"].(int); ok {
		out.Cols = v
	}
	if v, ok := snap["rows"].(int); ok {
		out.Rows = v
	}
	if v, ok := snap["created_at"].(time.Time); ok {
		out.CreatedAt = v
	}
	if v, ok := snap["last_active"].(time.Time); ok {
		out.LastActive = v
	}
	if v, ok := snap["mode"].(string); ok {
		out.Mode = v
	}
	if v, ok := snap["resume_id"].(string); ok {
		out.ResumeID = v
	}
	if v, ok := snap["grok_session_id"].(string); ok {
		out.GrokSessionID = v
	}
	if v, ok := snap["model"].(string); ok {
		out.Model = v
	}
	if v, ok := snap["permission_mode"].(string); ok {
		out.PermissionMode = v
	}
	if v, ok := snap["sandbox"].(string); ok {
		out.Sandbox = v
	}
	if v, ok := snap["yolo"].(bool); ok {
		out.Yolo = v
	}
	return out
}

func (h *SessionHandler) ListByProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				projectID = parts[i+1]
				break
			}
		}
	}

	var res []sessionOut
	existing := map[string]bool{}
	for _, s := range h.manager.ListByProject(projectID) {
		res = append(res, sessionFromLive(s))
		existing[s.ID] = true
	}

	rows, err := h.db.Query(`SELECT id, project_id, title, status, cols, rows, created_at, last_active, mode, resume_id, grok_session_id, model, permission_mode, sandbox, yolo FROM sessions WHERE project_id=? ORDER BY created_at ASC`, projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			row, ok := scanSessionRow(rows)
			if !ok || existing[row.ID] {
				continue
			}
			res = append(res, h.normalizeRow(row))
		}
	}
	if res == nil {
		res = []sessionOut{}
	}
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})
	writeJSON(w, http.StatusOK, res)
}

// normalizeRow corrects the status of DB rows that no longer have a live
// PTY behind them. After a server restart rows can still say "running";
// if the manager doesn't hold the process, it has exited (and the tab
// becomes restorable via /restore).
func (h *SessionHandler) normalizeRow(row sessionOut) sessionOut {
	if row.Status == "running" {
		if h.manager == nil {
			row.Status = "exited"
		} else if _, live := h.manager.Get(row.ID); !live {
			row.Status = "exited"
		}
	}
	return row
}

func scanSessionRow(rows *sql.Rows) (sessionOut, bool) {	var row sessionOut
	var created, last string
	var yolo int
	if err := rows.Scan(&row.ID, &row.ProjectID, &row.Title, &row.Status, &row.Cols, &row.Rows, &created, &last, &row.Mode, &row.ResumeID, &row.GrokSessionID, &row.Model, &row.PermissionMode, &row.Sandbox, &yolo); err != nil {
		return row, false
	}
	row.CreatedAt = parseTime(created)
	row.LastActive = parseTime(last)
	row.Yolo = yolo != 0
	return row, true
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				projectID = parts[i+1]
				break
			}
		}
	}
	var req struct {
		Title          string `json:"title"`
		Cols           int    `json:"cols"`
		Rows           int    `json:"rows"`
		Mode           string `json:"mode"`
		ResumeID       string `json:"resume_id"`
		Model          string `json:"model"`
		PermissionMode string `json:"permission_mode"`
		Sandbox        string `json:"sandbox"`
		Yolo           bool   `json:"yolo"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var projectPath string
	err := h.db.QueryRow(`SELECT path FROM projects WHERE id=?`, projectID).Scan(&projectPath)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if _, err := os.Stat(projectPath); err != nil {
		writeError(w, http.StatusBadRequest, "project path missing")
		return
	}
	sess, err := h.manager.Create(projectID, projectPath, session.CreateOpts{
		Title:          req.Title,
		Cols:           req.Cols,
		Rows:           req.Rows,
		Mode:           req.Mode,
		ResumeID:       req.ResumeID,
		Model:          req.Model,
		PermissionMode: req.PermissionMode,
		Sandbox:        req.Sandbox,
		Yolo:           req.Yolo,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sessionFromLive(sess))
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	if sess, ok := h.manager.Get(id); ok {
		writeJSON(w, http.StatusOK, sessionFromLive(sess))
		return
	}
	row, err := h.loadRow(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, h.normalizeRow(row))
}

func (h *SessionHandler) loadRow(id string) (sessionOut, error) {
	var row sessionOut
	var created, last string
	var yolo int
	err := h.db.QueryRow(`SELECT id, project_id, title, status, cols, rows, created_at, last_active, mode, resume_id, grok_session_id, model, permission_mode, sandbox, yolo FROM sessions WHERE id=?`, id).
		Scan(&row.ID, &row.ProjectID, &row.Title, &row.Status, &row.Cols, &row.Rows, &created, &last, &row.Mode, &row.ResumeID, &row.GrokSessionID, &row.Model, &row.PermissionMode, &row.Sandbox, &yolo)
	if err != nil {
		return row, err
	}
	row.CreatedAt = parseTime(created)
	row.LastActive = parseTime(last)
	row.Yolo = yolo != 0
	return row, nil
}

func (h *SessionHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	var req struct {
		Title *string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title != nil {
		if err := h.manager.Rename(id, *req.Title); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if sess, ok := h.manager.Get(id); ok {
		writeJSON(w, http.StatusOK, sessionFromLive(sess))
		return
	}
	row, err := h.loadRow(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, h.normalizeRow(row))
}

func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	_ = h.manager.Close(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SessionHandler) Resize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "sessions" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	var req struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		writeError(w, http.StatusBadRequest, "invalid size")
		return
	}
	if err := h.manager.Resize(id, req.Cols, req.Rows); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SessionHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "sessions" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	if sess, ok := h.manager.Get(id); ok && sess.IsRunning() {
		writeJSON(w, http.StatusOK, sessionFromLive(sess))
		return
	}
	h.manager.Forget(id)
	row, err := h.loadRow(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var projectPath string
	if err := h.db.QueryRow(`SELECT path FROM projects WHERE id=?`, row.ProjectID).Scan(&projectPath); err != nil {
		writeError(w, http.StatusBadRequest, "project missing")
		return
	}
	_, _ = h.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	mode := row.Mode
	resumeID := row.ResumeID
	if row.GrokSessionID != "" {
		mode = "resume"
		resumeID = row.GrokSessionID
	} else if mode == "" || mode == "new" {
		mode = "continue"
	}
	sess, err := h.manager.Create(row.ProjectID, projectPath, session.CreateOpts{
		ID:             id,
		Title:          row.Title,
		Cols:           row.Cols,
		Rows:           row.Rows,
		Mode:           mode,
		ResumeID:       resumeID,
		Model:          row.Model,
		PermissionMode: row.PermissionMode,
		Sandbox:        row.Sandbox,
		Yolo:           row.Yolo,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessionFromLive(sess))
}
