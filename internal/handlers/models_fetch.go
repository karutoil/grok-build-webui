package handlers

// Provider-side model discovery and models.dev defaults for the settings UI.
//
// FetchModels lets the WebUI ask a provider endpoint (OpenAI-compatible
// /v1/models, Anthropic /v1/models, …) which model IDs it serves, so a new
// [model.<id>] or [model_providers.<id>] entry can pick from a list instead of
// typing IDs by hand. ModelInfo looks an ID up in the models.dev catalog to
// prefill sensible defaults: display name, context window, max output tokens,
// and reasoning-effort support. The API key given to the probe lives only in
// that request; it is never persisted. These endpoints are admin-authenticated
// like every other route here — they are intended for the server operator's
// own use when wiring up BYOK endpoints.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultModelsDevURL = "https://models.dev/api.json"
	modelsDevTTLSecs    = 12 * time.Hour

	probeTimeout   = 15 * time.Second
	catalogTimeout = 30 * time.Second

	maxProbeBody   = 8 << 20 // 8 MiB is plenty for any /models listing
	maxCatalogBody = 32 << 20

	anthropicVersionHeader = "2023-06-01"

	errBodySnippet = 300
)

var (
	probeHTTPClient   = &http.Client{Timeout: probeTimeout}
	catalogHTTPClient = &http.Client{Timeout: catalogTimeout}

	catalogMu      sync.Mutex
	catalogCache   []byte
	catalogFetched time.Time
)

type fetchModelsRequest struct {
	BaseURL      string            `json:"base_url"`
	ModelsURL    string            `json:"models_url"`
	APIBackend   string            `json:"api_backend"`
	APIKey       string            `json:"api_key"`
	ExtraHeaders map[string]string `json:"extra_headers"`
}

type remoteModel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// buildModelsListURL turns a base URL into the provider's model-listing URL.
// An explicit override wins verbatim; otherwise "/models" is appended unless
// the caller already pointed at it.
func buildModelsListURL(baseURL, override string) (string, error) {
	raw := strings.TrimSpace(override)
	if raw == "" {
		raw = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if raw == "" {
			return "", fmt.Errorf("base_url is required")
		}
		if !strings.HasSuffix(raw, "/models") {
			raw += "/models"
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("not a valid URL: %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("URL must be http:// or https://")
	}
	return u.String(), nil
}

// buildProbeRequest assembles the outbound GET with auth headers appropriate
// to the wire protocol ("messages" endpoints expect x-api-key, everything else
// Bearer). Caller-supplied extra_headers always win.
func buildProbeRequest(in fetchModelsRequest) (*http.Request, error) {
	listURL, err := buildModelsListURL(in.BaseURL, in.ModelsURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	key := strings.TrimSpace(in.APIKey)
	if key != "" && strings.TrimSpace(in.APIBackend) == "messages" {
		req.Header.Set("x-api-key", key)
		if req.Header.Get("anthropic-version") == "" {
			req.Header.Set("anthropic-version", anthropicVersionHeader)
		}
	} else if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range in.ExtraHeaders {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	return req, nil
}

// parseRemoteModels accepts the response shapes seen in the wild:
// OpenAI {"data":[{"id":…}]}, Anthropic {"data":[{"id","display_name"}]},
// gateways returning top-level arrays, {"models":["id", …]} with plain
// strings, Ollama-style {"models":[{"name":…}]} / {"model":{…}} leaves, and
// LM-Studio-style {"model_list":[{"model_name":…}]} entries. Nested ids are
// discovered by walking any structured children; duplicates are collapsed.
func parseRemoteModels(body []byte) ([]remoteModel, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("provider did not return JSON (%v)", err)
	}
	var out []remoteModel
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			out = appendRemoteModel(out, t, "")
		case map[string]any:
			var structured []any
			for _, val := range t {
				switch val.(type) {
				case map[string]any, []any:
					structured = append(structured, val)
				}
			}
			id := pickStr(t, "id", "model", "model_name")
			// A bare {"name": …} leaf describes one model; wrappers that hold
			// lists never get their label treated as an id.
			if id == "" && len(structured) == 0 {
				id = pickStr(t, "name")
			}
			name := pickStr(t, "display_name")
			if name == "" {
				name = pickStr(t, "name")
			}
			if id != "" {
				out = appendRemoteModel(out, id, name)
			}
			for _, child := range structured {
				walk(child)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(doc)
	if len(out) == 0 {
		return nil, fmt.Errorf("no model ids found in the response")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	deduped := out[:1]
	for _, m := range out[1:] {
		if m.ID != deduped[len(deduped)-1].ID {
			deduped = append(deduped, m)
		}
	}
	return deduped, nil
}

func pickStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func appendRemoteModel(list []remoteModel, id, name string) []remoteModel {
	return append(list, remoteModel{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name)})
}

// FetchModels handles POST /api/settings/grok/fetch-models.
func (h *SettingsHandler) FetchModels(w http.ResponseWriter, r *http.Request) {
	var in fetchModelsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req, err := buildProbeRequest(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("could not reach %s: %v", req.URL.Host, err))
		return
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProbeBody))
	if resp.StatusCode >= 400 {
		snippet := string(body)
		if len(snippet) > errBodySnippet {
			snippet = snippet[:errBodySnippet]
		}
		if readErr != nil && snippet == "" {
			snippet = readErr.Error()
		}
		msg := fmt.Sprintf("%s returned HTTP %d", req.URL.Host, resp.StatusCode)
		if snippet != "" {
			msg += ": " + strings.TrimSpace(snippet)
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}
	if readErr != nil {
		writeError(w, http.StatusBadGateway, "reading provider response failed: "+readErr.Error())
		return
	}
	models, perr := parseRemoteModels(body)
	if perr != nil {
		writeError(w, http.StatusBadGateway, perr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":    req.URL.String(),
		"count":  len(models),
		"models": models,
	})
}

// ---- models.dev catalog ----

type devReasoningOption struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
}

type devModel struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Reasoning        bool                 `json:"reasoning"`
	ReasoningOptions []devReasoningOption `json:"reasoning_options"`
	Limit            struct {
		Context int64 `json:"context"`
		Output  int64 `json:"output"`
	} `json:"limit"`
}

type devProvider struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Env    []string            `json:"env"`
	API    string              `json:"api"`
	Models map[string]devModel `json:"models"`
}

