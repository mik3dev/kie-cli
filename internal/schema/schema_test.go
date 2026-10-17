package schema_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/schema"
)

func load(t *testing.T, p string) schema.Doc {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var d schema.Doc
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return d
}

func liveDocs(t *testing.T) map[string]schema.Doc {
	t.Helper()
	files, _ := filepath.Glob("../../testdata/live/schemas/*.json")
	if len(files) < 200 {
		t.Fatalf("only %d fixtures", len(files))
	}
	out := map[string]schema.Doc{}
	for _, f := range files {
		out[f] = load(t, f)
	}
	return out
}

func fieldByName(fs []schema.Field, n string) (schema.Field, bool) {
	for _, f := range fs {
		if f.Name == n {
			return f, true
		}
	}
	return schema.Field{}, false
}

func TestResolvePercentEncodedRef(t *testing.T) {
	doc := map[string]any{"components": map[string]any{"schemas": map[string]any{
		"response not with recordId": map[string]any{"type": "object"}}}}
	got, err := schema.Resolve(doc, map[string]any{"$ref": "#/components/schemas/response%20not%20with%20recordId"})
	if err != nil || !reflect.DeepEqual(got, map[string]any{"type": "object"}) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveTrailingSpaceKey(t *testing.T) {
	doc := map[string]any{"components": map[string]any{"responses": map[string]any{
		"Error ": map[string]any{"description": "boom"}}}}
	got, err := schema.Resolve(doc, map[string]any{"$ref": "#/components/responses/Error "})
	if err != nil || !reflect.DeepEqual(got, map[string]any{"description": "boom"}) {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := schema.Resolve(doc, map[string]any{"$ref": "#/components/responses/Error"}); err == nil {
		t.Fatal("trimmed key must not resolve")
	}
}

func TestResolveCycleTerminates(t *testing.T) {
	doc := map[string]any{"components": map[string]any{"schemas": map[string]any{
		"Node": map[string]any{"type": "object", "properties": map[string]any{
			"next": map[string]any{"$ref": "#/components/schemas/Node"}}}}}}
	got, err := schema.Resolve(doc, map[string]any{"$ref": "#/components/schemas/Node"})
	if err != nil {
		t.Fatal(err)
	}
	next := got.(map[string]any)["properties"].(map[string]any)["next"].(map[string]any)
	if next["$ref"] != "#/components/schemas/Node" {
		t.Fatalf("cycle not left as ref: %v", next)
	}
}

func TestResolveDoesNotMutate(t *testing.T) {
	raw := `{"a":{"$ref":"#/b"},"b":{"x":[1,{"$ref":"#/c"}]},"c":{"y":1}}`
	var doc, snap map[string]any
	json.Unmarshal([]byte(raw), &doc)
	json.Unmarshal([]byte(raw), &snap)
	got, err := schema.Resolve(doc, doc["a"])
	if err != nil {
		t.Fatal(err)
	}
	got.(map[string]any)["x"].([]any)[1].(map[string]any)["y"] = 99
	if !reflect.DeepEqual(doc, snap) {
		t.Fatal("doc mutated")
	}
}

func TestResolveDanglingAndExternalRef(t *testing.T) {
	doc := map[string]any{}
	if _, err := schema.Resolve(doc, map[string]any{"$ref": "#/nope/x"}); err == nil {
		t.Fatal("dangling: want error")
	}
	got, err := schema.Resolve(doc, map[string]any{"$ref": "http://x/y.json"})
	if err != nil || got.(map[string]any)["$ref"] != "http://x/y.json" {
		t.Fatalf("external ref kept as is: %v %v", got, err)
	}
}

func TestResolveSiblingsOverlay(t *testing.T) {
	doc := map[string]any{"d": map[string]any{"type": "string", "description": "old"}}
	got, _ := schema.Resolve(doc, map[string]any{"$ref": "#/d", "description": "new"})
	want := map[string]any{"type": "string", "description": "new"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestSummarizeGptImage2(t *testing.T) {
	d := load(t, "../../testdata/live/schemas/gpt-image-2-text-to-image.json")
	s, err := schema.Summarize(d)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Job || s.Null || s.Method != "POST" || s.Path != "/api/v1/jobs/createTask" {
		t.Fatalf("meta: %+v", s)
	}
	p, ok := fieldByName(s.Fields, "prompt")
	if !ok || !p.Required || p.Type != "string" {
		t.Fatalf("prompt: %+v", p)
	}
	ar, ok := fieldByName(s.Fields, "aspect_ratio")
	if !ok || len(ar.Enum) == 0 {
		t.Fatalf("aspect_ratio: %+v", ar)
	}
	if !s.Fields[0].Required {
		t.Fatal("required fields must come first")
	}
}

func TestSummarizeOneOfVariants(t *testing.T) {
	found := false
	for f, d := range liveDocs(t) {
		if d.OpenAPI == nil {
			continue
		}
		in, err := d.InputSchema()
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if _, ok := in["oneOf"]; !ok {
			continue
		}
		s, err := schema.Summarize(d)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if len(s.Variants) >= 2 && len(s.Fields) == 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("no live oneOf produced variants")
	}
	s, err := schema.Summarize(load(t, "testdata/oneof.json"))
	if err != nil || len(s.Variants) != 2 || len(s.Fields) != 0 {
		t.Fatalf("fixture: %+v %v", s, err)
	}
	// base required/props are merged into each variant
	for _, v := range s.Variants {
		if p, ok := fieldByName(v, "prompt"); !ok || !p.Required {
			t.Fatalf("variant lacks base prompt: %+v", v)
		}
	}
	b, _ := fieldByName(s.Variants[1], "b")
	if b.Minimum == nil || *b.Minimum != 1 || *b.Maximum != 5 || !b.Required {
		t.Fatalf("b: %+v", b)
	}
}

func TestSummarizeAllOfMerges(t *testing.T) {
	d := load(t, "../../testdata/live/schemas/kling-3.0-omni__text-to-video.json")
	s, err := schema.Summarize(d)
	if err != nil || len(s.Fields) == 0 {
		t.Fatalf("%v %+v", err, s)
	}
	mp, ok := fieldByName(s.Fields, "multi_prompt")
	if !ok || mp.Type != "array" || mp.Items != "object" || mp.MaxItems == nil || *mp.MaxItems != 6 {
		t.Fatalf("multi_prompt: %+v", mp)
	}
}

func TestSummarizeNonJobPath(t *testing.T) {
	d := load(t, "../../testdata/live/schemas/gemini-omni-audio.json")
	s, err := schema.Summarize(d)
	if err != nil || s.Job || len(s.Fields) == 0 || s.Path != "/api/v1/omni/audio/create" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestNullSchema(t *testing.T) {
	d := load(t, "../../testdata/live/schemas/google__gemini-3-8-flash-tts.json")
	if d.OpenAPI != nil || !d.Null() {
		t.Fatal("want null doc")
	}
	m, p, err := d.Endpoint()
	if err != nil || m != "POST" || p != "/api/v1/jobs/createTask" || !d.IsJob() {
		t.Fatalf("%s %s %v", m, p, err)
	}
	s, err := schema.Summarize(d)
	if err != nil || !s.Null || !s.Job || len(s.Fields) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestDescriptionTruncatedTo300Runes(t *testing.T) {
	long := strings.Repeat("é", 400)
	d := schema.Doc{Model: "m", OpenAPI: map[string]any{"paths": map[string]any{"/api/v1/jobs/createTask": map[string]any{"post": map[string]any{
		"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
			"properties": map[string]any{"input": map[string]any{"properties": map[string]any{
				"p": map[string]any{"type": []any{"string", "null"}, "description": long}}}}}}}}}}}}}
	s, err := schema.Summarize(d)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(s.Fields[0].Description)); n != 300 {
		t.Fatalf("len=%d", n)
	}
	if s.Fields[0].Type != "string|null" {
		t.Fatalf("type=%q", s.Fields[0].Type)
	}
}

func TestSummarizeAllCaptured(t *testing.T) {
	nulls, nonJob, oneOf := 0, 0, 0
	for f, d := range liveDocs(t) {
		s, err := schema.Summarize(d)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		switch {
		case s.Null:
			nulls++
			continue
		case !s.Job:
			nonJob++
		}
		if len(s.Fields) == 0 && len(s.Variants) == 0 {
			t.Errorf("%s: empty summary", f)
		}
		if len(s.Variants) > 0 {
			oneOf++
		}
	}
	t.Logf("null=%d non-job=%d with-variants=%d", nulls, nonJob, oneOf)
}

func TestFetchCachesAndRefresh(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/api/v1/models/a/b/schema" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"code":200,"msg":"ok","data":{"model":"a/b","openapi":{"paths":{}}}}`))
	}))
	defer srv.Close()
	c := api.New("k", func(n string) string {
		if n == "KIE_BASE_URL" {
			return srv.URL
		}
		return ""
	})
	dir := t.TempDir()
	now := time.Now()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		d, err := schema.Fetch(ctx, c, dir, "a/b", false, now)
		if err != nil || d.Model != "a/b" {
			t.Fatalf("%v %+v", err, d)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
	if _, err := schema.Fetch(ctx, c, dir, "a/b", true, now); err != nil || hits.Load() != 2 {
		t.Fatalf("refresh: %v hits=%d", err, hits.Load())
	}
	if _, err := schema.Fetch(ctx, c, dir, "a/b", false, now.Add(25*time.Hour)); err != nil || hits.Load() != 3 {
		t.Fatalf("expired: %v hits=%d", err, hits.Load())
	}
}

func TestFetchNullData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"code":200,"msg":"ok","data":null}`))
	}))
	defer srv.Close()
	c := api.New("k", func(n string) string {
		if n == "KIE_BASE_URL" {
			return srv.URL
		}
		return ""
	})
	d, err := schema.Fetch(context.Background(), c, t.TempDir(), "m", false, time.Now())
	if err != nil || d.Model != "m" || !d.Null() {
		t.Fatalf("%v %+v", err, d)
	}
}
