package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"grok-build-webui/internal/config"
)

func TestOriginAllowedLocalhost(t *testing.T) {
	cfg := &config.Config{}
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
	r.Host = "localhost:8080"
	if !OriginAllowed("http://localhost:8080", r, cfg, nil) {
		t.Fatal("localhost origin should be allowed")
	}
}

func TestOriginAllowedSameHost(t *testing.T) {
	cfg := &config.Config{}
	r := httptest.NewRequest(http.MethodGet, "https://abc.trycloudflare.com/", nil)
	r.Host = "abc.trycloudflare.com"
	r.Header.Set("X-Forwarded-Host", "abc.trycloudflare.com")
	if !OriginAllowed("https://abc.trycloudflare.com", r, cfg, nil) {
		t.Fatal("same-host tunnel origin should be allowed")
	}
}

func TestOriginAllowedPublicURL(t *testing.T) {
	cfg := &config.Config{PublicURL: "https://grok.example.com"}
	r := httptest.NewRequest(http.MethodGet, "https://grok.example.com/", nil)
	r.Host = "grok.example.com"
	if !OriginAllowed("https://grok.example.com", r, cfg, nil) {
		t.Fatal("configured public URL origin should be allowed")
	}
	if OriginAllowed("https://evil.example", r, cfg, nil) {
		t.Fatal("unrelated origin must be rejected")
	}
}
