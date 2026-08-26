package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"grok-build-webui/internal/auth"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
	"grok-build-webui/internal/handlers"
	"grok-build-webui/internal/middleware"
	"grok-build-webui/internal/session"
	webfs "grok-build-webui/web"
)

// version is stamped at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cfg := config.Load()
	log.Printf("starting grok-build-webui %s data=%s grok=%s public_url=%s", cfg.Addr, cfg.DataDir, cfg.GrokBin, cfg.PublicURL)
	log.Printf("version %s", version)

	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if cfg.PublicURL == "" {
		if v, ok := database.GetSetting("public_url"); ok && v != "" {
			cfg.PublicURL = v
		}
	}
	if v, ok := database.GetSetting("jwt_secret"); ok && v != "" {
		cfg.JWTSecret = v
	}

	authSvc, err := auth.NewService(database, cfg)
	if err != nil {
		log.Fatalf("auth init: %v", err)
	}

	manager := session.NewManager(cfg, database)

	authH := handlers.NewAuthHandler(authSvc)
	projectH := handlers.NewProjectHandlerWithManager(database, manager, cfg)
	sessionH := handlers.NewSessionHandler(database, manager)
	wsH := handlers.NewWSHandler(authSvc, manager, cfg, database)
	settingsH := handlers.NewSettingsHandler(database, cfg, authSvc, manager)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/setup-required", authH.SetupRequired)
	mux.HandleFunc("POST /api/auth/setup", authH.Setup)
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/webauthn/login/begin", authH.LoginBegin)
	mux.HandleFunc("POST /api/auth/webauthn/login/finish", authH.LoginFinish)

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/auth/logout", authH.Logout)
	protected.HandleFunc("POST /api/auth/refresh", authH.Refresh)
	protected.HandleFunc("GET /api/auth/me", authH.Me)
	protected.HandleFunc("POST /api/auth/webauthn/register/begin", authH.RegisterBegin)
	protected.HandleFunc("POST /api/auth/webauthn/register/finish", authH.RegisterFinish)
	protected.HandleFunc("GET /api/auth/webauthn/credentials", authH.ListCredentials)
	protected.HandleFunc("DELETE /api/auth/webauthn/credentials/{id}", authH.DeleteCredential)

	protected.HandleFunc("GET /api/projects", projectH.List)
	protected.HandleFunc("POST /api/projects", projectH.Create)
	protected.HandleFunc("GET /api/projects/{id}", projectH.Get)
	protected.HandleFunc("PUT /api/projects/{id}", projectH.Update)
	protected.HandleFunc("DELETE /api/projects/{id}", projectH.Delete)
	protected.HandleFunc("GET /api/projects/{id}/conversations", projectH.Conversations)
	protected.HandleFunc("GET /api/browse", projectH.Browse)

	protected.HandleFunc("GET /api/projects/{id}/sessions", sessionH.ListByProject)
	protected.HandleFunc("POST /api/projects/{id}/sessions", sessionH.Create)
	protected.HandleFunc("GET /api/sessions/{id}", sessionH.Get)
	protected.HandleFunc("PATCH /api/sessions/{id}", sessionH.Patch)
	protected.HandleFunc("DELETE /api/sessions/{id}", sessionH.Delete)
	protected.HandleFunc("POST /api/sessions/{id}/resize", sessionH.Resize)
	protected.HandleFunc("POST /api/sessions/{id}/restore", sessionH.Restore)
	protected.HandleFunc("GET /api/sessions/{id}/ws", wsH.ServeHTTP)

	protected.HandleFunc("GET /api/settings", settingsH.Get)
	protected.HandleFunc("PUT /api/settings", settingsH.Update)
	protected.HandleFunc("GET /api/settings/grok", settingsH.GetGrok)
	protected.HandleFunc("PUT /api/settings/grok", settingsH.UpdateGrok)

	mux.Handle("/api/auth/logout", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/refresh", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/me", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/webauthn/register/begin", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/webauthn/register/finish", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/webauthn/credentials", authMiddleware(authSvc, protected))
	mux.Handle("/api/auth/webauthn/credentials/", authMiddleware(authSvc, protected))
	mux.Handle("/api/projects", authMiddleware(authSvc, protected))
	mux.Handle("/api/projects/", authMiddleware(authSvc, protected))
	mux.Handle("/api/sessions/", authMiddleware(authSvc, protected))
	mux.Handle("/api/settings", authMiddleware(authSvc, protected))
	mux.Handle("/api/settings/", authMiddleware(authSvc, protected))
	mux.Handle("/api/browse", authMiddleware(authSvc, protected))

	webSub := webfs.FS
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(webSub, path); err != nil {
			path = "index.html"
		}
		http.ServeFileFS(w, r, webSub, path)
	})

	handler := middleware.CORS(cfg, database)(mux)
	handler = middleware.SecurityHeaders(handler)
	handler = loggingMiddleware(handler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		log.Printf("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("listening on %s", cfg.Addr)
	if publicURL := cfg.PublicURL; publicURL != "" {
		log.Printf("public URL: %s (CORS/WebAuthn origin)", publicURL)
	} else {
		log.Printf("public URL not set — use Settings UI or --public-url flag when tunneling via Cloudflare")
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
	manager.StopAll()
}

func authMiddleware(svc *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := svc.VerifyRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.Contains(r.URL.Path, "/ws") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}
