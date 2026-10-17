package schema

import (
	"sort"
	"strings"
)

const (
	maxDescRunes  = 300
	maxNotesRunes = 600
)

// Field is one input key as an agent needs to know it.
type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Enum        []any    `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	MaxItems    *int     `json:"maxItems,omitempty"`
	Items       string   `json:"items,omitempty"`
	// ItemFields describes the element object of an array of objects, one level deep.
	ItemFields []Field `json:"itemFields,omitempty"`
}

// Variant is one alternative input shape of a oneOf/anyOf.
type Variant struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Fields      []Field `json:"fields"`
}

// Summary is the compact, agent-facing view of a model's input.
type Summary struct {
	Model    string    `json:"model"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Job      bool      `json:"job"`
	Null     bool      `json:"null,omitempty"`
	Fields   []Field   `json:"fields"`
	Variants []Variant `json:"variants,omitempty"`
	// Notes is the input object's own description (rules across fields).
	Notes string `json:"notes,omitempty"`
}

// branch is one flattened alternative: its properties and required names.
type branch struct {
	props       map[string]any
	req         map[string]bool
	title, desc string
}

// Summarize lists the input fields of d. A oneOf/anyOf with per-branch
// properties becomes Variants (Fields empty); allOf is merged.
func Summarize(d Doc) (Summary, error) {
	method, path, err := d.Endpoint()
	if err != nil {
		return Summary{}, err
	}
	s := Summary{Model: d.Model, Method: method, Path: path, Job: d.IsJob(), Null: d.Null(), Fields: []Field{}}
	if d.Null() {
		return s, nil
	}
	in, err := d.InputSchema()
	if err != nil {
		return Summary{}, err
	}
	if d, ok := in["description"].(string); ok {
		s.Notes = truncateTo(d, maxNotesRunes)
	}
	branches := expand(in)
	if len(branches) == 1 {
		s.Fields = fieldsOf(branches[0])
		return s, nil
	}
	for _, b := range branches {
		s.Variants = append(s.Variants, Variant{Title: b.title, Description: truncate(b.desc), Fields: fieldsOf(b)})
	}
	return s, nil
}

// expand flattens a schema node into one branch, or several when a oneOf/anyOf
// contributes properties. Branches that only add `required` constraints are
// not alternatives of the input shape and are ignored.
func expand(node map[string]any) []branch {
	base := branch{props: map[string]any{}, req: map[string]bool{}}
	mergeOwn(&base, node)
	if all, ok := node["allOf"].([]any); ok {
		for _, a := range all {
			if m, ok := a.(map[string]any); ok {
				mergeInto(&base, expand(m)[0])
			}
		}
	}
	var alts []branch
	for _, key := range []string{"oneOf", "anyOf"} {
		list, _ := node[key].([]any)
		for _, a := range list {
			m, ok := a.(map[string]any)
			if !ok {
				continue
			}
			alts = append(alts, expand(m)...)
		}
	}
	adds := false
	for _, a := range alts {
		if len(a.props) > 0 {
			adds = true
		}
	}
	if !adds {
		return []branch{base}
	}
	out := make([]branch, 0, len(alts))
	for _, a := range alts {
		b := branch{props: map[string]any{}, req: map[string]bool{}}
		mergeInto(&b, base)
		mergeInto(&b, a)
		b.title, b.desc = a.title, a.desc
		out = append(out, b)
	}
	return out
}

func mergeOwn(b *branch, node map[string]any) {
	if t, ok := node["title"].(string); ok {
		b.title = t
	}
	if t, ok := node["description"].(string); ok {
		b.desc = t
	}
	if props, ok := node["properties"].(map[string]any); ok {
		for k, v := range props {
			b.props[k] = v
		}
	}
	if req, ok := node["required"].([]any); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				b.req[name] = true
			}
		}
	}
}

func mergeInto(dst *branch, src branch) {
	for k, v := range src.props {
		dst.props[k] = v
	}
	for k := range src.req {
		dst.req[k] = true
	}
}

func fieldsOf(b branch) []Field { return buildFields(b, true) }

// flatFields summarizes item properties without descending further.
func flatFields(b branch) []Field { return buildFields(b, false) }

func buildFields(b branch, deep bool) []Field {
	fs := make([]Field, 0, len(b.props))
	for name, raw := range b.props {
		p, _ := raw.(map[string]any)
		f := fieldFrom(name, p, deep)
		f.Required = b.req[name]
		fs = append(fs, f)
	}
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].Required != fs[j].Required {
			return fs[i].Required
		}
		return fs[i].Name < fs[j].Name
	})
	return fs
}

func fieldFrom(name string, p map[string]any, deep bool) Field {
	f := Field{Name: name, Type: typeOf(p), Default: p["default"]}
	if e, ok := p["enum"].([]any); ok && len(e) > 0 {
		f.Enum = e
	}
	if d, ok := p["description"].(string); ok {
		f.Description = truncate(d)
	}
	f.Minimum = number(p["minimum"])
	f.Maximum = number(p["maximum"])
	if n := number(p["maxItems"]); n != nil {
		v := int(*n)
		f.MaxItems = &v
	}
	if items, ok := p["items"].(map[string]any); ok {
		f.Items = typeOf(items)
		if props, ok := items["properties"].(map[string]any); ok && len(props) > 0 && deep {
			ib := branch{props: props, req: map[string]bool{}}
			mergeOwn(&ib, items)
			f.ItemFields = flatFields(ib)
		}
	}
	return f
}

// typeOf names a schema's type; type arrays join with "|", and untyped nodes
// are inferred from their shape.
func typeOf(p map[string]any) string {
	switch t := p["type"].(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "|")
	}
	switch {
	case p["properties"] != nil:
		return "object"
	case p["items"] != nil:
		return "array"
	case p["enum"] != nil:
		return "enum"
	case p["oneOf"] != nil, p["anyOf"] != nil, p["allOf"] != nil:
		return "any"
	}
	return ""
}

func number(v any) *float64 {
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}

func truncate(s string) string { return truncateTo(s, maxDescRunes) }

func truncateTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
