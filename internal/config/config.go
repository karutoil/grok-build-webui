package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr        string
	DataDir     string
	PublicURL   string
	JWTSecret   string
	GrokBin     string
	GrokHome    string
	AllowRoot   string
	MaxSessions int
	CleanEnv    bool
	StartedAt   time.Time
}

func Load() *Config {
	c := &Config{StartedAt: time.Now()}
	var publicURL string
	var maxSessions int
	flag.StringVar(&c.Addr, "addr", getEnv("GROK_WEBUI_ADDR", ":8080"), "listen address")
	flag.StringVar(&c.DataDir, "data", getEnv("GROK_WEBUI_DATA", "./data"), "data directory")
	flag.StringVar(&publicURL, "public-url", getEnv("GROK_WEBUI_PUBLIC_URL", ""), "public URL for CORS/WebAuthn (e.g. https://example.trycloudflare.com)")
	flag.StringVar(&c.JWTSecret, "jwt-secret", getEnv("GROK_WEBUI_JWT_SECRET", ""), "JWT secret (auto-generated if empty)")
	flag.StringVar(&c.GrokBin, "grok-bin", getEnv("GROK_WEBUI_GROK_BIN", "grok"), "grok binary path")
	flag.StringVar(&c.GrokHome, "grok-home", getEnv("GROK_HOME", getEnv("GROK_WEBUI_GROK_HOME", "")), "Grok home directory (default ~/.grok)")
	flag.StringVar(&c.AllowRoot, "allow-root", getEnv("GROK_WEBUI_ROOT", ""), "optional allowlist root for project paths")
	flag.IntVar(&maxSessions, "max-sessions", getEnvInt("GROK_WEBUI_MAX_SESSIONS", 16), "max concurrent running PTYs")
	flag.BoolVar(&c.CleanEnv, "clean-env", getEnv("GROK_WEBUI_CLEAN_ENV", "") == "1", "do not pass host environment into PTYs (except a small allowlist)")
	flag.Parse()
	c.PublicURL = strings.TrimSuffix(strings.TrimSpace(publicURL), "/")
	c.AllowRoot = strings.TrimSpace(c.AllowRoot)
	c.MaxSessions = maxSessions
	if c.MaxSessions <= 0 {
		c.MaxSessions = 16
	}
	if c.JWTSecret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		c.JWTSecret = hex.EncodeToString(b)
	}
	return c
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func (c *Config) EffectivePublicURL() string {
	return c.PublicURL
}

func (c *Config) RPID() string {
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return "localhost"
}

func (c *Config) RPOrigins() []string {
	origins := []string{"http://localhost:8080", "http://127.0.0.1:8080", "http://localhost:3000"}
	if c.PublicURL != "" {
		origins = append(origins, c.PublicURL)
		if strings.HasPrefix(c.PublicURL, "http://") {
			origins = append(origins, "https://"+strings.TrimPrefix(c.PublicURL, "http://"))
		}
	}
	return origins
}

func (c *Config) IsSecure() bool {
	return c.PublicURL != "" && strings.HasPrefix(c.PublicURL, "https://")
}
