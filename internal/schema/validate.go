package schema

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	schemaResource  = "input.json"
	unknownFieldMsg = "unknown field; see 'kie-cli models schema <model>' for valid keys"
)

var printer = message.NewPrinter(language.English)

// Problem is one validation failure. Field is the path relative to the input
// ("image_urls[0]", "elements[1].name"); Got is absent when the value is
// missing.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Got     any    `json:"got,omitempty"`
}

// Validate checks input against the (resolved) input schema. It returns an
// error only when the schema does not compile; problems are sorted by Field
// then Message and nil when the input is valid. Neither argument is mutated.
func Validate(sch map[string]any, input map[string]any) ([]Problem, error) {
	compiled, err := compile(sch)
	if err != nil {
		return nil, err
	}
	var problems []Problem
	if err := compiled.Validate(input); err != nil {
		var verr *jsonschema.ValidationError
		if !errors.As(err, &verr) {
			return nil, fmt.Errorf("validate input: %w", err)
		}
		problems = collect(verr, sch, input, "")
	}
	problems = append(problems, unknownFields(sch, input)...)
	return finish(problems), nil
}

func compile(sch map[string]any) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(schemaResource, sch); err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	compiled, err := c.Compile(schemaResource)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return compiled, nil
}

// collect walks the error tree down to its leaves. variant is the prefix of
// the root oneOf/anyOf branch the subtree belongs to.
func collect(e *jsonschema.ValidationError, root, input map[string]any, variant string) []Problem {
	if len(e.Causes) == 0 {
		return leafProblems(e, root, input, variant)
	}
	var out []Problem
	for i, c := range e.Causes {
		v := variant
		if len(e.InstanceLocation) == 0 && v == "" {
			if kw := branchKeyword(e.ErrorKind); kw != "" {
				v = variantPrefix(root, kw, branchIndex(c, i))
			}
		}
		out = append(out, collect(c, root, input, v)...)
	}
	return out
}

func branchKeyword(k jsonschema.ErrorKind) string {
	switch k.(type) {
	case *kind.OneOf:
		return "oneOf"
	case *kind.AnyOf:
		return "anyOf"
	}
	return ""
}

// branchIndex reads the branch number from the cause's schema location
// (".../oneOf/2"), falling back to its position.
func branchIndex(c *jsonschema.ValidationError, pos int) int {
	frag := c.SchemaURL
	if i := strings.LastIndex(frag, "#"); i >= 0 {
		frag = frag[i+1:]
	}
	parts := strings.Split(frag, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "oneOf" || parts[i] == "anyOf" {
			if n, err := strconv.Atoi(parts[i+1]); err == nil {
				return n
			}
		}
	}
	return pos
}

func variantPrefix(root map[string]any, keyword string, i int) string {
	return fmt.Sprintf("variant %s: ", variantName(root, keyword, i))
}

func variantName(root map[string]any, keyword string, i int) string {
	branches, _ := root[keyword].([]any)
	if i >= 0 && i < len(branches) {
		if b, ok := branches[i].(map[string]any); ok {
			if t, ok := b["title"].(string); ok && t != "" {
				return strconv.Quote(t)
			}
		}
	}
	return strconv.Itoa(i + 1)
}

func leafProblems(e *jsonschema.ValidationError, root, input map[string]any, variant string) []Problem {
	field, got, found := locate(input, e.InstanceLocation)
	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		out := make([]Problem, 0, len(k.Missing))
		for _, m := range k.Missing {
			out = append(out, Problem{Field: join(field, m), Message: variant + "missing required field"})
		}
		return out
	case *kind.AdditionalProperties:
		parent, _ := got.(map[string]any)
		return additionalProblems(k.Properties, field, parent, root, variant)
	case *kind.OneOf:
		if len(k.Subschemas) > 1 {
			names := make([]string, len(k.Subschemas))
			for i, s := range k.Subschemas {
				names[i] = variantName(root, "oneOf", s)
			}
			msg := "input matches more than one variant: " + strings.Join(names, ", ")
			return []Problem{{Field: field, Message: variant + msg}}
		}
	}
	p := Problem{Field: field, Message: variant + e.ErrorKind.LocalizedString(printer)}
	if found {
		p.Got = got
	}
	return []Problem{p}
}

// additionalProblems reports keys rejected by additionalProperties:false. A
// top-level key declared nowhere is left to unknownFields, which reports it
// once instead of once per variant.
func additionalProblems(props []string, field string, parent, root map[string]any, variant string) []Problem {
	out := make([]Problem, 0, len(props))
	for _, p := range props {
		if field == "" && hasProperties(root) {
			if _, known := PropSchema(root, p); !known {
				continue
			}
		}
		pr := Problem{Field: join(field, p), Message: variant + unknownFieldMsg}
		if v, ok := parent[p]; ok {
			pr.Got = v
		}
		out = append(out, pr)
	}
	return out
}

// locate renders the instance location as a readable path and returns the
// value found there.
func locate(input map[string]any, loc []string) (field string, got any, found bool) {
	var cur any = input
	for _, tok := range loc {
		switch v := cur.(type) {
		case map[string]any:
			field = join(field, tok)
			cur, found = v[tok]
		case []any:
			field += "[" + tok + "]"
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(v) {
				return field, nil, false
			}
			cur, found = v[i], true
		default:
			return join(field, tok), nil, false
		}
		if !found {
			return field, nil, false
		}
	}
	return field, cur, len(loc) > 0
}

func join(field, key string) string {
	if field == "" {
		return key
	}
	return field + "." + key
}

// unknownFields flags top-level keys the schema declares nowhere. It skips
// open schemas and schemas without any declared property.
func unknownFields(sch, input map[string]any) []Problem {
	if !closedEnough(sch) {
		return nil
	}
	var out []Problem
	for k, v := range input {
		if _, ok := PropSchema(sch, k); ok {
			continue
		}
		out = append(out, Problem{Field: k, Message: unknownFieldMsg, Got: v})
	}
	return out
}

func closedEnough(sch map[string]any) bool {
	switch ap := sch["additionalProperties"].(type) {
	case bool:
		if ap {
			return false
		}
	case map[string]any:
		return false
	}
	return hasProperties(sch)
}

func hasProperties(sch map[string]any) bool {
	if props, ok := sch["properties"].(map[string]any); ok && len(props) > 0 {
		return true
	}
	for _, bk := range branchKeys {
		branches, _ := sch[bk].([]any)
		for _, b := range branches {
			if bm, ok := b.(map[string]any); ok && hasProperties(bm) {
				return true
			}
		}
	}
	return false
}

func finish(in []Problem) []Problem {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]Problem, 0, len(in))
	for _, p := range in {
		key := p.Field + "\x00" + p.Message
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		return out[i].Message < out[j].Message
	})
	return out
}
