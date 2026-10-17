package schema

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

var branchKeys = []string{"oneOf", "anyOf", "allOf"}

// coerceOrder is the order in which string values are tried against a
// property's declared types.
var coerceOrder = []string{"integer", "number", "boolean", "array", "object", "string"}

// PropSchema finds the schema of property key in sch.properties and, failing
// that, in the oneOf/anyOf/allOf branches in order. First match wins.
func PropSchema(sch map[string]any, key string) (map[string]any, bool) {
	if sch == nil {
		return nil, false
	}
	if props, ok := sch["properties"].(map[string]any); ok {
		if p, ok := props[key].(map[string]any); ok {
			return p, true
		}
	}
	for _, bk := range branchKeys {
		branches, _ := sch[bk].([]any)
		for _, b := range branches {
			if bm, ok := b.(map[string]any); ok {
				if p, ok := PropSchema(bm, key); ok {
					return p, true
				}
			}
		}
	}
	return nil, false
}

// Coerce returns a new input where each value is converted to the type its
// schema property declares. It never fails: a value that cannot be converted
// is kept as is so validation can report it. Keys absent from the schema (or
// a nil schema) get literal typing: JSON for {/[, bool, number, else string.
func Coerce(sch map[string]any, input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		if p, ok := PropSchema(sch, k); ok {
			out[k] = coerceValue(p, v)
		} else {
			out[k] = guessLiteral(v)
		}
	}
	return out
}

// coerceNested converts the known properties of a nested object; unknown
// keys are left as they are.
func coerceNested(sch map[string]any, in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if p, ok := PropSchema(sch, k); ok {
			out[k] = coerceValue(p, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// declaredTypes returns the property's types: its own `type`, else the union
// of its oneOf/anyOf branch types.
func declaredTypes(p map[string]any) []string {
	if ts := typeList(p["type"]); len(ts) > 0 {
		return ts
	}
	var out []string
	seen := map[string]bool{}
	for _, bk := range []string{"oneOf", "anyOf"} {
		branches, _ := p[bk].([]any)
		for _, b := range branches {
			bm, _ := b.(map[string]any)
			for _, t := range declaredTypes(bm) {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
	}
	return out
}

func typeList(t any) []string {
	switch tv := t.(type) {
	case string:
		return []string{tv}
	case []any:
		out := make([]string, 0, len(tv))
		for _, e := range tv {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func has(types []string, t string) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

func coerceValue(p map[string]any, v any) any {
	types := declaredTypes(p)
	if len(types) == 0 || v == nil {
		return v
	}
	switch tv := v.(type) {
	case []any:
		if has(types, "array") {
			return coerceItems(p, tv)
		}
		return v
	case map[string]any:
		if has(types, "object") {
			return coerceNested(p, tv)
		}
		return v
	}
	if matches(types, v) {
		return v
	}
	if s, ok := v.(string); ok {
		return fromString(p, types, s)
	}
	if has(types, "string") {
		if s, ok := scalarString(v); ok {
			return s
		}
	}
	if has(types, "array") {
		return coerceItems(p, []any{v})
	}
	return v
}

// matches reports whether a scalar value already satisfies one of the types.
func matches(types []string, v any) bool {
	switch tv := v.(type) {
	case string:
		return has(types, "string")
	case bool:
		return has(types, "boolean")
	case float64:
		return has(types, "number") || (has(types, "integer") && tv == math.Trunc(tv))
	case int, int64:
		return has(types, "number") || has(types, "integer")
	case json.Number:
		if has(types, "number") {
			return true
		}
		_, err := tv.Int64()
		return has(types, "integer") && err == nil
	}
	return false
}

func scalarString(v any) (string, bool) {
	switch tv := v.(type) {
	case float64:
		return strconv.FormatFloat(tv, 'f', -1, 64), true
	case int:
		return strconv.Itoa(tv), true
	case int64:
		return strconv.FormatInt(tv, 10), true
	case json.Number:
		return tv.String(), true
	case bool:
		return strconv.FormatBool(tv), true
	}
	return "", false
}

func fromString(p map[string]any, types []string, s string) any {
	t := strings.TrimSpace(s)
	for _, typ := range coerceOrder {
		if !has(types, typ) {
			continue
		}
		switch typ {
		case "integer":
			if n, err := strconv.ParseInt(t, 10, 64); err == nil {
				return json.Number(strconv.FormatInt(n, 10))
			}
		case "number":
			if f, err := strconv.ParseFloat(t, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
				return json.Number(strconv.FormatFloat(f, 'f', -1, 64))
			}
		case "boolean":
			switch strings.ToLower(t) {
			case "true":
				return true
			case "false":
				return false
			}
		case "array":
			return coerceItems(p, stringToArray(s))
		case "object":
			if strings.HasPrefix(t, "{") {
				var m map[string]any
				if json.Unmarshal([]byte(t), &m) == nil {
					return coerceNested(p, m)
				}
			}
		case "string":
			return s
		}
	}
	return s
}

// stringToArray reads a JSON array literal, or wraps any other string.
func stringToArray(s string) []any {
	if t := strings.TrimSpace(s); strings.HasPrefix(t, "[") {
		var a []any
		if json.Unmarshal([]byte(t), &a) == nil {
			return a
		}
	}
	return []any{s}
}

func coerceItems(p map[string]any, items []any) []any {
	out := make([]any, len(items))
	ip, _ := p["items"].(map[string]any)
	for i, it := range items {
		if ip == nil {
			out[i] = it
		} else {
			out[i] = coerceValue(ip, it)
		}
	}
	return out
}

// guessLiteral types a string with no schema: JSON for {/[, bool, number.
func guessLiteral(v any) any {
	raw, ok := v.(string)
	if !ok {
		return v
	}
	var out any
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		if json.Unmarshal([]byte(raw), &out) == nil {
			return out
		}
		return raw
	}
	if raw == strings.TrimSpace(raw) && json.Unmarshal([]byte(raw), &out) == nil {
		switch out.(type) {
		case bool, float64:
			return out
		}
	}
	return raw
}
