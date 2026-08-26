package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "grok-webui.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(1)
	if err := sdb.Ping(); err != nil {
		return nil, err
	}
	d := &DB{sdb}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS webauthn_credentials (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT,
			credential BLOB NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT UNIQUE NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			cols INTEGER NOT NULL,
			rows INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			last_active DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS revoked_tokens (
			jti TEXT PRIMARY KEY,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_revoked_exp ON revoked_tokens(expires_at)`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	alters := []string{
		`ALTER TABLE sessions ADD COLUMN mode TEXT NOT NULL DEFAULT 'new'`,
		`ALTER TABLE sessions ADD COLUMN resume_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN grok_session_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN permission_mode TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN sandbox TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN yolo INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE projects ADD COLUMN layout TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN active_tab TEXT NOT NULL DEFAULT ''`,
	}
	for _, s := range alters {
		_, _ = d.Exec(s) // ignore duplicate-column on existing DBs
	}
	return nil
}

func (d *DB) GetSetting(key string) (string, bool) {
	var v string
	err := d.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		return "", false
	}
	return v, true
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (d *DB) EnsureJWTSecret(fallback string) string {
	if v, ok := d.GetSetting("jwt_secret"); ok && v != "" {
		return v
	}
	_ = d.SetSetting("jwt_secret", fallback)
	return fallback
}

func (d *DB) RevokeToken(jti string, expiresAt time.Time) error {
	_, err := d.Exec(`INSERT OR REPLACE INTO revoked_tokens(jti, expires_at) VALUES(?,?)`, jti, expiresAt.UTC())
	return err
}

func (d *DB) IsRevoked(jti string) bool {
	if jti == "" {
		return false
	}
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM revoked_tokens WHERE jti=? AND expires_at > ?`, jti, time.Now().UTC()).Scan(&n)
	return n > 0
}

func (d *DB) PurgeRevoked() {
	_, _ = d.Exec(`DELETE FROM revoked_tokens WHERE expires_at < ?`, time.Now().UTC())
}

type User struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	Layout    string    `json:"layout,omitempty"`
	ActiveTab string    `json:"active_tab,omitempty"`
}

type SessionRow struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	Cols          int       `json:"cols"`
	Rows          int       `json:"rows"`
	CreatedAt     time.Time `json:"created_at"`
	LastActive    time.Time `json:"last_active"`
	Mode          string    `json:"mode"`
	ResumeID      string    `json:"resume_id"`
	GrokSessionID string    `json:"grok_session_id"`
	Model         string    `json:"model"`
	PermissionMode string   `json:"permission_mode"`
	Sandbox       string    `json:"sandbox"`
	Yolo          bool      `json:"yolo"`
}
