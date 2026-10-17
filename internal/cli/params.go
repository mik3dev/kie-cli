package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mik3dev/kie-cli/internal/output"
)

// ParseParams turns key=value pairs into an input map.
// ponytail: literal-based typing; stage 2 coerces by schema type.
func ParseParams(pairs []string, readFile func(string) ([]byte, error)) (map[string]any, error) {
	out := make(map[string]any, len(pairs))
	lists := map[string]bool{} // keys whose value is an array built from repeats
	for _, p := range pairs {
		k, raw, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, &output.Error{Code: output.CodeUsage, Message: fmt.Sprintf("invalid -p %q: want key=value", p)}
		}
		v, err := parseValue(raw, readFile)
		if err != nil {
			return nil, err
		}
		prev, seen := out[k]
		switch {
		case !seen:
			out[k] = v
		case lists[k]:
			out[k] = append(append([]any{}, prev.([]any)...), v)
		default:
			out[k] = []any{prev, v}
			lists[k] = true
		}
	}
	return out, nil
}

func parseValue(raw string, readFile func(string) ([]byte, error)) (any, error) {
	if rest, ok := strings.CutPrefix(raw, "@@"); ok {
		return "@" + rest, nil
	}
	if path, ok := strings.CutPrefix(raw, "@"); ok {
		b, err := readFile(path)
		if err != nil {
			return nil, &output.Error{Code: output.CodeUsage, Message: fmt.Sprintf("cannot read %s: %v", path, err)}
		}
		return string(b), nil
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var v any
		if json.Unmarshal([]byte(raw), &v) == nil {
			return v, nil
		}
		return raw, nil
	}
	var v any
	if raw == strings.TrimSpace(raw) && json.Unmarshal([]byte(raw), &v) == nil {
		switch v.(type) {
		case bool, float64:
			return v, nil
		}
	}
	return raw, nil
}

// MergeInput returns a new map where over wins key by key.
func MergeInput(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}