// DevMatch flattens one models.dev entry into the Grok config field names so
// the UI can prefill a form directly from it.
type DevMatch struct {
	ProviderID       string   `json:"provider_id"`
	ProviderName     string   `json:"provider_name,omitempty"`
	ProviderEnvVar   string   `json:"provider_env_var,omitempty"`
	ModelID          string   `json:"model_id"`
	ModelName        string   `json:"model_name,omitempty"`
	Description      string   `json:"description,omitempty"`
	ContextWindow    int64    `json:"context_window,omitempty"`
	MaxOutputTokens  int64    `json:"max_output_tokens,omitempty"`
	Reasoning        bool     `json:"reasoning"`
	ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`
}

func modelsDevURL() string {
	if v := strings.TrimSpace(os.Getenv("GROK_WEBUI_MODELS_DEV_URL")); v != "" {
		return v
	}
	return defaultModelsDevURL
}

// catalogData returns the models.dev catalog bytes, cached for half a day.
// When a refresh fails but a stale copy exists, the stale copy is served
// rather than failing the request.
func catalogData() ([]byte, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if catalogCache != nil && time.Since(catalogFetched) < modelsDevTTLSecs {
		return catalogCache, nil
	}
	resp, err := catalogHTTPClient.Get(modelsDevURL())
	if err != nil {
		if catalogCache != nil {
			log.Printf("models.dev refresh failed, serving stale copy: %v", err)
			return catalogCache, nil
		}
		return nil, fmt.Errorf("could not reach models.dev: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBody))
	if err != nil {
		if catalogCache != nil {
			return catalogCache, nil
		}
		return nil, fmt.Errorf("downloading models.dev catalog failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(bytes.TrimSpace(body)) == 0 {
		if catalogCache != nil {
			return catalogCache, nil
		}
		return nil, fmt.Errorf("models.dev returned HTTP %d", resp.StatusCode)
	}
	catalogCache, catalogFetched = body, time.Now()
	return catalogCache, nil
}

func extractDevMatches(catalog []byte, query string) []DevMatch {
	var providers map[string]devProvider
	if err := json.Unmarshal(catalog, &providers); err != nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var matches []DevMatch
	for provKey, prov := range providers {
		envVar := ""
		if len(prov.Env) > 0 {
			envVar = prov.Env[0]
		}
		for modelKey, mdl := range prov.Models {
			idMatch := strings.EqualFold(strings.TrimSpace(mdl.ID), q)
			keyMatch := strings.EqualFold(modelKey, q)
			if !idMatch && !keyMatch {
				continue
			}
			m := DevMatch{
				ProviderID:      firstNonEmpty(prov.ID, provKey),
				ProviderName:    prov.Name,
				ProviderEnvVar:  envVar,
				ModelID:         firstNonEmpty(mdl.ID, modelKey),
				ModelName:       mdl.Name,
				Description:     mdl.Description,
				ContextWindow:   mdl.Limit.Context,
				MaxOutputTokens: mdl.Limit.Output,
				Reasoning:       mdl.Reasoning,
			}
			for _, opt := range mdl.ReasoningOptions {
				if strings.EqualFold(opt.Type, "effort") && len(opt.Values) > 0 {
					m.ReasoningEfforts = append(m.ReasoningEfforts, opt.Values...)
					break
				}
			}
			matches = append(matches, m)
		}
	}
	rankDevMatches(matches, query)
	return matches
}

// knownProviders orders the canonical source-of-truth listings ahead of
// aggregators/mirrors (e.g. openrouter re-lists everyone).
var knownProviders = map[string]int{
	"openai": 0, "anthropic": 1, "google": 2, "xai": 3, "x-ai": 3,
	"groq": 4, "mistral": 5, "deepseek": 6, "amazon-bedrock": 7,
	"azure": 8, "github-copilot": 9,
}

func rankDevMatches(matches []DevMatch, hint string) {
	hint = strings.ToLower(strings.TrimSpace(hint))
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		ascore, bscore := devRankScore(a, hint), devRankScore(b, hint)
		if ascore != bscore {
			return ascore < bscore
		}
		return a.ProviderID < b.ProviderID
	})
}

func devRankScore(m DevMatch, hint string) int {
	if hint != "" && strings.EqualFold(m.ProviderID, hint) {
		return -1
	}
	if rank, ok := knownProviders[m.ProviderID]; ok {
		return rank
	}
	return len(knownProviders)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ModelInfo handles POST /api/settings/grok/model-info.
func (h *SettingsHandler) ModelInfo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ModelID      string `json:"model_id"`
		ProviderHint string `json:"provider_hint"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(in.ModelID) == "" {
		writeError(w, http.StatusBadRequest, "model_id is required")
		return
	}
	catalog, err := catalogData()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	matches := extractDevMatches(catalog, in.ModelID)
	var best *DevMatch
	if len(matches) > 0 {
		best = &matches[0]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   in.ModelID,
		"best":    best,
		"matches": matches,
	})
}
