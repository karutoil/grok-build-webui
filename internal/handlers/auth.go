package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"grok-build-webui/internal/auth"
)

type AuthHandler struct {
	auth    *auth.Service
	limiter *auth.RateLimiter
}

func NewAuthHandler(a *auth.Service) *AuthHandler {
	return &AuthHandler{auth: a, limiter: auth.NewRateLimiter()}
}

func (h *AuthHandler) SetupRequired(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"setup_required": !h.auth.HasUsers()})
}

func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	if h.auth.HasUsers() {
		writeError(w, http.StatusBadRequest, "already setup")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := h.auth.CreateUser(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, err := h.auth.GenerateToken(u.ID, u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}
	h.auth.SetCookie(w, token, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "username": u.Username})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := h.auth.VerifyPassword(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := h.auth.GenerateToken(u.ID, u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}
	h.auth.SetCookie(w, token, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "username": u.Username})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.RevokeRequest(r)
	h.auth.ClearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	tokenStr := ""
	if c, err := r.Cookie("grok_token"); err == nil {
		tokenStr = c.Value
	}
	if tokenStr == "" {
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Bearer ") {
			tokenStr = strings.TrimPrefix(authz, "Bearer ")
		}
	}
	if tokenStr == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tok, user, err := h.auth.RefreshToken(tokenStr)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.auth.SetCookie(w, tok, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "username": user.Username})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.VerifyRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": user.ID, "username": user.Username})
}

func (h *AuthHandler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.VerifyRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	opts, token, err := h.auth.BeginRegistration(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, webauthnCookie("webauthn_reg", token, 300, r))
	writeJSON(w, http.StatusOK, opts)
}

func (h *AuthHandler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.VerifyRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	token := webauthnToken(r, "webauthn_reg")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing token")
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "passkey"
	}
	if err := h.auth.FinishRegistration(r, token, name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.SetCookie(w, webauthnCookie("webauthn_reg", "", -1, r))
	_ = user
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) LoginBegin(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	opts, token, err := h.auth.BeginLogin(req.Username)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.SetCookie(w, webauthnCookie("webauthn_login", token, 300, r))
	writeJSON(w, http.StatusOK, opts)
}

func (h *AuthHandler) LoginFinish(w http.ResponseWriter, r *http.Request) {
	token := webauthnToken(r, "webauthn_login")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing token")
		return
	}
	u, err := h.auth.FinishLogin(r, token)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tokStr, err := h.auth.GenerateToken(u.ID, u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token error")
		return
	}
	h.auth.SetCookie(w, tokStr, r)
	http.SetCookie(w, webauthnCookie("webauthn_login", "", -1, r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "username": u.Username})
}

func (h *AuthHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.VerifyRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := h.auth.ListCredentials(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *AuthHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	user, err := h.auth.VerifyRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		id = lastSegment(r.URL.Path)
	}
	if err := h.auth.DeleteCredential(user.ID, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func webauthnToken(r *http.Request, cookieName string) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	if v := r.Header.Get("X-WebAuthn-Token"); v != "" {
		return v
	}
	return r.URL.Query().Get("token")
}

func webauthnCookie(name, value string, maxAge int, r *http.Request) *http.Cookie {
	secure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || r.Header.Get("X-Forwarded-Ssl") == "on") {
		secure = true
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}
