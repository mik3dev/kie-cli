package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func liveServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	read := func(n string) json.RawMessage {
		b, err := os.ReadFile("../../testdata/live/" + n)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var data json.RawMessage
		switch {
		case r.URL.Path == "/api/v1/models":
			data = read("models.json")
		case r.URL.Path == "/api/v1/chat/credit":
			data = read("credit.json")
		case strings.HasSuffix(r.URL.Path, "/price"):
			data = read("price/google__imagen4.json")
		case strings.HasSuffix(r.URL.Path, "/success-rate"):
			data = read("success-rate/ai-music-api__extend.json")
		default:
			w.Write([]byte(`{"code":404,"msg":"nf"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, env map[string]string, args ...string) (int, map[string]any, *bytes.Buffer) {
	t.Helper()
	deps, out, errw := authDeps(t, "", env)
	deps.Now = time.Now
	code := cli.Execute(deps, args)
	return code, decode(t, out), errw
}

func apiEnv(srv *httptest.Server) map[string]string {
	return map[string]string{"KIE_API_KEY": "k-secret-123456", "KIE_BASE_URL": srv.URL}
}

func TestCreditsCommand(t *testing.T) {
	var hits atomic.Int32
	srv := liveServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "credits")
	want, _ := os.ReadFile("../../testdata/live/credit.json")
	var wantN float64
	json.Unmarshal(want, &wantN)
	if code != 0 || m["ok"] != true || m["credits"] != wantN {
		t.Fatalf("code=%d m=%v want=%v", code, m, wantN)
	}
}

func TestCreditsWithoutKeyIsAuthError(t *testing.T) {
	code, m, _ := run(t, map[string]string{}, "credits")
	if code != 3 || m["ok"] != false {
		t.Fatalf("code=%d m=%v", code, m)
	}
}

func TestModelsListFilterAndCache(t *testing.T) {
	var hits atomic.Int32
	srv := liveServer(t, &hits)
	env := apiEnv(srv)
	env["KIE_CLI_HOME"] = t.TempDir()
	code, m, _ := run(t, env, "models", "list", "--type", "video")
	if code != 0 || m["ok"] != true || m["total"].(float64) == 0 {
		t.Fatalf("code=%d total=%v", code, m["total"])
	}
	run(t, env, "models", "list")
	if hits.Load() != 1 {
		t.Fatalf("hits = %d (cache not used)", hits.Load())
	}
	run(t, env, "models", "list", "--refresh")
	if hits.Load() != 2 {
		t.Fatalf("hits = %d after refresh", hits.Load())
	}
}

func TestModelsListEmptyFilterIsOK(t *testing.T) {
	var hits atomic.Int32
	srv := liveServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "models", "list", "--provider", "no-such-provider")
	models, _ := m["models"].([]any)
	if code != 0 || m["ok"] != true || m["total"] != float64(0) || models == nil || len(models) != 0 {
		t.Fatalf("code=%d m=%v", code, m)
	}
}

func TestModelsSearchHidesChat(t *testing.T) {
	var hits atomic.Int32
	srv := liveServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "models", "search", "claude")
	if code != 0 || m["total"] != float64(0) {
		t.Fatalf("code=%d total=%v", code, m["total"])
	}
	_, m, _ = run(t, apiEnv(srv), "models", "search", "imagen")
	if m["total"].(float64) == 0 {
		t.Fatal("imagen not found")
	}
}

func TestModelsPriceAndHealth(t *testing.T) {
	var hits atomic.Int32
	srv := liveServer(t, &hits)
	code, m, _ := run(t, apiEnv(srv), "models", "price", "google/imagen4")
	if code != 0 || m["model"] != "google/imagen4" || !strings.Contains(m["pricingDesc"].(string), "Imagen 4") {
		t.Fatalf("price: %d %v", code, m)
	}
	code, m, _ = run(t, apiEnv(srv), "models", "health", "ai-music-api/extend")
	pts, _ := m["points"].([]any)
	if code != 0 || len(pts) == 0 {
		t.Fatalf("health: %d %v", code, m)
	}
}

func TestModelsAPIErrorMapsToExit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"code":401,"msg":"bad key"}`))
	}))
	defer srv.Close()
	code, m, _ := run(t, apiEnv(srv), "models", "list")
	if code != 3 || m["ok"] != false {
		t.Fatalf("code=%d m=%v", code, m)
	}
}
