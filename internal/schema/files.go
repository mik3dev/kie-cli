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
