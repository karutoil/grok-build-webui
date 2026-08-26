package grokconfig

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func coerce(field Field, v any) (any, error) {
	if v == nil {
		return nil, fmt.Errorf("%s: value is null (use unset)", field.Key)
	}
	switch field.Type {
	case TypeBool:
		return coerceBool(field.Key, v)
	case TypeInt:
		n, err := coerceInt(field.Key, v)
		if err != nil {
			return nil, err
		}
		if err := checkRange(field, float64(n)); err != nil {
			return nil, err
		}
		return n, nil
	case TypeFloat:
		n, err := coerceFloat(field.Key, v)
		if err != nil {
			return nil, err
		}
		if err := checkRange(field, n); err != nil {
			return nil, err
		}
		return n, nil
	case TypeString:
		s, err := coerceString(field.Key, v)
		if err != nil {
			return nil, err
		}
		return s, nil
	case TypeEnum:
		s, err := coerceString(field.Key, v)
		if err != nil {
			return nil, err
		}
		if len(field.Options) > 0 {
			ok := false
			for _, o := range field.Options {
				if s == o {
					ok = true
					break
				}
			}
			if !ok {
				return nil, fmt.Errorf("%s: %q is not one of %s", field.Key, s, strings.Join(field.Options, ", "))
			}
		}
		return s, nil
	case TypeStringList:
		return coerceStringList(field.Key, v)
	case TypeMap:
		return coerceMap(field.Key, v, false)
	case TypeIntMap:
		return coerceMap(field.Key, v, true)
	default:
		return v, nil
	}
}

func checkRange(field Field, n float64) error {
	if field.Min != nil && n < *field.Min {
		return fmt.Errorf("%s: %v is below minimum %v", field.Key, n, *field.Min)
	}
	if field.Max != nil && n > *field.Max {
		return fmt.Errorf("%s: %v is above maximum %v", field.Key, n, *field.Max)
	}
	return nil
}

func coerceBool(key string, v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		}
	}
	return false, fmt.Errorf("%s: expected boolean, got %T", key, v)
}

func coerceInt(key string, v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case float64:
		if x != math.Trunc(x) {
			return 0, fmt.Errorf("%s: expected integer, got %v", key, x)
		}
		return int64(x), nil
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			f, ferr := x.Float64()
			if ferr != nil || f != math.Trunc(f) {
				return 0, fmt.Errorf("%s: expected integer", key)
			}
			return int64(f), nil
		}
		return n, nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: expected integer", key)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("%s: expected integer, got %T", key, v)
	}
}

func coerceFloat(key string, v any) (float64, error) {
	switch x := v.(type) {
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case float64:
		return x, nil
	case json.Number:
		return x.Float64()
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, fmt.Errorf("%s: expected number", key)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("%s: expected number, got %T", key, v)
	}
}

func coerceString(key string, v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	default:
		return "", fmt.Errorf("%s: expected string, got %T", key, v)
	}
}

func coerceStringList(key string, v any) ([]any, error) {
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return []any{}, nil
		}
		if strings.Contains(s, "\n") {
			var out []any
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					out = append(out, line)
				}
			}
			return out, nil
		}
		if strings.Contains(s, ",") {
			var out []any
			for _, part := range strings.Split(s, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					out = append(out, part)
				}
			}
			return out, nil
		}
		return []any{s}, nil
	}
	s, ok := asSlice(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected string or array of strings", key)
	}
	out := make([]any, 0, len(s))
	for i, item := range s {
		str, err := coerceString(fmt.Sprintf("%s[%d]", key, i), item)
		if err != nil {
			return nil, err
		}
		out = append(out, str)
	}
	return out, nil
}

func coerceMap(key string, v any, ints bool) (map[string]any, error) {
	if s, ok := v.(string); ok {
		m := map[string]any{}
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			k, val, ok := strings.Cut(line, "=")
			if !ok {
				k, val, ok = strings.Cut(line, ":")
			}
			if !ok {
				return nil, fmt.Errorf("%s: map lines must be key = value", key)
			}
			k = strings.TrimSpace(k)
			val = strings.TrimSpace(val)
			if ints {
				n, err := strconv.ParseInt(val, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("%s.%s: expected integer", key, k)
				}
				m[k] = n
			} else {
				m[k] = val
			}
		}
		return m, nil
	}
	m, ok := asMap(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected object / map", key)
	}
	out := map[string]any{}
	for k, item := range m {
		if ints {
			n, err := coerceInt(key+"."+k, item)
			if err != nil {
				return nil, err
			}
			out[k] = n
			continue
		}
		switch x := item.(type) {
		case string:
			out[k] = x
		case bool:
			out[k] = x
		case int, int64:
			n, _ := coerceInt(key+"."+k, x)
			out[k] = n
		case float64:
			if x == float64(int64(x)) {
				out[k] = int64(x)
			} else {
				out[k] = x
			}
		case nil:
			continue
		default:
			s, err := coerceString(key+"."+k, item)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: expected string, bool, or number", key, k)
			}
			out[k] = s
		}
	}
	return out, nil
}
