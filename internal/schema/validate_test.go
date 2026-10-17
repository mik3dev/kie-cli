package schema_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mik3dev/kie-cli/internal/schema"
)

func inputSchema(t *testing.T, model string) map[string]any {
	t.Helper()
	sch, err := load(t, "../../testdata/live/schemas/"+model+".json").InputSchema()
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func findProblem(ps []schema.Problem, field string) (schema.Problem, bool) {
	for _, p := range ps {
		if p.Field == field {
			return p, true
		}
	}
	return schema.Problem{}, false
}

func TestValidateAllLiveSchemasCompile(t *testing.T) {
	n := 0
	for f, d := range liveDocs(t) {
		if d.Null() || !d.IsJob() {
			continue
		}
		sch, err := d.InputSchema()
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if _, err := schema.Validate(sch, map[string]any{}); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		n++
	}
	if n == 0 {
		t.Fatal("no job schemas validated")
	}
}

func TestValidateSeedance(t *testing.T) {
	sch := inputSchema(t, "bytedance__seedance-2-5")

	ps, err := schema.Validate(sch, map[string]any{"prompt": "x", "resolution": "4k"})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := findProblem(ps, "resolution")
	if !ok || p.Got != "4k" || !strings.Contains(p.Message, "720p") {
		t.Fatalf("resolution: %#v", ps)
	}

	ps, _ = schema.Validate(sch, map[string]any{"prompt": "x", "duration": "cinco"})
	p, ok = findProblem(ps, "duration")
	if !ok || p.Got != "cinco" || !strings.Contains(p.Message, "integer") {
		t.Fatalf("duration: %#v", ps)
	}

	urls := make([]any, 31)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://a/%d.png", i)
	}
	ps, _ = schema.Validate(sch, map[string]any{"prompt": "x", "reference_image_urls": urls})
	p, ok = findProblem(ps, "reference_image_urls")
	if !ok || !strings.Contains(p.Message, "30") {
		t.Fatalf("urls: %#v", ps)
	}

	ps, err = schema.Validate(sch, map[string]any{"prompt": "x", "resolution": "720p", "duration": json.Number("5")})
	if err != nil || ps != nil {
		t.Fatalf("valid input: %v %#v", err, ps)
	}
}

func TestValidateRequiredAndUnknown(t *testing.T) {
	sch := inputSchema(t, "kling-3.0-omni__image-to-video")

	ps, err := schema.Validate(sch, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"prompt", "image_urls"} {
		p, ok := findProblem(ps, f)
		if !ok || p.Got != nil {
			t.Fatalf("%s missing: %#v", f, ps)
		}
	}

	ps, _ = schema.Validate(sch, map[string]any{"prompt": "x", "image_urls": []any{"https://a/1.png"}, "promt": "y"})
	p, ok := findProblem(ps, "promt")
	if !ok || !strings.Contains(p.Message, "unknown field") || !strings.Contains(p.Message, "kie-cli models schema") {
		t.Fatalf("unknown: %#v", ps)
	}
}

func TestValidateOneOfNamesVariants(t *testing.T) {
	sch := inputSchema(t, "kling-3.0-omni__image-to-video")
	ps, _ := schema.Validate(sch, map[string]any{
		"prompt":     "x",
		"image_urls": []any{"https://a/1.png", "https://a/2.png", "https://a/3.png"},
	})
	var all strings.Builder
	for _, p := range ps {
		all.WriteString(p.Message + "\n")
	}
	for _, v := range []string{`"Single First Frame"`, `"First and Last Frames"`} {
		if !strings.Contains(all.String(), v) {
			t.Fatalf("missing %s in:\n%s", v, all.String())
		}
	}
}

