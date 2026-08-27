package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestBuildModelsListURL(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		override string
		want     string
		wantErr  bool
	}{
		{"appends /models", "https://api.openai.com/v1", "", "https://api.openai.com/v1/models", false},
		{"trims trailing slash", "https://api.openai.com/v1/", "", "https://api.openai.com/v1/models", false},
		{"override wins verbatim", "https://api.openai.com/v1", "http://gw.local/list", "http://gw.local/list", false},
		{"already points at models", "https://api.openai.com/v1/models", "", "https://api.openai.com/v1/models", false},
		{"bare host ok", "https://llm.internal:8080", "", "https://llm.internal:8080/models", false},
		{"empty base errors", "   ", "", "", true},
		{"missing scheme errors", "api.openai.com/v1", "", "", true},
		{"ftp scheme errors", "ftp://files/x", "", "", true},
		{"garbage override errors", "https://ok.example", "::::", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildModelsListURL(tc.base, tc.override)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildProbeRequestAuth(t *testing.T) {
	extra := map[string]string{"x-org": "acme", "": "skipped"}

	req, err := buildProbeRequest(fetchModelsRequest{
		BaseURL: "https://api.openai.com/v1", APIKey: "sk-1", ExtraHeaders: extra,
	})
	if err != nil {
		t.Fatalf("openai-style request: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-1" {
		t.Errorf("Authorization = %q, want bearer", got)
	}

	req, err = buildProbeRequest(fetchModelsRequest{
		BaseURL: "https://api.anthropic.com/v1", APIKey: "key-2", APIBackend: "messages",
	})
	if err != nil {
		t.Fatalf("anthropic-style request: %v", err)
	}
	if got := req.Header.Get("x-api-key"); got != "key-2" {
		t.Errorf("x-api-key = %q, want the key", got)
	}
	if req.Header.Get("anthropic-version") == "" {
		t.Error("messages backend should set a default anthropic-version")
	}

	// extra_headers must win over defaults.
	req, err = buildProbeRequest(fetchModelsRequest{
		BaseURL: "https://api.anthropic.com/v1", APIBackend: "messages",
		ExtraHeaders: map[string]string{"anthropic-version": "2099-01-01"},
	})
	if err != nil {
		t.Fatalf("header override request: %v", err)
	}
	if got := req.Header.Get("anthropic-version"); got != "2099-01-01" {
		t.Errorf("extra_headers should win; anthropic-version = %q", got)
	}
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
}

func TestParseRemoteModels(t *testing.T) {
	openAI := []byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"},{"id":"gpt-4.1-mini"}]}`)
	models, err := parseRemoteModels(openAI)
	if err != nil || len(models) != 2 || models[0].ID != "gpt-4.1-mini" {
		t.Fatalf("openai shape: %v %+v", err, models)
	}

	anthropic := []byte(`{"data":[{"id":"claude-opus-4","display_name":"Claude Opus 4"},{"type":"model","id":"claude-haiku"}]}`)
	models, err = parseRemoteModels(anthropic)
	if err != nil || len(models) != 2 {
		t.Fatalf("anthropic shape: %v %+v", err, models)
	}
	if models[0].ID != "claude-haiku" || models[1].Name != "Claude Opus 4" {
		t.Fatalf("anthropic entries wrong: %+v", models)
	}

	stringsShape := []byte(`{"models":["m-a","m-b"]}`)
	models, err = parseRemoteModels(stringsShape)
	if err != nil || len(models) != 2 || models[0].ID != "m-a" {
		t.Fatalf("models-of-strings shape: %v %+v", err, models)
	}

	topLevel := []byte(`["zeta","alpha","alpha"]`)
	models, err = parseRemoteModels(topLevel)
	if err != nil || len(models) != 2 || models[0].ID != "alpha" || models[1].ID != "zeta" {
		t.Fatalf("top-level array + dedupe/sort: %v %+v", err, models)
	}

	nested := []byte(`{"model_list":[{"model_name":"llama3"},{"model":{"id":"deepseek-r1"}}]}`)
	models, err = parseRemoteModels(nested)
	if err != nil {
		t.Fatalf("nested walk stopped early on unknown keys: %v %d", err, len(models))
	}
	if models[0].ID != "deepseek-r1" || models[1].ID != "llama3" {
		t.Fatalf("nested entries wrong: %+v", models)
	}

	for name, bad := range map[string][]byte{
		"html page":   []byte("<html><body>502</body></html>"),
		"no ids":      []byte(`{"data":[{"object":"model"}]}`),
		"not json":    []byte(` SERVER ERROR `),
		"wrong types": []byte(`{"data":{"a":1}}`),
	} {
		if _, err := parseRemoteModels(bad); err == nil {
			t.Errorf("%s: expected error", name)
		} else if !strings.Contains(err.Error(), "model") && !strings.Contains(err.Error(), "JSON") {
			t.Errorf("%s: error should mention models/JSON, got %q", name, err.Error())
		}
	}
}

const devCatalogFixture = `{
  "openai": {
    "id": "openai", "name": "OpenAI", "env": ["OPENAI_API_KEY"],
    "api": "https://api.openai.com/v1",
    "models": {
      "gpt-test": {"id": "gpt-test", "name": "GPT Test", "description": "fixture model",
        "reasoning": true,
        "reasoning_options": [{"type": "effort", "values": ["high", "max"]}],
        "limit": {"context": 400000, "output": 32000}}
    }
  },
  "openrouter": {
    "id": "openrouter", "name": "OpenRouter", "env": ["OPENROUTER_API_KEY"],
    "models": {
      "mirror/gpt-test": {"id": "gpt-test", "name": "GPT Test (mirrored)",
        "reasoning": true, "limit": {"context": 99999}}
    }
  },
  "ollama": {
    "id": "ollama", "name": "Ollama",
    "models": {
      "tiny-local": {"id": "tiny-local", "name": "Tiny Local", "limit": {}}
    }
  }
}`

func flatten(match DevMatch) string {
	return match.ProviderID + "|" + match.ModelID + "|" + match.ModelName
}

func TestExtractDevMatchesRanking(t *testing.T) {
	matches := extractDevMatches([]byte(devCatalogFixture), "gpt-test")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	// Canonical provider sorts ahead of the openrouter mirror.
	if got := flatten(matches[0]); got != "openai|gpt-test|GPT Test" {
		t.Fatalf("best match = %q, want canonical provider first", got)
	}

	// A provider hint overrides canonical ordering.
	matches = extractDevMatches([]byte(devCatalogFixture), "gpt-test")
	rankDevMatches(matches, "openrouter")
	if got := flatten(matches[0]); got != "openrouter|gpt-test|GPT Test (mirrored)" {
		t.Fatalf("hinted best = %q, want mirror first", got)
	}
}

func TestExtractDevMatchFields(t *testing.T) {
	matches := extractDevMatches([]byte(devCatalogFixture), "gpt-test")
	best := matches[0]
	if best.ContextWindow != 400000 || best.MaxOutputTokens != 32000 {
		t.Fatalf("limits not extracted: %+v", best)
	}
	if !best.Reasoning || strings.Join(best.ReasoningEfforts, ",") != "high,max" {
		t.Fatalf("reasoning not extracted: reasoning=%v efforts=%v", best.Reasoning, best.ReasoningEfforts)
	}
	if best.ProviderEnvVar != "OPENAI_API_KEY" {
		t.Fatalf("provider metadata wrong: %+v", best)
	}
}

func TestExtractDevMatchToleratesMissingData(t *testing.T) {
	matches := extractDevMatches([]byte(devCatalogFixture), "tiny-local")
	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.ContextWindow != 0 || m.MaxOutputTokens != 0 || m.ReasoningEfforts != nil {
		t.Fatalf("absent fields should stay zero-valued: %+v", m)
	}
	if extractDevMatches([]byte(devCatalogFixture), "does-not-exist") != nil {
		t.Fatal("unknown id should return no matches")
	}
	if extractDevMatches([]byte(`{`), "anything") != nil {
		t.Fatal("broken catalog should return no matches")
	}
}
