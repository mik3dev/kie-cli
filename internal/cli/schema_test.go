package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func schemaServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/models/"), "/schema")
		b, err := os.ReadFile("../../testdata/live/schemas/" + strings.ReplaceAll(model, "/", "__") + ".json")
		if err != nil {
			w.Write([]byte(`{"code":404,"msg":"nf"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": json.RawMessage(b)})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestModelsSchemaSummaryAndCache(t *testing.T) {
	var hits atomic.Int32
	srv := schemaServer(t, &hits)
	env := apiEnv(srv)
	env["KIE_CLI_HOME"] = t.TempDir()
	code, m, _ := run(t, env, "models", "schema", "gpt-image-2-text-to-image")
	sum, _ := m["schema"].(map[string]any)
	if code != 0 || m["ok"] != true || sum["job"] != true || len(sum["fields"].([]any)) == 0 || m["warning"] != nil {
		t.Fatalf("code=%d m=%v", code, m)
	}
	run(t, env, "models", "schema", "gpt-image-2-text-to-image")
	if hits.Load() != 1 {
		t.Fatalf("hits=%d (cache not used)", hits.Load())
	}
	run(t, env, "models", "schema", "gpt-image-2-text-to-image", "--refresh")
	if hits.Load() != 2 {
		t.Fatalf("hits=%d after refresh", hits.Load())
	}
}

func TestModelsSchemaRaw(t *testing.T) {
	var hits atomic.Int32
	srv := schemaServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "models", "schema", "gpt-image-2-text-to-image", "--raw")
	oa, _ := m["openapi"].(map[string]any)
	if code != 0 || m["model"] != "gpt-image-2-text-to-image" || oa["paths"] == nil || m["schema"] != nil {
		t.Fatalf("code=%d m=%v", code, m)
	}
}

func TestModelsSchemaNullWarns(t *testing.T) {
	var hits atomic.Int32
	srv := schemaServer(t, &hits)
	for _, args := range [][]string{
		{"models", "schema", "google/gemini-3-8-flash-tts"},
		{"models", "schema", "google/gemini-3-8-flash-tts", "--raw"},
	} {
		code, m, _ := run(t, apiEnv(srv), args...)
		if code != 0 || m["warning"] != "schema not synced by kie; run without validation" {
			t.Fatalf("%v: code=%d m=%v", args, code, m)
		}
	}
}

func TestModelsSchemaUnknownModelIsAPIError(t *testing.T) {
	var hits atomic.Int32
	srv := schemaServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "models", "schema", "no/such")
	if code == 0 || m["ok"] != false {
		t.Fatalf("code=%d m=%v", code, m)
	}
}
