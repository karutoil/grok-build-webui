package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
	"grok-build-webui/internal/grok"
)

type Manager struct {
	cfg      *config.Config
	db       *db.DB
	mu       sync.RWMutex
	sessions map[string]*Session
}

type CreateOpts struct {
	ID             string
	Title          string
	Cols           int
	Rows           int
	Mode           string
	ResumeID       string
	Model          string
	PermissionMode string
	Sandbox        string
	Yolo           bool
}

type Session struct {
	ID             string
	ProjectID      string
	ProjectPath    string
	Title          string
	Status         string // running, exited
	Cols           int
	Rows           int
	CreatedAt      time.Time
	LastActive     time.Time
	Mode           string
	ResumeID       string
	GrokSessionID  string
	Model          string
	PermissionMode string
	Sandbox        string
	Yolo           bool
	Cmd            *exec.Cmd
	Pty            *os.File
	Buffer         *RingBuffer
	Modes          *ModeTracker
	Clients        map[string]chan []byte
	clientsMu      sync.Mutex
	exitCode       int
	mu             sync.Mutex
	done           chan struct{}
	closeOnce      sync.Once
}

func NewManager(cfg *config.Config, database *db.DB) *Manager {
	m := &Manager{
		cfg:      cfg,
		db:       database,
		sessions: make(map[string]*Session),
	}
	// PTYs do not survive process restart. Keep rows as a restore recipe
	// (mode/resume/layout) but mark them exited so the UI can respawn.
	_, _ = database.Exec(`UPDATE sessions SET status='exited' WHERE status='running'`)
	return m
}

