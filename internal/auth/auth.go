package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
)

type contextKey string

const UserContextKey contextKey = "user"

type User struct {
	ID       string
	Username string
}

type Service struct {
	db        *db.DB
	cfg       *config.Config
	jwtSecret []byte
	wa        *webauthn.WebAuthn
	waMu      sync.RWMutex

	// pending challenges: session token -> data
	regSessions   map[string]*regSession
	loginSessions map[string]*loginSession
	mu            sync.Mutex
}

type regSession struct {
	UserID    string
	Session   *webauthn.SessionData
	ExpiresAt time.Time
}

type loginSession struct {
	Session      *webauthn.SessionData
	ExpiresAt    time.Time
	Username     string
	Discoverable bool
}

func NewService(d *db.DB, cfg *config.Config) (*Service, error) {
	// ensure jwt secret persisted
	secretStr := d.EnsureJWTSecret(cfg.JWTSecret)
	cfg.JWTSecret = secretStr
	s := &Service{
		db:            d,
		cfg:           cfg,
		jwtSecret:     []byte(secretStr),
		regSessions:   make(map[string]*regSession),
		loginSessions: make(map[string]*loginSession),
	}
	if err := s.initWebAuthn(); err != nil {
		return nil, err
	}
	go s.cleanupLoop()
	return s, nil
}

func (s *Service) initWebAuthn() error {
	s.waMu.Lock()
	defer s.waMu.Unlock()
	publicURL := s.cfg.PublicURL
	if publicURL == "" {
		if v, ok := s.db.GetSetting("public_url"); ok && v != "" {
			publicURL = v
		}
	}
	rpID := s.cfg.RPID()
	rpOrigins := s.cfg.RPOrigins()
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil && u.Hostname() != "" {
			rpID = u.Hostname()
			rpOrigins = append([]string{publicURL}, rpOrigins...)
			// dedup
			seen := map[string]bool{}
			var dedup []string
			for _, o := range rpOrigins {
				o = strings.TrimSuffix(o, "/")
				if !seen[o] {
					seen[o] = true
					dedup = append(dedup, o)
				}
			}
			rpOrigins = dedup
		}
	}
	// Ensure origins are valid URLs
	rpDisplayName := "Grok Build WebUI"
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return fmt.Errorf("webauthn init: %w", err)
	}
	s.wa = wa
	return nil
}

func (s *Service) ReloadWebAuthn() error {
	return s.initWebAuthn()
}

func (s *Service) cleanupLoop() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.regSessions {
			if now.After(v.ExpiresAt) {
				delete(s.regSessions, k)
			}
		}
		for k, v := range s.loginSessions {
			if now.After(v.ExpiresAt) {
				delete(s.loginSessions, k)
			}
		}
		s.mu.Unlock()
		s.db.PurgeRevoked()
	}
}

// JWT

const tokenTTL = 12 * time.Hour

func (s *Service) GenerateToken(userID, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(tokenTTL).Unix(),
		"iat":      time.Now().Unix(),
		"jti":      uuid.NewString(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.jwtSecret)
}

func (s *Service) parseClaims(tokenStr string) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	jti, _ := claims["jti"].(string)
	if s.db.IsRevoked(jti) {
		return nil, fmt.Errorf("revoked")
	}
	return claims, nil
}

func (s *Service) VerifyToken(tokenStr string) (*User, error) {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}
	sub, _ := claims["sub"].(string)
	uname, _ := claims["username"].(string)
	if sub == "" {
		return nil, fmt.Errorf("no sub")
	}
	return &User{ID: sub, Username: uname}, nil
}

func (s *Service) RefreshToken(tokenStr string) (string, *User, error) {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return "", nil, err
	}
	sub, _ := claims["sub"].(string)
	uname, _ := claims["username"].(string)
	if sub == "" {
		return "", nil, fmt.Errorf("no sub")
	}
	if jti, _ := claims["jti"].(string); jti != "" {
		exp := time.Now().Add(tokenTTL)
		if v, ok := claims["exp"].(float64); ok {
			exp = time.Unix(int64(v), 0)
		}
		_ = s.db.RevokeToken(jti, exp)
	}
	tok, err := s.GenerateToken(sub, uname)
	if err != nil {
		return "", nil, err
	}
	return tok, &User{ID: sub, Username: uname}, nil
}

