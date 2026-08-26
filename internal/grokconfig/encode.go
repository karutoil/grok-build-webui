package grokconfig

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var inlineFieldNames = map[string]bool{
	"extra_headers":    true,
	"query_params":     true,
	"env_http_headers": true,
	"env":              true,
	"headers":          true,
	"tool_timeouts":    true,
	"set":              true,
}

var collectionPrefixes = map[string]bool{
	"model":           true,
	"model_providers": true,
	"mcp_servers":     true,
}

func EncodeTOML(root map[string]any) ([]byte, error) {
	if root == nil {
		root = map[string]any{}
	}
	var buf bytes.Buffer
	buf.WriteString("# ~/.grok/config.toml — written by Grok Build WebUI\n")
	buf.WriteString("# Saving from the form rewrites this file. A .bak copy is kept beside it.\n\n")
	if err := writeTable(&buf, nil, root, true); err != nil {
		return nil, err
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	out = append(out, '\n')
	return out, nil
}

func writeTable(buf *bytes.Buffer, path []string, m map[string]any, isRoot bool) error {
	if m == nil {
		return nil
	}
	keys := orderedTableKeys(path, m)
	var scalars []string
	var nested []string
	var collections []string
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		child, isMap := asMap(v)
		if !isMap {
			scalars = append(scalars, k)
			continue
		}
		if collectionPrefixes[k] && isRoot && isCollectionTable(child) {
			collections = append(collections, k)
			continue
		}
		if shouldInline(k, child) {
			scalars = append(scalars, k)
			continue
		}
		nested = append(nested, k)
	}

	if !isRoot && (len(scalars) > 0 || (len(nested) == 0 && len(collections) == 0)) {
		if buf.Len() > 0 && !bytes.HasSuffix(buf.Bytes(), []byte("\n\n")) {
			if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
				buf.WriteByte('\n')
			}
			buf.WriteByte('\n')
		}
		buf.WriteByte('[')
		buf.WriteString(formatTablePath(path))
		buf.WriteString("]\n")
		for _, k := range scalars {
			if err := writeKeyValue(buf, k, m[k]); err != nil {
				return err
			}
		}
	} else if isRoot {
		for _, k := range scalars {
			if err := writeKeyValue(buf, k, m[k]); err != nil {
				return err
			}
		}
		if len(scalars) > 0 && (len(nested) > 0 || len(collections) > 0) {
			buf.WriteByte('\n')
		}
	} else if len(scalars) == 0 && len(nested) > 0 {
		// header omitted; nested tables carry the path
	}

	for _, k := range nested {
		child, _ := asMap(m[k])
		if err := writeTable(buf, append(append([]string{}, path...), k), child, false); err != nil {
			return err
		}
	}
	for _, k := range collections {
		child, _ := asMap(m[k])
		ids := sortedKeys(child)
		for _, id := range ids {
			item, ok := asMap(child[id])
			if !ok {
				// unusual non-table; write as nested key
				if err := writeTable(buf, append([]string{k}, id), map[string]any{"value": child[id]}, false); err != nil {
					return err
				}
				continue
			}
			if err := writeTable(buf, []string{k, id}, item, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func isCollectionTable(m map[string]any) bool {
	if len(m) == 0 {
		return true
	}
	for _, v := range m {
		if _, ok := asMap(v); !ok {
			return false
		}
	}
	return true
}

func shouldInline(key string, m map[string]any) bool {
	if !allScalarLike(m) {
		return false
	}
	if inlineFieldNames[key] {
		return true
	}
	// Small maps of scalars on a collection item look like extra_headers.
	return false
}

func allScalarLike(m map[string]any) bool {
	for _, v := range m {
		if v == nil {
			continue
		}
		if _, ok := asMap(v); ok {
			return false
		}
		if s, ok := asSlice(v); ok {
			for _, item := range s {
				if _, isMap := asMap(item); isMap {
					return false
				}
			}
		}
	}
	return true
}

func asSlice(v any) ([]any, bool) {
	switch s := v.(type) {
	case []any:
		return s, true
	case []string:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out, true
	case []int:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out, true
	case []int64:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out, true
	default:
		return nil, false
	}
}

func writeKeyValue(buf *bytes.Buffer, key string, v any) error {
	buf.WriteString(formatKey(key))
	buf.WriteString(" = ")
	if err := writeValue(buf, v); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return nil
}

func writeValue(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("\"\"")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		buf.WriteString(quoteTOML(x))
	case int:
		buf.WriteString(strconv.Itoa(x))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(x, 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(x, 10))
	case float32:
		writeFloat(buf, float64(x))
	case float64:
		if !math.IsNaN(x) && !math.IsInf(x, 0) && x == math.Trunc(x) && math.Abs(x) < 1e15 {
			buf.WriteString(strconv.FormatInt(int64(x), 10))
		} else {
			writeFloat(buf, x)
		}
	default:
		if m, ok := asMap(v); ok {
			return writeInlineTable(buf, m)
		}
		if s, ok := asSlice(v); ok {
			return writeArray(buf, s)
		}
		return fmt.Errorf("unsupported TOML value %T", v)
	}
	return nil
}

func writeFloat(buf *bytes.Buffer, x float64) {
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	buf.WriteString(s)
}

func writeArray(buf *bytes.Buffer, s []any) error {
	buf.WriteByte('[')
	for i, item := range s {
		if i > 0 {
			buf.WriteString(", ")
		}
		if err := writeValue(buf, item); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeInlineTable(buf *bytes.Buffer, m map[string]any) error {
	keys := sortedKeys(m)
	buf.WriteString("{ ")
	first := true
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		if !first {
			buf.WriteString(", ")
		}
		first = false
		buf.WriteString(formatKey(k))
		buf.WriteString(" = ")
		if err := writeValue(buf, v); err != nil {
			return err
		}
	}
	if first {
		buf.WriteString("}")
		return nil
	}
	buf.WriteString(" }")
	return nil
}

func orderedTableKeys(path []string, m map[string]any) []string {
	preferred := preferredKeys(path)
	seen := map[string]bool{}
	var out []string
	for _, k := range preferred {
		if _, ok := m[k]; ok {
			out = append(out, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func preferredKeys(path []string) []string {
	if len(path) == 0 {
		return []string{
			"cli", "auth", "grok_com_config", "endpoints",
			"models", "model", "model_providers",
			"features", "session", "tools", "toolset",
			"ui", "sandbox", "permission", "agent",
			"mcp", "mcp_servers",
			"memory", "subagents", "workflows", "skills", "plugins",
			"telemetry", "hints", "marketplace", "compat",
			"shell_environment_policy",
		}
	}
	joined := strings.Join(path, ".")
	switch {
	case joined == "model" || strings.HasPrefix(joined, "model."):
		return []string{"model", "name", "description", "base_url", "model_provider", "api_backend", "api_key", "env_key", "auth_provider", "temperature", "top_p", "max_completion_tokens", "context_window", "max_retries", "inference_idle_timeout_secs", "stream_tool_calls", "supports_reasoning_effort", "reasoning_efforts", "supports_backend_search", "extra_headers", "query_params", "env_http_headers"}
	case joined == "model_providers" || strings.HasPrefix(joined, "model_providers."):
		return []string{"name", "api_base_url", "base_url", "api_backend", "api_key", "env_key", "auth_provider", "extra_headers", "query_params", "env_http_headers"}
	case joined == "mcp_servers" || strings.HasPrefix(joined, "mcp_servers."):
		return []string{"command", "args", "url", "env", "headers", "enabled", "startup_timeout_sec", "tool_timeout_sec", "tool_timeouts"}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatTablePath(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = formatKey(p)
	}
	return strings.Join(out, ".")
}

func formatKey(k string) string {
	if isBareKey(k) {
		return k
	}
	return quoteTOML(k)
}

func isBareKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func quoteTOML(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(`\u`)
				h := strconv.FormatInt(int64(r), 16)
				for len(h) < 4 {
					h = "0" + h
				}
				b.WriteString(h)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
