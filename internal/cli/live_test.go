//go:build live

package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
)

// Live smoke tests: run by hand only (build tag live + KIE_E2E=1). Paid tests
// additionally need KIE_E2E_PAID=1. Nothing here prints the API key.

const defaultLiveBudget = 30.0

var (
	budgetOnce  sync.Once
	budgetStart float64
)

// liveEnv skips unless the live run is enabled and returns a runner that
// executes the CLI in-process and parses the single JSON object on stdout.
func liveEnv(t *testing.T) func(args ...string) (map[string]any, int) {
	t.Helper()
	if os.Getenv("KIE_E2E") != "1" {
		t.Skip("set KIE_E2E=1 to run live tests")
	}
	if os.Getenv("KIE_API_KEY") == "" {
		t.Skip("KIE_API_KEY is empty")
	}
	home := t.TempDir()
	env := func(k string) string {
		if k == "KIE_CLI_HOME" {
			return home
		}
		return os.Getenv(k)
	}
	return func(args ...string) (map[string]any, int) {
		t.Helper()
		var out, errw bytes.Buffer
		code := cli.Execute(cli.Deps{
			Stdout: &out, Stderr: &errw, Stdin: strings.NewReader(""),
			Env: env, Now: time.Now,
		}, args)
		for _, l := range strings.Split(errw.String(), "\n") {
			if strings.Contains(l, "warning:") {
				t.Log(l)
			}
		}
		var m map[string]any
		if err := json.Unmarshal(out.Bytes(), &m); err != nil {
			t.Fatalf("%v: stdout is not one JSON object (%d bytes, exit %d)", args[:min(2, len(args))], out.Len(), code)
		}
		return m, code
	}
}

// credits reads the current balance.
func credits(t *testing.T, run func(...string) (map[string]any, int)) float64 {
	t.Helper()
	m, code := run("credits")
	n, ok := m["credits"].(float64)
	if code != 0 || !ok {
		t.Fatalf("credits: exit %d, ok=%v", code, m["ok"])
	}
	return n
}

// paid skips unless paid tests are enabled and cost fits the run budget.
func paid(t *testing.T, cost float64) {
	t.Helper()
	if os.Getenv("KIE_E2E_PAID") != "1" {
		t.Skip("set KIE_E2E_PAID=1 to run paid live tests")
	}
	run := liveEnv(t)
	limit := defaultLiveBudget
	if v, err := strconv.ParseFloat(os.Getenv("KIE_E2E_BUDGET"), 64); err == nil && v > 0 {
		limit = v
	}
	current := credits(t, run)
	budgetOnce.Do(func() { budgetStart = current })
	if ok, reason := (liveBudget{limit: limit, start: budgetStart}).allow(current, cost); !ok {
		t.Skip(reason)
	}
}

func TestLiveAuthAndCredits(t *testing.T) {
	run := liveEnv(t)
	m, code := run("auth", "status")
	if code != 0 || m["ok"] != true || m["source"] != "env" {
		t.Fatalf("auth status: exit %d ok=%v source=%v", code, m["ok"], m["source"])
	}
	n := credits(t, run)
	if n <= 0 {
		t.Fatalf("credits = %v, want > 0", n)
	}
	t.Logf("credits balance: %v", n)
}

func TestLiveCatalog(t *testing.T) {
	run := liveEnv(t)
	m, code := run("models", "list")
	total, _ := m["total"].(float64)
	if code != 0 || total < 180 {
		t.Fatalf("models list: exit %d total=%v, want >= 180 (chat models are hidden: 227 catalog - 40 chat = 187)", code, m["total"])
	}
	t.Logf("model count: %v", total)
	// Search matches model id/title: "suno" only names the recovery model,
	// the main Suno generator is found through "ai-music".
	for query, want := range map[string]string{
		"suno":     "ai-music-api/suno-recovery-audio",
		"ai-music": "ai-music-api/generate",
	} {
		m, code = run("models", "search", query)
		if code != 0 {
			t.Fatalf("models search %s: exit %d", query, code)
		}
		found := false
		ms, _ := m["models"].([]any)
		for _, x := range ms {
			if mm, _ := x.(map[string]any); mm["model"] == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("search %s (%d hits) misses %s", query, len(ms), want)
		}
	}
}

var schemaModels = []string{
	"z-image", "recraft/remove-background", "veo-3-1", "ai-music-api/generate",
	"elevenlabs/text-to-speech-multilingual-v2", "kling/v2-5-turbo-image-to-video-pro",
}

func TestLiveSchemas(t *testing.T) {
	run := liveEnv(t)
	for _, model := range schemaModels {
		t.Run(model, func(t *testing.T) {
			m, code := run("models", "schema", model)
			sum, _ := m["schema"].(map[string]any)
			fields, _ := sum["fields"].([]any)
			if code != 0 || len(fields) == 0 {
				t.Fatalf("schema: exit %d fields=%d", code, len(fields))
			}
		})
	}
}

var paidModels = []string{
	"z-image", "recraft/remove-background", "google/imagen4-fast",
	"elevenlabs/text-to-speech-multilingual-v2", "ai-music-api/generate",
	"ai-music-api/suno-recovery-audio",
}

func TestLivePrices(t *testing.T) {
	run := liveEnv(t)
	for _, model := range paidModels {
		t.Run(model, func(t *testing.T) {
			m, code := run("models", "price", model)
			if code != 0 {
				t.Fatalf("price: exit %d error=%v", code, m["error"])
			}
			t.Logf("%s: %v", model, m["pricingDesc"])
		})
	}
}

func tempPNG(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.png")
	if err := os.WriteFile(p, pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLiveDryRunPresets(t *testing.T) {
	run := liveEnv(t)
	png := tempPNG(t)
	cases := map[string][]string{
		"imagen-rapida":    {"prompt=test"},
		"imagen-calidad":   {"prompt=test"},
		"imagen-edicion":   {"prompt=test", "image_urls=" + png},
		"video-calidad":    {"prompt=test"},
		"video-imagen":     {"prompt=test", "image_url=" + png},
		"video-inicio-fin": {"prompt=test", "image_urls=" + png, "image_urls=" + png},
		"video-barato":     {"prompt=test"},
		"musica":           {"prompt=test"},
		"instrumental":     {"prompt=test"},
		"voz":              {"text=hola"},
	}
	for name, params := range cases {
		t.Run(name, func(t *testing.T) {
			args := []string{"run", "@" + name, "--dry-run"}
			for _, p := range params {
				args = append(args, "-p", p)
			}
			m, code := run(args...)
			if code != 0 {
				t.Fatalf("dry-run: exit %d error=%v", code, m["error"])
			}
		})
	}
}

func TestLiveDryRunVideoChain(t *testing.T) {
	run := liveEnv(t)
	m, code := run("run", "veo/extend", "-p", "task_id=fake-task", "-p", "prompt=x", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run: exit %d error=%v", code, m["error"])
	}
}

// TestLivePaidGate checks the paid gate itself: an unknown cost must skip, so
// it can never spend credits (it also keeps paid in use until paid tests land).
func TestLivePaidGate(t *testing.T) {
	paid(t, 0)
	t.Fatal("paid(0) must skip")
}