func (m *Manager) Create(projectID, projectPath string, opts CreateOpts) (*Session, error) {
	if opts.Cols <= 0 {
		opts.Cols = 120
	}
	if opts.Rows <= 0 {
		opts.Rows = 30
	}
	mode := grok.NormalizeMode(opts.Mode)
	if opts.Title == "" {
		switch mode {
		case "continue":
			opts.Title = "continue"
		case "resume":
			opts.Title = "resume"
		default:
			opts.Title = "new"
		}
	}

	m.mu.RLock()
	n := 0
	for _, s := range m.sessions {
		if s.IsRunning() {
			n++
		}
	}
	m.mu.RUnlock()
	if n >= m.cfg.MaxSessions {
		return nil, fmt.Errorf("too many running sessions (max %d)", m.cfg.MaxSessions)
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC()
	launch := grok.Launch{
		Mode:           mode,
		ResumeID:       opts.ResumeID,
		Model:          opts.Model,
		PermissionMode: opts.PermissionMode,
		Sandbox:        opts.Sandbox,
		Yolo:           opts.Yolo,
	}
	args := grok.BuildArgs(launch)

	cmd := exec.Command(m.cfg.GrokBin, args...)
	cmd.Dir = projectPath
	cmd.Env = m.ptyEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(opts.Cols), Rows: uint16(opts.Rows)})

	grokID := opts.ResumeID
	if mode == "continue" {
		if c, ok := grok.Latest(m.cfg.GrokHome, projectPath); ok {
			grokID = c.ID
			if opts.Title == "continue" && c.Title != "" {
				opts.Title = c.Title
			}
		}
	} else if mode == "resume" && grokID != "" {
		if c, ok := grok.Find(m.cfg.GrokHome, projectPath, grokID); ok && opts.Title == "resume" {
			opts.Title = c.Title
		}
	}

	sess := &Session{
		ID:             id,
		ProjectID:      projectID,
		ProjectPath:    projectPath,
		Title:          opts.Title,
		Status:         "running",
		Cols:           opts.Cols,
		Rows:           opts.Rows,
		CreatedAt:      now,
		LastActive:     now,
		Mode:           mode,
		ResumeID:       opts.ResumeID,
		GrokSessionID:  grokID,
		Model:          opts.Model,
		PermissionMode: opts.PermissionMode,
		Sandbox:        opts.Sandbox,
		Yolo:           opts.Yolo,
		Cmd:            cmd,
		Pty:            ptmx,
		Buffer:         NewRingBuffer(256 * 1024),
		Modes:          NewModeTracker(),
		Clients:        make(map[string]chan []byte),
		done:           make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	_, err = m.db.Exec(
		`INSERT INTO sessions(id,project_id,title,status,cols,rows,created_at,last_active,mode,resume_id,grok_session_id,model,permission_mode,sandbox,yolo)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, projectID, opts.Title, "running", opts.Cols, opts.Rows, now, now,
		mode, opts.ResumeID, grokID, opts.Model, opts.PermissionMode, opts.Sandbox, boolToInt(opts.Yolo),
	)
	if err != nil {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
		return nil, err
	}

	go m.ioLoop(sess)
	go m.waitLoop(sess)
	if mode == "new" {
		go m.discoverGrokID(sess, now)
	}
	return sess, nil
}

func (m *Manager) ptyEnv() []string {
	home, _ := os.UserHomeDir()
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	base := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"PATH=" + path,
		"HOME=" + home,
		"USER=" + os.Getenv("USER"),
		"SHELL=" + firstNonEmpty(os.Getenv("SHELL"), "/bin/bash"),
	}
	if m.cfg.GrokHome != "" {
		base = append(base, "GROK_HOME="+m.cfg.GrokHome)
	} else if v := os.Getenv("GROK_HOME"); v != "" {
		base = append(base, "GROK_HOME="+v)
	}
	if m.cfg.CleanEnv {
		return base
	}
	// Pass a filtered host env: skip secrets-looking keys and the ones we set.
	skip := map[string]bool{
		"TERM": true, "COLORTERM": true, "LANG": true, "LC_ALL": true,
	}
	out := append([]string{}, base...)
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		uk := strings.ToUpper(k)
		if skip[k] {
			continue
		}
		if strings.Contains(uk, "SECRET") || strings.Contains(uk, "PASSWORD") || strings.Contains(uk, "TOKEN") && !strings.HasPrefix(uk, "GROK_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (m *Manager) discoverGrokID(s *Session, since time.Time) {
	ignore := map[string]bool{}
	for _, other := range m.ListAll() {
		if other.GrokSessionID != "" {
			ignore[other.GrokSessionID] = true
		}
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !s.IsRunning() {
			return
		}
		if c, ok := grok.NewestSince(m.cfg.GrokHome, s.ProjectPath, since, ignore); ok {
			s.mu.Lock()
			s.GrokSessionID = c.ID
			if s.Title == "new" || s.Title == "grok" {
				s.Title = c.Title
			}
			title := s.Title
			s.mu.Unlock()
			_, _ = m.db.Exec(`UPDATE sessions SET grok_session_id=?, title=?, last_active=? WHERE id=?`,
				c.ID, title, time.Now().UTC(), s.ID)
			return
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func (m *Manager) ioLoop(s *Session) {
	buf := make([]byte, 8192)
	for {
		if s.Pty == nil {
			return
		}
		n, err := s.Pty.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.emit(data)
		}
		if err != nil {
			if err != io.EOF {
				select {
				case <-s.done:
				default:
					time.Sleep(30 * time.Millisecond)
					select {
					case <-s.done:
					default:
						continue
					}
				}
			}
			break
		}
	}
}

func (m *Manager) waitLoop(s *Session) {
	err := s.Cmd.Wait()
	s.mu.Lock()
	s.Status = "exited"
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			s.exitCode = exitErr.ExitCode()
		} else {
			s.exitCode = 1
		}
	} else {
		s.exitCode = 0
	}
	s.mu.Unlock()
	_, _ = m.db.Exec(`UPDATE sessions SET status='exited', last_active=? WHERE id=?`, time.Now().UTC(), s.ID)
	s.broadcastExit()
	if s.Pty != nil {
		_ = s.Pty.Close()
	}
	s.closeOnce.Do(func() { close(s.done) })
}

func (s *Session) touch() {
	s.mu.Lock()
	s.LastActive = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

func (s *Session) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status == "running"
}

// emit records a PTY chunk into the scrollback and fans it out to live
// clients under a single critical section. Holding clientsMu across both
// steps makes attaching atomic (see AttachClient): a new client either has
// the chunk already inside its history snapshot, or receives it on its
// channel — never both, never neither.
func (s *Session) emit(data []byte) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	_, _ = s.Buffer.Write(data)
	s.Modes.Write(data)
	for _, ch := range s.Clients {
		payload := append([]byte(nil), data...)
		select {
		case ch <- payload:
		default:
			// Slow client: block briefly rather than dropping PTY output.
			select {
			case ch <- payload:
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
}

func (s *Session) broadcastExit() {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for _, ch := range s.Clients {
		select {
		case ch <- nil:
		default:
		}
	}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) ListByProject(projectID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Session
	for _, s := range m.sessions {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out
}

func (m *Manager) ListAll() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

func (m *Manager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, s := range m.sessions {
		if s.IsRunning() {
			n++
		}
	}
	return n
}

func (m *Manager) Resize(id string, cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("invalid size")
	}
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("not found")
	}
	s.mu.Lock()
	s.Cols = cols
	s.Rows = rows
	running := s.Status == "running"
	ptmx := s.Pty
	s.mu.Unlock()
	_, _ = m.db.Exec(`UPDATE sessions SET cols=?, rows=?, last_active=? WHERE id=?`, cols, rows, time.Now().UTC(), id)
	if running && ptmx != nil {
		return pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
	return nil
}

func (m *Manager) Write(id string, data []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("not found")
	}
	s.mu.Lock()
	running := s.Status == "running"
	ptmx := s.Pty
	s.mu.Unlock()
	if !running || ptmx == nil {
		return fmt.Errorf("session exited")
	}
	_, err := ptmx.Write(data)
	s.touch()
	return err
}

func (m *Manager) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title required")
	}
	s, ok := m.Get(id)
	if ok {
		s.mu.Lock()
		s.Title = title
		s.mu.Unlock()
	}
	res, err := m.db.Exec(`UPDATE sessions SET title=?, last_active=? WHERE id=?`, title, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 && !ok {
		return fmt.Errorf("not found")
	}
	return nil
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		_, _ = m.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
		return fmt.Errorf("not found")
	}
	m.stop(s)
	// User-initiated close removes the session entirely.
	_, _ = m.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return nil
}

func (m *Manager) stop(s *Session) {
	if s.Cmd != nil && s.Cmd.Process != nil {
		_ = s.Cmd.Process.Signal(os.Interrupt)
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			_ = s.Cmd.Process.Kill()
			select {
			case <-s.done:
			case <-time.After(2 * time.Second):
			}
		}
	} else {
		s.closeOnce.Do(func() { close(s.done) })
	}
	if s.Pty != nil {
		_ = s.Pty.Close()
	}
	s.broadcastExit()
	s.closeClients()
}

// StopAll kills every live PTY without deleting session rows, marking them
// "exited" so a browser can offer to restore the conversations (via
// continue/resume) after a server restart.
func (m *Manager) StopAll() {
	for _, s := range m.ListAll() {
		m.stop(s)
		_, _ = m.db.Exec(`UPDATE sessions SET status='exited' WHERE id=?`, s.ID)
	}
}

func (s *Session) closeClients() {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	s.Clients = make(map[string]chan []byte)
}

func (m *Manager) AttachClient(sessionID string) (chan []byte, func(), []byte, bool) {
	s, ok := m.Get(sessionID)
	if !ok {
		return nil, func() {}, nil, false
	}
	ch := make(chan []byte, 256)
	id := uuid.NewString()

	// Snapshot the history and register the live channel in one critical
	// section. emit() also runs under clientsMu, so every PTY chunk is
	// either fully contained in this snapshot or arrives on the channel
	// afterwards. Without this, a reconnect could duplicate bytes (chunk
	// both in history and queued live) or drop them (registered after the
	// broadcast but before the snapshot).
	s.clientsMu.Lock()
	var history []byte
	if prefix := s.Modes.Prefix(); len(prefix) > 0 {
		replay := s.Buffer.ReplayBytes()
		history = make([]byte, 0, len(prefix)+len(replay))
		history = append(history, prefix...)
		history = append(history, replay...)
	} else {
		history = s.Buffer.ReplayBytes()
	}
	s.Clients[id] = ch
	s.clientsMu.Unlock()

	if !s.IsRunning() {
		select {
		case ch <- nil:
		default:
		}
	}

	remove := func() {
		s.clientsMu.Lock()
		delete(s.Clients, id)
		s.clientsMu.Unlock()
	}
	return ch, remove, history, true
}

func (m *Manager) Forget(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *Manager) CloseAll() {
	for _, s := range m.ListAll() {
		_ = m.Close(s.ID)
	}
}

func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{"total": len(m.sessions), "max": m.cfg.MaxSessions}
}

func (s *Session) Snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"id":              s.ID,
		"project_id":      s.ProjectID,
		"title":           s.Title,
		"status":          s.Status,
		"cols":            s.Cols,
		"rows":            s.Rows,
		"created_at":      s.CreatedAt,
		"last_active":     s.LastActive,
		"mode":            s.Mode,
		"resume_id":       s.ResumeID,
		"grok_session_id": s.GrokSessionID,
		"model":           s.Model,
		"permission_mode": s.PermissionMode,
		"sandbox":         s.Sandbox,
		"yolo":            s.Yolo,
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func MarshalLayout(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
