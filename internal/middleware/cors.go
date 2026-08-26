package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
)

func CORS(cfg *config.Config, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed := OriginAllowed(origin, r, cfg, database)
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				if allowed {
					w.WriteHeader(http.StatusNoContent)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OriginAllowed reports whether origin may talk to this server (CORS + WebSocket).
func OriginAllowed(origin string, r *http.Request, cfg *config.Config, database *db.DB) bool {
	if origin == "" {
		return true
	}
	if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
		return true
	}
	if origin == "http://localhost" || origin == "http://127.0.0.1" {
		return true
	}

	originU, err := url.Parse(origin)
	if err != nil || originU.Host == "" {
		return false
	}

	// Same-host as the request itself (direct tunnel / reverse proxy).
	if r != nil {
		reqHost := r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			reqHost = strings.Split(fwd, ",")[0]
			reqHost = strings.TrimSpace(reqHost)
		}
		if strings.EqualFold(originU.Host, reqHost) {
			return true
		}
	}

	publicURL := cfg.PublicURL
	if publicURL == "" && database != nil {
		if v, ok := database.GetSetting("public_url"); ok {
			publicURL = v
		}
	}
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil && u.Host != "" {
			if strings.EqualFold(u.Host, originU.Host) && strings.EqualFold(u.Scheme, originU.Scheme) {
				return true
			}
			if origin == strings.TrimSuffix(publicURL, "/") {
				return true
			}
		}
	}

	for _, o := range cfg.RPOrigins() {
		if o == origin {
			return true
		}
		u, err := url.Parse(o)
		if err == nil && u.Host != "" && strings.EqualFold(u.Host, originU.Host) {
			return true
		}
	}
	return false
}
