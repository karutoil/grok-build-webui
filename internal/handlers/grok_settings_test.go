package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grok-build-webui/internal/config"
	"grok-build-webui/internal/grokconfig"
)

func TestGrokSettingsGetAndPatch(t *testing.T) {
	dir := t.TempDir()
	src := `
[models]
default = "my-model"

[model.my-model]
model = "grok-4.6"
base_url = "https://example.com/v1"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &SettingsHandler{cfg: &config.Config{GrokHome: dir}}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/grok", nil)
	rr := httptest.NewRecorder()
	h.GetGrok(rr, req)
	if rr.Code != 200 {
		t.Fatalf("GET %d %s", rr.Code, rr.Body.String())
	}
	var view grokconfig.View
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Exists || view.Path != filepath.Join(dir, "config.toml") {
		t.Fatalf("view %#v", view)
	}
	var defaultSet bool
	for _, sec := range view.Sections {
		for _, f := range sec.Fields {
			if f.Key == "models.default" {
				defaultSet = f.Set
				if f.Value != "my-model" {
					t.Fatalf("default=%v", f.Value)
				}
			}
			if f.Key == "features.telemetry" && f.Set {
				t.Fatal("unset telemetry should not be set")
			}
		}
	}
	if !defaultSet {
		t.Fatal("models.default should be set")
	}
	found := false
	for _, col := range view.Collections {
		if col.ID != "models" {
			continue
		}
		if len(col.Items) != 1 || col.Items[0].ID != "my-model" {
			t.Fatalf("items=%+v", col.Items)
		}
		found = true
	}
	if !found {
		t.Fatal("custom model missing")
	}

	body, _ := json.Marshal(grokconfig.Patch{
		Set: map[string]any{"features.telemetry": false},
		Collections: map[string]grokconfig.CollectionPatch{
			"providers": {
				Items: map[string]grokconfig.ItemPatch{
					"openai": {Set: map[string]any{
						"api_base_url": "https://api.openai.com/v1",
						"env_key":      "OPENAI_API_KEY",
					}},
				},
			},
		},
		IfMatchMTime: view.MTime,
	})
	req = httptest.NewRequest(http.MethodPut, "/api/settings/grok", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.UpdateGrok(rr, req)
	if rr.Code != 200 {
		t.Fatalf("PUT %d %s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "telemetry = false") {
		t.Fatalf("missing telemetry:\n%s", s)
	}
	if !strings.Contains(s, "[model_providers.openai]") {
		t.Fatalf("missing provider:\n%s", s)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml.bak")); err != nil {
		t.Fatal("expected backup")
	}
}
