package schema

// HasURIFormat reports whether the schema of property key declares
// "format": "uri" anywhere in its subtree (items, nested properties,
// oneOf/anyOf branches). Such a property holds file references.
func HasURIFormat(sch map[string]any, key string) bool {
	p, ok := PropSchema(sch, key)
	if !ok {
		return false
	}
	return containsURIFormat(p)
}

func containsURIFormat(node any) bool {
	switch t := node.(type) {
	case map[string]any:
		if f, _ := t["format"].(string); f == "uri" {
			return true
		}
		for _, v := range t {
			if containsURIFormat(v) {
				return true
			}
		}
	case []any:
		for _, v := range t {
			if containsURIFormat(v) {
				return true
			}
		}
	}
	return false
}

// FileCandidate reports whether property key is declared in sch (known) and
// whether its declared type can hold a file reference (ok): a string or an
// array of strings, and not an enum.
func FileCandidate(sch map[string]any, key string) (known, ok bool) {
	p, found := PropSchema(sch, key)
	if !found {
		return false, false
	}
	return true, stringLike(p)
}

func stringLike(node map[string]any) bool {
	if _, isEnum := node["enum"]; isEnum {
		return false
	}
	switch t := node["type"].(type) {
	case string:
		if t == "array" {
			items, _ := node["items"].(map[string]any)
			return items != nil && stringLike(items)
		}
		return t == "string"
	case []any:
		for _, v := range t {
			if v == "string" {
				return true
			}
		}
		return false
	}
	for _, bk := range branchKeys {
		branches, _ := node[bk].([]any)
		for _, b := range branches {
			if bm, isMap := b.(map[string]any); isMap && stringLike(bm) {
				return true
			}
		}
	}
	return node["format"] == "uri"
}
