package grokconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func Load(path string) (map[string]any, string, time.Time, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, "", time.Time{}, false, nil
		}
		return nil, "", time.Time{}, false, err
	}
	info, _ := os.Stat(path)
	mtime := time.Time{}
	if info != nil {
		mtime = info.ModTime()
	}
	var raw map[string]any
	if err := toml.Unmarshal(b, &raw); err != nil {
		return nil, string(b), mtime, true, fmt.Errorf("parse config.toml: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	norm, _ := normalize(raw).(map[string]any)
	if norm == nil {
		norm = map[string]any{}
	}
	return norm, string(b), mtime, true, nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalize(val)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = val
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalize(val)
		}
		return out
	default:
		return v
	}
}

func Save(path string, root map[string]any) error {
	data, err := EncodeTOML(root)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func SaveRaw(path string, raw string) error {
	var check map[string]any
	if err := toml.Unmarshal([]byte(raw), &check); err != nil {
		return fmt.Errorf("invalid TOML: %w", err)
	}
	if !strings.HasSuffix(raw, "\n") {
		raw += "\n"
	}
	return writeAtomic(path, []byte(raw))
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if prev, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", prev, 0o600)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func Snapshot(path string, root map[string]any, raw string, mtime time.Time, exists bool) *View {
	view := &View{
		Path:        path,
		Exists:      exists,
		Raw:         raw,
		Groups:      Groups(),
		Sections:    nil,
		Collections: nil,
	}
	if !mtime.IsZero() {
		view.MTime = mtime.UTC().Format(time.RFC3339Nano)
	}
	for _, sec := range Sections() {
		sv := SectionView{
			ID:          sec.ID,
			Group:       sec.Group,
			Title:       sec.Title,
			Description: sec.Description,
		}
		for _, field := range sec.Fields {
			sv.Fields = append(sv.Fields, viewField(field, root))
		}
		view.Sections = append(view.Sections, sv)
	}
	for _, col := range Collections() {
		cv := CollectionView{
			ID:          col.ID,
			Group:       col.Group,
			Title:       col.Title,
			Description: col.Description,
			Prefix:      col.Prefix,
			KeyLabel:    col.KeyLabel,
			ItemFields:  col.Fields,
			Templates:   col.Templates,
			Items:       []ItemView{},
		}
		table, _ := asMap(root[col.Prefix])
		ids := make([]string, 0, len(table))
		for id := range table {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		schemaKeys := map[string]bool{}
		for _, field := range col.Fields {
			schemaKeys[field.Key] = true
		}
		for _, id := range ids {
			item, _ := asMap(table[id])
			if item == nil {
				item = map[string]any{}
			}
			iv := ItemView{ID: id}
			for _, field := range col.Fields {
				iv.Fields = append(iv.Fields, viewField(field, item))
			}
			for k := range item {
				if !schemaKeys[k] {
					iv.Extra = append(iv.Extra, k)
				}
			}
			sort.Strings(iv.Extra)
			cv.Items = append(cv.Items, iv)
		}
		view.Collections = append(view.Collections, cv)
	}
	return view
}

func viewField(field Field, root map[string]any) FieldView {
	fv := FieldView{Field: field}
	if v, ok := GetPath(root, field.Key); ok {
		fv.Set = true
		fv.Value = v
	} else {
		fv.Value = field.Default
	}
	return fv
}

func Apply(root map[string]any, patch Patch) error {
	if root == nil {
		return fmt.Errorf("nil document")
	}
	idx := fieldIndex()
	for _, key := range patch.Unset {
		if _, ok := idx[key]; !ok {
			return fmt.Errorf("unknown setting %q", key)
		}
		DeletePath(root, key)
	}
	for key, val := range patch.Set {
		field, ok := idx[key]
		if !ok {
			return fmt.Errorf("unknown setting %q", key)
		}
		cv, err := coerce(field, val)
		if err != nil {
			return err
		}
		if err := SetPath(root, key, cv); err != nil {
			return err
		}
	}
	for colID, cp := range patch.Collections {
		col, ok := collectionByID(colID)
		if !ok {
			return fmt.Errorf("unknown collection %q", colID)
		}
		table, _ := asMap(root[col.Prefix])
		if table == nil {
			table = map[string]any{}
			root[col.Prefix] = table
		}
		for oldID, newID := range cp.Rename {
			if err := validateID(col, newID); err != nil {
				return err
			}
			item, ok := table[oldID]
			if !ok {
				return fmt.Errorf("%s %q not found", col.KeyLabel, oldID)
			}
			if _, exists := table[newID]; exists && oldID != newID {
				return fmt.Errorf("%s %q already exists", col.KeyLabel, newID)
			}
			delete(table, oldID)
			table[newID] = item
		}
		for _, id := range cp.Delete {
			delete(table, id)
		}
		for id, ip := range cp.Items {
			if err := validateID(col, id); err != nil {
				return err
			}
			item, _ := asMap(table[id])
			if item == nil {
				item = map[string]any{}
			}
			for _, key := range ip.Unset {
				if _, ok := collectionField(col, key); !ok {
					return fmt.Errorf("unknown %s field %q", col.ID, key)
				}
				delete(item, key)
			}
			for key, val := range ip.Set {
				field, ok := collectionField(col, key)
				if !ok {
					return fmt.Errorf("unknown %s field %q", col.ID, key)
				}
				cv, err := coerce(field, val)
				if err != nil {
					return err
				}
				item[key] = cv
			}
			table[id] = item
		}
		if len(table) == 0 {
			delete(root, col.Prefix)
		} else {
			root[col.Prefix] = table
		}
	}
	return nil
}

func validateID(col Collection, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%s id is required", col.KeyLabel)
	}
	if len(id) > 80 {
		return fmt.Errorf("%s id is too long", col.KeyLabel)
	}
	if !idRe.MatchString(id) {
		return fmt.Errorf("%s id %q must start with a letter or number and contain only letters, numbers, dots, hyphens, and underscores", col.KeyLabel, id)
	}
	return nil
}

func Clone(root map[string]any) map[string]any {
	if root == nil {
		return map[string]any{}
	}
	out, _ := normalize(root).(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}