func (s *Service) RevokeRequest(r *http.Request) {
	tokenStr := ""
	if c, err := r.Cookie("grok_token"); err == nil {
		tokenStr = c.Value
	}
	if tokenStr == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if tokenStr == "" {
		return
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil || tok == nil {
		return
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return
	}
	exp := time.Now().Add(tokenTTL)
	if v, ok := claims["exp"].(float64); ok {
		exp = time.Unix(int64(v), 0)
	}
	_ = s.db.RevokeToken(jti, exp)
}

func (s *Service) VerifyRequest(r *http.Request) (*User, error) {
	// cookie first
	if c, err := r.Cookie("grok_token"); err == nil && c.Value != "" {
		if u, err := s.VerifyToken(c.Value); err == nil {
			return u, nil
		}
	}
	// Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		tok := strings.TrimPrefix(auth, "Bearer ")
		if u, err := s.VerifyToken(tok); err == nil {
			return u, nil
		}
	}
	return nil, fmt.Errorf("unauthorized")
}

func (s *Service) SetCookie(w http.ResponseWriter, token string, r *http.Request) {
	secure := false
	if r != nil {
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || r.Header.Get("X-Forwarded-Ssl") == "on" {
			secure = true
		}
		// Also consider forwarded host scheme if via cloudflare tunnel: check X-Forwarded-Proto already covers
		// Do not use cfg.IsSecure alone for http localhost; only secure when request is actually https
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "grok_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(tokenTTL.Seconds()),
	})
}

func (s *Service) ClearCookie(w http.ResponseWriter, r *http.Request) {
	secure := false
	if r != nil {
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || r.Header.Get("X-Forwarded-Ssl") == "on" {
			secure = true
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "grok_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// User management

func (s *Service) HasUsers() bool {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count > 0
}

func (s *Service) CreateUser(username, password string) (*db.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 8 {
		return nil, fmt.Errorf("username required and password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(?,?,?,?)`, id, username, string(hash), now)
	if err != nil {
		return nil, err
	}
	return &db.User{ID: id, Username: username, PasswordHash: string(hash), CreatedAt: now}, nil
}

func (s *Service) VerifyPassword(username, password string) (*db.User, error) {
	var u db.User
	var created string
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &created)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}
	// parse time
	if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
		u.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", created); err == nil {
		u.CreatedAt = t
	}
	return &u, nil
}

func (s *Service) GetUserByID(id string) (*db.User, error) {
	var u db.User
	var created string
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &created)
	if err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
		u.CreatedAt = t
	}
	return &u, nil
}

// WebAuthn helpers

type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
	// internal
	userID string
}

func (u WebAuthnUser) WebAuthnID() []byte                         { return u.ID }
func (u WebAuthnUser) WebAuthnName() string                       { return u.Name }
func (u WebAuthnUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }
func (u WebAuthnUser) WebAuthnIcon() string                       { return "" }

func (s *Service) loadWebAuthnUser(username string) (*WebAuthnUser, error) {
	var id, uname string
	err := s.db.QueryRow(`SELECT id, username FROM users WHERE username=?`, username).Scan(&id, &uname)
	if err != nil {
		return nil, err
	}
	return s.loadWebAuthnUserByID(id)
}

func (s *Service) loadWebAuthnUserByID(userID string) (*WebAuthnUser, error) {
	var id, uname string
	err := s.db.QueryRow(`SELECT id, username FROM users WHERE id=?`, userID).Scan(&id, &uname)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT credential FROM webauthn_credentials WHERE user_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []webauthn.Credential
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			continue
		}
		var c webauthn.Credential
		if err := json.Unmarshal(blob, &c); err != nil {
			continue
		}
		creds = append(creds, c)
	}
	// webauthn user handle is id bytes
	return &WebAuthnUser{
		ID:          []byte(id),
		Name:        uname,
		DisplayName: uname,
		Credentials: creds,
		userID:      id,
	}, nil
}

// Registration

func (s *Service) BeginRegistration(userID string) (*protocol.CredentialCreation, string, error) {
	wu, err := s.loadWebAuthnUserByID(userID)
	if err != nil {
		return nil, "", err
	}
	s.waMu.RLock()
	wa := s.wa
	s.waMu.RUnlock()
	options, session, err := wa.BeginRegistration(wu)
	if err != nil {
		return nil, "", err
	}
	token := uuid.NewString()
	s.mu.Lock()
	s.regSessions[token] = &regSession{UserID: userID, Session: session, ExpiresAt: time.Now().Add(5 * time.Minute)}
	s.mu.Unlock()
	return options, token, nil
}

func (s *Service) FinishRegistration(r *http.Request, token string, credName string) error {
	s.mu.Lock()
	rs, ok := s.regSessions[token]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("session not found or expired")
	}
	delete(s.regSessions, token)
	s.mu.Unlock()

	wu, err := s.loadWebAuthnUserByID(rs.UserID)
	if err != nil {
		return err
	}
	s.waMu.RLock()
	wa := s.wa
	s.waMu.RUnlock()
	cred, err := wa.FinishRegistration(wu, *rs.Session, r)
	if err != nil {
		return err
	}
	blob, _ := json.Marshal(cred)
	id := uuid.NewString()
	_, err = s.db.Exec(`INSERT INTO webauthn_credentials(id,user_id,name,credential,created_at) VALUES(?,?,?,?,?)`, id, rs.UserID, credName, blob, time.Now().UTC())
	return err
}

