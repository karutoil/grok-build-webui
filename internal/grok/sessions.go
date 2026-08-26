package grok

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Conversation struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	CWD          string    `json:"cwd"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	NumMessages  int       `json:"num_messages"`
	Sandbox      string    `json:"sandbox"`
	AgentName    string    `json:"agent_name,omitempty"`
}

type summaryFile struct {
	Info struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"info"`
	SessionSummary  string `json:"session_summary"`
	GeneratedTitle  string `json:"generated_title"`
	CurrentModelID  string `json:"current_model_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	LastActiveAt    string `json:"last_active_at"`
	NumMessages     int    `json:"num_messages"`
	SandboxProfile  string `json:"sandbox_profile"`
	AgentName       string `json:"agent_name"`
}

func Home(override string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("GROK_HOME"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".grok")
}

func encodeCWD(p string) string {
	return strings.ReplaceAll(url.QueryEscape(p), "+", "%20")
}

func ListConversations(grokHome, cwd string) []Conversation {
	root := filepath.Join(Home(grokHome), "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	cwd = filepath.Clean(cwd)
	var out []Conversation
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		group := filepath.Join(root, e.Name())
		groupCWD := readGroupCWD(group, e.Name())
		if cwd != "" && groupCWD != "" && filepath.Clean(groupCWD) != cwd {
			continue
		}
		kids, err := os.ReadDir(group)
		if err != nil {
			continue
		}
		for _, k := range kids {
			if !k.IsDir() {
				continue
			}
			c, ok := readSummary(filepath.Join(group, k.Name(), "summary.json"))
			if !ok {
				continue
			}
			if cwd != "" && c.CWD != "" && filepath.Clean(c.CWD) != cwd {
				continue
			}
			if c.CWD == "" {
				c.CWD = groupCWD
			}
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func Latest(grokHome, cwd string) (Conversation, bool) {
	list := ListConversations(grokHome, cwd)
	if len(list) == 0 {
		return Conversation{}, false
	}
	return list[0], true
}

func Find(grokHome, cwd, id string) (Conversation, bool) {
	for _, c := range ListConversations(grokHome, cwd) {
		if c.ID == id {
			return c, true
		}
	}
	return Conversation{}, false
}

func NewestSince(grokHome, cwd string, since time.Time, ignore map[string]bool) (Conversation, bool) {
	for _, c := range ListConversations(grokHome, cwd) {
		if ignore[c.ID] {
			continue
		}
		if c.CreatedAt.After(since.Add(-2 * time.Second)) {
			return c, true
		}
	}
	return Conversation{}, false
}

func readGroupCWD(group, name string) string {
	if b, err := os.ReadFile(filepath.Join(group, ".cwd")); err == nil {
		return filepath.Clean(strings.TrimSpace(string(b)))
	}
	if unesc, err := url.QueryUnescape(name); err == nil && strings.HasPrefix(unesc, "/") {
		return filepath.Clean(unesc)
	}
	return ""
}

func readSummary(path string) (Conversation, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Conversation{}, false
	}
	var s summaryFile
	if err := json.Unmarshal(b, &s); err != nil {
		return Conversation{}, false
	}
	id := s.Info.ID
	if id == "" {
		id = filepath.Base(filepath.Dir(path))
	}
	title := strings.TrimSpace(s.GeneratedTitle)
	if title == "" {
		title = strings.TrimSpace(s.SessionSummary)
	}
	if title == "" {
		title = "untitled"
	}
	c := Conversation{
		ID:          id,
		Title:       title,
		Model:       s.CurrentModelID,
		CWD:         s.Info.CWD,
		NumMessages: s.NumMessages,
		Sandbox:     s.SandboxProfile,
		AgentName:   s.AgentName,
		CreatedAt:   parseTime(s.CreatedAt),
		UpdatedAt:   parseTime(firstNonEmpty(s.LastActiveAt, s.UpdatedAt, s.CreatedAt)),
	}
	return c, true
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func EncodeCWD(p string) string { return encodeCWD(p) }
