package grokconfig

import (
	"fmt"
	"strings"
)

func splitPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, true
	default:
		return nil, false
	}
}

func GetPath(root map[string]any, path string) (any, bool) {
	if root == nil {
		return nil, false
	}
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, false
	}
	cur := any(root)
	for _, p := range parts {
		m, ok := asMap(cur)
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func SetPath(root map[string]any, path string, value any) error {
	if root == nil {
		return fmt.Errorf("nil document")
	}
	parts := splitPath(path)
	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}
	cur := root
	for i, p := range parts[:len(parts)-1] {
		next, ok := cur[p]
		if !ok || next == nil {
			child := map[string]any{}
			cur[p] = child
			cur = child
			continue
		}
		child, ok := asMap(next)
		if !ok {
			return fmt.Errorf("cannot set %s: %s is not a table", path, strings.Join(parts[:i+1], "."))
		}
		if _, isOrig := next.(map[string]any); !isOrig {
			cur[p] = child
		}
		cur = child
	}
	cur[parts[len(parts)-1]] = value
	return nil
}

func DeletePath(root map[string]any, path string) {
	if root == nil {
		return
	}
	parts := splitPath(path)
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		delete(root, parts[0])
		return
	}
	parentAny, ok := GetPath(root, strings.Join(parts[:len(parts)-1], "."))
	if !ok {
		return
	}
	parent, ok := asMap(parentAny)
	if !ok {
		return
	}
	delete(parent, parts[len(parts)-1])
	pruneEmpty(root, parts[:len(parts)-1])
}

func pruneEmpty(root map[string]any, parts []string) {
	for len(parts) > 0 {
		parentParts := parts[:len(parts)-1]
		var parent map[string]any
		if len(parentParts) == 0 {
			parent = root
		} else {
			v, ok := GetPath(root, strings.Join(parentParts, "."))
			if !ok {
				return
			}
			parent, ok = asMap(v)
			if !ok {
				return
			}
		}
		key := parts[len(parts)-1]
		child, ok := parent[key]
		if !ok {
			return
		}
		m, ok := asMap(child)
		if !ok || len(m) > 0 {
			return
		}
		delete(parent, key)
		parts = parentParts
	}
}
