package grokconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestPathGetSetDelete(t *testing.T) {
	root := map[string]any{}
	if err := SetPath(root, "models.default", "my-model"); err != nil {
		t.Fatal(err)
	}
	if err := SetPath(root, "features.telemetry", false); err != nil {
		t.Fatal(err)
	}
	v, ok := GetPath(root, "models.default")
	if !ok || v != "my-model" {
		t.Fatalf("got %v %v", v, ok)
	}
	DeletePath(root, "features.telemetry")
	if _, ok := GetPath(root, "features.telemetry"); ok {
		t.Fatal("expected unset")
	}
	if _, ok := root["features"]; ok {
		t.Fatal("empty table should be pruned")
	}
}

func TestEncodeModelsAndProviders(t *testing.T) {
	root := map[string]any{
		"cli": map[string]any{"auto_update": false},
		"models": map[string]any{
			"default":                  "my-model",
			"web_search":               "grok-4.6",
			"default_reasoning_effort": "xhigh",
		},
		"model": map[string]any{
			"my-model": map[string]any{
				"model":                     "grok-4.6",
				"base_url":                  "https://ckff.dev/v1",
				"name":                      "grok-4.6",
				"api_key":                   "sk-test",
				"api_backend":               "chat_completions",
				"context_window":            int64(500000),
				"max_completion_tokens":     int64(128000),
				"supports_reasoning_effort": true,
			},
			"musk": map[string]any{
				"model":       "grok-4.6",
				"base_url":    "https://api.muskapi.cc",
				"name":        "grok-4.6-musk",
				"api_backend": "responses",
			},
		},
		"features": map[string]any{"telemetry": false, "feedback": false},
		"ui": map[string]any{
			"yolo":            true,
			"permission_mode": "always-approve",
			"compact_mode":    true,
		},
	}
	data, err := EncodeTOML(root)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"[cli]",
		"auto_update = false",
		"[models]",
		`default = "my-model"`,
		"[model.my-model]",
		`api_key = "sk-test"`,
		"[model.musk]",
		`api_backend = "responses"`,
		"[features]",
		"[ui]",
		`permission_mode = "always-approve"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("encoded TOML missing %q\n%s", want, s)
		}
	}
	var round map[string]any
	if err := toml.Unmarshal(data, &round); err != nil {
		t.Fatalf("round-trip parse: %v\n%s", err, s)
	}
	models, _ := asMap(round["model"])
	if _, ok := models["my-model"]; !ok {
		t.Fatalf("missing my-model after round-trip: %#v", round["model"])
	}
}

func TestEncodeNestedUITables(t *testing.T) {
	root := map[string]any{
		"ui": map[string]any{
			"compact_mode": true,
			"notifications": map[string]any{
				"method": "auto",
				"title":  map[string]any{"enabled": true},
			},
		},
		"subagents": map[string]any{
			"enabled": true,
			"toggle":  map[string]any{"explore": true, "plan": false},
		},
	}
	data, err := EncodeTOML(root)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"[ui]",
		"compact_mode = true",
		"[ui.notifications]",
		`method = "auto"`,
		"[ui.notifications.title]",
		"enabled = true",
		"[subagents]",
		"[subagents.toggle]",
		"explore = true",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q\n%s", want, s)
		}
	}
}

func TestEncodeQuotedModelID(t *testing.T) {
	root := map[string]any{
		"model": map[string]any{
			"grok-4.6": map[string]any{"model": "grok-4.6"},
		},
	}
	data, err := EncodeTOML(root)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `[model."grok-4.6"]`) {
		t.Fatalf("expected quoted table id, got:\n%s", s)
	}
	var round map[string]any
	if err := toml.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	models, _ := asMap(round["model"])
	if _, ok := models["grok-4.6"]; !ok {
		t.Fatalf("quoted id did not round-trip: %#v", round["model"])
	}
}

func TestApplySetUnsetAndCollection(t *testing.T) {
	root := map[string]any{}
	err := Apply(root, Patch{
		Set: map[string]any{
			"models.default":     "my-model",
			"features.telemetry": false,
		},
		Collections: map[string]CollectionPatch{
			"models": {
				Items: map[string]ItemPatch{
					"my-model": {Set: map[string]any{
						"model":    "grok-4.6",
						"base_url": "https://example.com/v1",
						"api_key":  "sk-1",
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := GetPath(root, "models.default"); v != "my-model" {
		t.Fatalf("default=%v", v)
	}
	item, _ := asMap(asMapMust(t, root["model"])["my-model"])
	if item["api_key"] != "sk-1" {
		t.Fatalf("api_key=%v", item["api_key"])
	}
	if err := Apply(root, Patch{Unset: []string{"features.telemetry"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetPath(root, "features.telemetry"); ok {
		t.Fatal("telemetry should be unset")
	}
	if err := Apply(root, Patch{Collections: map[string]CollectionPatch{
		"models": {Delete: []string{"my-model"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["model"]; ok {
		t.Fatalf("empty model table should be removed: %#v", root["model"])
	}
}

func TestApplyRejectsUnknown(t *testing.T) {
	root := map[string]any{}
	if err := Apply(root, Patch{Set: map[string]any{"not.a.key": 1}}); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestSnapshotMarksSetAndUnset(t *testing.T) {
	root := map[string]any{
		"models": map[string]any{"default": "my-model"},
		"model": map[string]any{
			"my-model": map[string]any{"model": "grok-4.6"},
		},
	}
	view := Snapshot("/tmp/config.toml", root, "", time.Time{}, true)
	var found bool
	for _, sec := range view.Sections {
		for _, field := range sec.Fields {
			if field.Key == "models.default" {
				found = true
				if !field.Set || field.Value != "my-model" {
					t.Fatalf("default view: %+v", field)
				}
			}
			if field.Key == "features.telemetry" {
				if field.Set {
					t.Fatal("unset telemetry should not be marked set")
				}
				if field.Value != false {
					t.Fatalf("default telemetry want false got %v", field.Value)
				}
			}
		}
	}
	if !found {
		t.Fatal("models.default missing from snapshot")
	}
	if len(view.Collections) == 0 || len(view.Collections[0].Items) != 1 {
		t.Fatalf("expected one model item, got %+v", view.Collections)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	root := map[string]any{
		"models": map[string]any{"default": "x"},
		"model": map[string]any{
			"x": map[string]any{"model": "grok-4.6", "extra_headers": map[string]any{"X-A": "b"}},
		},
	}
	if err := Save(path, root); err != nil {
		t.Fatal(err)
	}
	got, _, _, exists, err := Load(path)
	if err != nil || !exists {
		t.Fatalf("load: exists=%v err=%v", exists, err)
	}
	if v, _ := GetPath(got, "models.default"); v != "x" {
		t.Fatalf("default=%v", v)
	}
	item, _ := asMap(asMapMust(t, got["model"])["x"])
	hdrs, _ := asMap(item["extra_headers"])
	if hdrs["X-A"] != "b" {
		t.Fatalf("headers=%v", hdrs)
	}
	if _, err := os.Stat(path + ".bak"); err == nil {
		t.Fatal("bak should not exist on first write")
	}
	if err := Save(path, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal("expected bak after overwrite")
	}
}

func TestEncodeInlineHeaders(t *testing.T) {
	root := map[string]any{
		"model": map[string]any{
			"x": map[string]any{
				"model": "m",
				"extra_headers": map[string]any{
					"X-Request-Tags":    "team=example",
					"anthropic-version": "2023-06-01",
				},
			},
		},
		"mcp_servers": map[string]any{
			"github": map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-github"},
				"env":     map[string]any{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_x"},
				"enabled": true,
			},
		},
	}
	data, err := EncodeTOML(root)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "extra_headers = {") {
		t.Fatalf("expected inline extra_headers:\n%s", s)
	}
	if !strings.Contains(s, "env = {") {
		t.Fatalf("expected inline env:\n%s", s)
	}
	var round map[string]any
	if err := toml.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
}

func TestCoerceEnumAndMap(t *testing.T) {
	field := Field{Key: "ui.permission_mode", Type: TypeEnum, Options: []string{"ask", "auto", "always-approve"}}
	v, err := coerce(field, "always-approve")
	if err != nil || v != "always-approve" {
		t.Fatalf("%v %v", v, err)
	}
	if _, err := coerce(field, "nope"); err == nil {
		t.Fatal("expected enum error")
	}
	mf := Field{Key: "models.extra_headers", Type: TypeMap}
	m, err := coerce(mf, "X-A = one\nX-B=two")
	if err != nil {
		t.Fatal(err)
	}
	mm := m.(map[string]any)
	if mm["X-A"] != "one" || mm["X-B"] != "two" {
		t.Fatalf("%v", mm)
	}
}

func asMapMust(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := asMap(v)
	if !ok {
		t.Fatalf("not a map: %T", v)
	}
	return m
}

func TestPatchEmpty(t *testing.T) {
	if !(Patch{}).Empty() {
		t.Fatal("zero patch should be empty")
	}
	raw := "x = 1"
	if (Patch{Raw: &raw}).Empty() {
		t.Fatal("raw patch is not empty")
	}
	if (Patch{Set: map[string]any{"models.default": "x"}}).Empty() {
		t.Fatal("set patch is not empty")
	}
}

func TestSnapshotMTimeEmpty(t *testing.T) {
	view := Snapshot("/x", map[string]any{}, "", time.Time{}, false)
	if view.Exists {
		t.Fatal("exists")
	}
	if view.MTime != "" {
		t.Fatalf("mtime=%q", view.MTime)
	}
}