func TestValidateNestedFieldPath(t *testing.T) {
	sch := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"elements": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"name": map[string]any{"type": "integer"}},
				},
			},
		},
	}
	ps, err := schema.Validate(sch, map[string]any{"elements": []any{
		map[string]any{"name": json.Number("1")}, map[string]any{"name": "x"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := findProblem(ps, "elements[1].name"); !ok || p.Got != "x" {
		t.Fatalf("got %#v", ps)
	}
}

func TestValidateAdditionalPropertiesTrueSkipsUnknown(t *testing.T) {
	sch := map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties":           map[string]any{"a": map[string]any{"type": "string"}},
	}
	ps, err := schema.Validate(sch, map[string]any{"a": "x", "extra": 1})
	if err != nil || ps != nil {
		t.Fatalf("%v %#v", err, ps)
	}
}

func TestValidateAdditionalPropertiesFalseDedups(t *testing.T) {
	sch := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{"a": map[string]any{"type": "string"}},
	}
	ps, _ := schema.Validate(sch, map[string]any{"a": "x", "extra": 1})
	if len(ps) != 1 || ps[0].Field != "extra" || !strings.Contains(ps[0].Message, "unknown field") {
		t.Fatalf("%#v", ps)
	}
}

func TestValidateOneOfMultipleMatch(t *testing.T) {
	sch := map[string]any{
		"type":       "object",
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
		"oneOf": []any{
			map[string]any{"title": "One", "properties": map[string]any{"a": map[string]any{"type": "string"}}},
			map[string]any{"title": "Two", "properties": map[string]any{"a": map[string]any{"minLength": 1}}},
		},
	}
	ps, _ := schema.Validate(sch, map[string]any{"a": "x"})
	if len(ps) != 1 || ps[0].Field != "" || !strings.Contains(ps[0].Message, `"One"`) || !strings.Contains(ps[0].Message, `"Two"`) {
		t.Fatalf("%#v", ps)
	}
}

func TestValidateSortedAndUntitledVariant(t *testing.T) {
	sch := map[string]any{
		"type":     "object",
		"required": []any{"z", "b"},
		"properties": map[string]any{
			"z": map[string]any{"type": "string"},
			"b": map[string]any{"type": "string"},
		},
		"oneOf": []any{
			map[string]any{"required": []any{"z"}},
			map[string]any{"required": []any{"b"}},
		},
	}
	ps, _ := schema.Validate(sch, map[string]any{})
	for i := 1; i < len(ps); i++ {
		if ps[i-1].Field > ps[i].Field {
			t.Fatalf("unsorted: %#v", ps)
		}
	}
	found := false
	for _, p := range ps {
		found = found || strings.Contains(p.Message, "variant 1: ")
	}
	if !found {
		t.Fatalf("no untitled variant prefix: %#v", ps)
	}
}

func TestValidateNoPropertiesSkipsUnknown(t *testing.T) {
	ps, err := schema.Validate(map[string]any{"type": "object"}, map[string]any{"x": 1})
	if err != nil || ps != nil {
		t.Fatalf("%v %#v", err, ps)
	}
}

func TestValidateBadSchemaErrors(t *testing.T) {
	sch := map[string]any{"type": "nonsense"}
	if _, err := schema.Validate(sch, map[string]any{}); err == nil {
		t.Fatal("want compile error")
	}
}

func TestValidateDoesNotMutate(t *testing.T) {
	sch := inputSchema(t, "kling-3.0-omni__image-to-video")
	schCopy := inputSchema(t, "kling-3.0-omni__image-to-video")
	in := map[string]any{"prompt": 3, "image_urls": []any{"u"}, "promt": "y"}
	inCopy := map[string]any{"prompt": 3, "image_urls": []any{"u"}, "promt": "y"}
	if _, err := schema.Validate(sch, in); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, inCopy) || !reflect.DeepEqual(sch, schCopy) {
		t.Fatal("mutated")
	}
}

func TestValidateUnknownReportedOncePerKey(t *testing.T) {
	sch := inputSchema(t, "kling-3.0-omni__image-to-video")
	ps, _ := schema.Validate(sch, map[string]any{"prompt": "x", "image_urls": []any{"https://a/1.png"}, "promt": "y"})
	n := 0
	for _, p := range ps {
		if p.Field == "promt" {
			n++
			if p.Got != "y" {
				t.Fatalf("got %#v", p)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want 1 problem for promt, got %d: %#v", n, ps)
	}
}

func TestValidateNestedAdditionalPropertiesFalse(t *testing.T) {
	sch := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"o": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"a": map[string]any{}}},
		},
	}
	ps, _ := schema.Validate(sch, map[string]any{"o": map[string]any{"a": 1, "b": 2}})
	if p, ok := findProblem(ps, "o.b"); !ok || p.Got != 2 {
		t.Fatalf("%#v", ps)
	}
}