func (s *Service) BeginLogin(username string) (*protocol.CredentialAssertion, string, error) {
	s.waMu.RLock()
	wa := s.wa
	s.waMu.RUnlock()
	var options *protocol.CredentialAssertion
	var session *webauthn.SessionData
	var err error
	if username != "" {
		wu, err2 := s.loadWebAuthnUser(username)
		if err2 != nil {
			return nil, "", fmt.Errorf("user not found")
		}
		options, session, err = wa.BeginLogin(wu)
	} else {
		options, session, err = wa.BeginDiscoverableLogin()
	}
	if err != nil {
		return nil, "", err
	}
	token := uuid.NewString()
	s.mu.Lock()
	s.loginSessions[token] = &loginSession{
		Session:      session,
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		Username:     username,
		Discoverable: username == "",
	}
	s.mu.Unlock()
	return options, token, nil
}

func (s *Service) FinishLogin(r *http.Request, token string) (*db.User, error) {
	s.mu.Lock()
	ls, ok := s.loginSessions[token]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("session not found or expired")
	}
	delete(s.loginSessions, token)
	s.mu.Unlock()

	s.waMu.RLock()
	wa := s.wa
	s.waMu.RUnlock()

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) > 0 {
			return s.loadWebAuthnUserByID(string(userHandle))
		}
		if ls.Username != "" {
			return s.loadWebAuthnUser(ls.Username)
		}
		uid, err := s.userIDForCredential(rawID)
		if err != nil {
			return nil, err
		}
		return s.loadWebAuthnUserByID(uid)
	}

	if !ls.Discoverable && ls.Username != "" {
		wu, err := s.loadWebAuthnUser(ls.Username)
		if err != nil {
			return nil, err
		}
		if _, err := wa.FinishLogin(wu, *ls.Session, r); err != nil {
			return nil, fmt.Errorf("webauthn login failed: %w", err)
		}
		return s.GetUserByID(wu.userID)
	}

	credential, err := wa.FinishDiscoverableLogin(handler, *ls.Session, r)
	if err != nil {
		return nil, fmt.Errorf("webauthn login failed: %w", err)
	}
	uid, err := s.userIDForCredential(credential.ID)
	if err != nil {
		if len(credential.ID) == 0 {
			return nil, fmt.Errorf("user lookup failed after discoverable login")
		}
		return nil, err
	}
	return s.GetUserByID(uid)
}

func (s *Service) userIDForCredential(credID []byte) (string, error) {
	rows, err := s.db.Query(`SELECT user_id, credential FROM webauthn_credentials`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		var blob []byte
		if err := rows.Scan(&uid, &blob); err != nil {
			continue
		}
		var c webauthn.Credential
		if err := json.Unmarshal(blob, &c); err != nil {
			continue
		}
		if bytes.Equal(c.ID, credID) {
			return uid, nil
		}
	}
	return "", fmt.Errorf("user not found for credential")
}

func (s *Service) ListCredentials(userID string) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM webauthn_credentials WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, name, created string
		if err := rows.Scan(&id, &name, &created); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{"id": id, "name": name, "created_at": created})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return out, nil
}

func (s *Service) DeleteCredential(userID, credID string) error {
	res, err := s.db.Exec(`DELETE FROM webauthn_credentials WHERE id=? AND user_id=?`, credID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// allow public endpoints
		if strings.HasPrefix(r.URL.Path, "/api/auth/setup") || strings.HasPrefix(r.URL.Path, "/api/auth/login") || r.URL.Path == "/api/auth/setup-required" || strings.HasPrefix(r.URL.Path, "/api/auth/webauthn/login") {
			next.ServeHTTP(w, r)
			return
		}
		// also allow static assets without auth? No, protect API only. Static is protected via JS check, but we can allow.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		user, err := s.VerifyRequest(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUser(r *http.Request) *User {
	if v := r.Context().Value(UserContextKey); v != nil {
		if u, ok := v.(*User); ok {
			return u
		}
	}
	return nil
}

// Rate limiting (simple in-memory)
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{attempts: make(map[string][]time.Time)}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	window := now.Add(-1 * time.Minute)
	arr := rl.attempts[ip]
	// filter
	var filtered []time.Time
	for _, t := range arr {
		if t.After(window) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= 5 {
		rl.attempts[ip] = filtered
		return false
	}
	filtered = append(filtered, now)
	rl.attempts[ip] = filtered
	return true
}

func RandomToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hexEncode(b)
}

func hexEncode(b []byte) string {
	// avoid import hex repeatedly
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}
