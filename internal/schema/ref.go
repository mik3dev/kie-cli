package schema

import (
	"fmt"
	"net/url"
	"strings"
)

// Resolve returns a deep copy of node with every local `$ref` expanded.
// Segments are percent-decoded (never trimmed). A ref that is already being
// expanded (cycle) is left as {"$ref": ...}; external refs are kept as is.
// Neither doc nor node is mutated.
func Resolve(doc map[string]any, node any) (any, error) {
	return resolve(doc, node, map[string]bool{})
}

func resolve(doc map[string]any, node any, active map[string]bool) (any, error) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok {
			return resolveRef(doc, n, ref, active)
		}
		out := make(map[string]any, len(n))
		for k, v := range n {
			r, err := resolve(doc, v, active)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(n))
		for i, v := range n {
			r, err := resolve(doc, v, active)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	default:
		return node, nil
	}
}

func resolveRef(doc, n map[string]any, ref string, active map[string]bool) (any, error) {
	if !strings.HasPrefix(ref, "#/") || active[ref] {
		return map[string]any{"$ref": ref}, nil
	}
	target, err := lookup(doc, ref)
	if err != nil {
		return nil, err
	}
	active[ref] = true
	defer delete(active, ref)
	res, err := resolve(doc, target, active)
	if err != nil {
		return nil, err
	}
	resMap, isMap := res.(map[string]any)
	if !isMap || len(n) == 1 {
		return res, nil
	}
	// Sibling keys of $ref override the target's (e.g. a local description).
	out := make(map[string]any, len(resMap)+len(n))
	for k, v := range resMap {
		out[k] = v
	}
	for k, v := range n {
		if k == "$ref" {
			continue
		}
		r, err := resolve(doc, v, active)
		if err != nil {
			return nil, err
		}
		out[k] = r
	}
	return out, nil
}

func lookup(doc map[string]any, ref string) (any, error) {
	var cur any = doc
	for _, raw := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		seg, err := url.PathUnescape(raw)
		if err != nil {
			return nil, fmt.Errorf("bad $ref %q: %w", ref, err)
		}
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unresolvable $ref %q", ref)
		}
		if cur, ok = m[seg]; !ok {
			return nil, fmt.Errorf("unresolvable $ref %q", ref)
		}
	}
	return cur, nil
}
