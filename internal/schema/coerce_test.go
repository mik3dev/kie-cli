package schema_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mik3dev/kie-cli/internal/schema"
)

func syntheticSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"n":     map[string]any{"type": "integer"},
		"f":     map[string]any{"type": "number"},
		"b":     map[string]any{"type": "boolean"},
		"s":     map[string]any{"type": "string"},
		"arr":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"urls":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"obj":   map[string]any{"type": "object", "properties": map[string]any{"k": map[string]any{"type": "integer"}}},
		"multi": map[string]any{"type": []any{"string", "integer"}},
	}}
}

func TestCoerce(t *testing.T) {
	sch := syntheticSchema()
	cases := []struct {
		key      string
		in, want any
	}{
		{"n", "5", json.Number("5")},
		{"n", " 5 ", json.Number("5")},
		{"n", "abc", "abc"},
		{"f", "1.5", json.Number("1.5")},
		{"b", "TRUE", true},
		{"b", "nope", "nope"},
		{"s", 5.0, "5"},
		{"s", 1.5, "1.5"},
		{"s", true, "true"},
		{"s", json.Number("7"), "7"},
		{"s", "[intro] hola", "[intro] hola"},
		{"s", "[1,2]", "[1,2]"},
		{"arr", "3", []any{json.Number("3")}},
		{"arr", "[1,2]", []any{1.0, 2.0}},
		{"arr", []any{"1", "2"}, []any{json.Number("1"), json.Number("2")}},
		{"urls", "a.png", []any{"a.png"}},
		{"urls", "[intro]", []any{"[intro]"}},
		{"obj", `{"k":"7"}`, map[string]any{"k": json.Number("7")}},
		{"obj", map[string]any{"k": "7", "z": "1"}, map[string]any{"k": json.Number("7"), "z": "1"}},
		{"multi", "5", "5"},
		{"unknown", "true", true},
		{"unknown", "[intro]", "[intro]"},
		{"unknown", 3.0, 3.0},
	}
	for _, c := range cases {
		got := schema.Coerce(sch, map[string]any{c.key: c.in})[c.key]
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s=%#v: got %#v, want %#v", c.key, c.in, got, c.want)
		}
	}
}

func TestCoerceNilSchemaGuessesLiterals(t *testing.T) {
	in := map[string]any{"a": "true", "b": "5", "c": `{"a":1}`, "d": "hola", "e": " 5", "f": "[1,2]", "g": "{nope"}
	want := map[string]any{"a": true, "b": 5.0, "c": map[string]any{"a": 1.0}, "d": "hola", "e": " 5", "f": []any{1.0, 2.0}, "g": "{nope"}
	if got := schema.Coerce(nil, in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestCoerceOneOfBranchProps(t *testing.T) {
	doc := load(t, "../../testdata/live/schemas/kling-3.0-omni__image-to-video.json")
	sch, err := doc.InputSchema()
	if err != nil {
		t.Fatal(err)
	}
	got := schema.Coerce(sch, map[string]any{"duration": "5", "image_urls": "a.png", "audio": "true"})
	want := map[string]any{"duration": json.Number("5"), "image_urls": []any{"a.png"}, "audio": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestPropSchema(t *testing.T) {
	sch := map[string]any{
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
		"oneOf": []any{
			map[string]any{"properties": map[string]any{"b": map[string]any{"type": "integer"}}},
			map[string]any{"properties": map[string]any{"b": map[string]any{"type": "string"}}},
		},
		"anyOf": []any{map[string]any{"properties": map[string]any{"c": map[string]any{"type": "number"}}}},
		"allOf": []any{map[string]any{"properties": map[string]any{"d": map[string]any{"type": "boolean"}}}},
	}
	for key, want := range map[string]string{"a": "string", "b": "integer", "c": "number", "d": "boolean"} {
		p, ok := schema.PropSchema(sch, key)
		if !ok || p["type"] != want {
			t.Errorf("%s: got %v ok=%v, want type %s", key, p, ok, want)
		}
	}
	if _, ok := schema.PropSchema(sch, "zzz"); ok {
		t.Error("zzz must not be found")
	}
	if _, ok := schema.PropSchema(nil, "a"); ok {
		t.Error("nil schema must not match")
	}
}

func TestCoerceDoesNotMutate(t *testing.T) {
	sch := syntheticSchema()
	schCopy := syntheticSchema()
	in := map[string]any{"n": "5", "arr": []any{"1", "2"}, "obj": map[string]any{"k": "7"}, "u": "true"}
	inCopy := map[string]any{"n": "5", "arr": []any{"1", "2"}, "obj": map[string]any{"k": "7"}, "u": "true"}
	_ = schema.Coerce(sch, in)
	if !reflect.DeepEqual(in, inCopy) || !reflect.DeepEqual(sch, schCopy) {
		t.Fatalf("mutated: in=%v sch=%v", in, sch)
	}
}

func TestCoerceSeedanceDuration(t *testing.T) {
	doc := load(t, "../../testdata/live/schemas/bytedance__seedance-2-5.json")
	sch, err := doc.InputSchema()
	if err != nil {
		t.Fatal(err)
	}
	got := schema.Coerce(sch, map[string]any{"duration": "5"})["duration"]
	if got != json.Number("5") {
		t.Fatalf("got %#v", got)
	}
}

func TestCoerceTypelessAndBranchTypes(t *testing.T) {
	sch := map[string]any{"properties": map[string]any{
		"u":     map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "boolean"}}},
		"blank": map[string]any{"description": "no type"},
	}}
	got := schema.Coerce(sch, map[string]any{"u": "true", "blank": "5", "x": 2})
	want := map[string]any{"u": true, "blank": "5", "x": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if got := schema.Coerce(sch, map[string]any{"u": "7"})["u"]; got != json.Number("7") {
		t.Fatalf("got %#v", got)
	}
}
