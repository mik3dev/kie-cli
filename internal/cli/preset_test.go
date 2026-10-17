package cli_test

import (
	"github.com/mik3dev/kie-cli/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePresets(t *testing.T, f *fakeKie, body string) string {
	t.Helper()
	if err := os.MkdirAll(f.home, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(f.home, "presets.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func presetList(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, _ := m["presets"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestPresetList(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "preset", "list")
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	list := presetList(t, r.m)
	if len(list) != 10 {
		t.Fatalf("want 10 presets, got %d", len(list))
	}
	prev := ""
	for _, p := range list {
		name := p["name"].(string)
		if name < prev {
			t.Errorf("not sorted: %s after %s", name, prev)
		}
		prev = name
		if p["source"] != "factory" {
			t.Errorf("%s source=%v", name, p["source"])
		}
		if _, ok := p["params"]; ok {
			t.Errorf("%s must not list params", name)
		}
	}
}

func TestPresetListUserOverride(t *testing.T) {
	f := newFake(t)
	writePresets(t, f, `presets:
  - name: imagen-rapida
    description: mine
    model: google/imagen4
  - name: mio
    description: custom
    model: google/imagen4
`)
	r := f.run(t, "preset", "list")
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	list := presetList(t, r.m)
	if len(list) != 11 {
		t.Fatalf("want 11 presets, got %d", len(list))
	}
	for _, p := range list {
		switch p["name"] {
		case "imagen-rapida", "mio":
			if p["source"] != "user" {
				t.Errorf("%v source=%v", p["name"], p["source"])
			}
		}
	}
}

func TestPresetListBrokenUserFile(t *testing.T) {
	f := newFake(t)
	path := writePresets(t, f, "presets: [")
	r := f.run(t, "preset", "list")
	if r.code != 2 || !strings.Contains(r.out, path) || errField(t, r.m, "code") != "usage" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestPresetShow(t *testing.T) {
	f := newFake(t)
	a := f.run(t, "preset", "show", "imagen-rapida")
	b := f.run(t, "preset", "show", "@imagen-rapida")
	if a.code != 0 || b.code != 0 || a.out != b.out {
		t.Fatalf("a=%d b=%d\n%s\n%s", a.code, b.code, a.out, b.out)
	}
	p := a.m["preset"].(map[string]any)
	if p["model"] != "google/imagen4-fast" || p["source"] != "factory" {
		t.Errorf("preset=%v", p)
	}
	if params := p["params"].(map[string]any); params["aspect_ratio"] != "1:1" {
		t.Errorf("params=%v", params)
	}
}

func TestPresetShowUnknown(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "preset", "show", "nope")
	if r.code != 2 || !strings.Contains(r.out, "unknown preset @nope") {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func presetDryBody(t *testing.T, r cliResult) map[string]any {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	return r.m["body"].(map[string]any)
}

func TestRunPresetDryRun(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--dry-run")
	body := presetDryBody(t, r)
	in := body["input"].(map[string]any)
	if body["model"] != "google/imagen4-fast" || in["prompt"] != "gato" || in["aspect_ratio"] != "1:1" || len(in) != 2 {
		t.Errorf("body=%v", body)
	}
	if r.m["preset"] != "imagen-rapida" {
		t.Errorf("preset=%v", r.m["preset"])
	}
	if plain := f.run(t, "run", imageModel, "-p", "prompt=x", "--dry-run"); plain.m["preset"] != nil {
		t.Errorf("plain run must not report a preset: %v", plain.m)
	}
}

func TestRunPresetPrecedence(t *testing.T) {
	f := newFake(t)
	in := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(in, []byte(`{"aspect_ratio":"16:9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--input", in, "--dry-run")
	if got := presetDryBody(t, r)["input"].(map[string]any)["aspect_ratio"]; got != "16:9" {
		t.Errorf("--input must beat preset, got %v", got)
	}
	r = f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--input", in, "-p", "aspect_ratio=9:16", "--dry-run")
	if got := presetDryBody(t, r)["input"].(map[string]any)["aspect_ratio"]; got != "9:16" {
		t.Errorf("-p must beat --input, got %v", got)
	}
}

func TestRunPresetUnknown(t *testing.T) {
	f := newFake(t)
	for _, arg := range []string{"@nope", "@"} {
		r := f.run(t, "run", arg)
		if r.code != 2 || !strings.Contains(r.out, "unknown preset "+arg) {
			t.Errorf("%s: code=%d out=%s", arg, r.code, r.out)
		}
	}
	if f.createHits.Load() != 0 {
		t.Error("createTask must not be called")
	}
}

func TestRunPresetUserOverride(t *testing.T) {
	f := newFake(t)
	writePresets(t, f, `presets:
  - name: imagen-rapida
    description: mine
    model: google/imagen4
`)
	r := f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--dry-run")
	if r.code != 2 || errField(t, r.m, "code") != "validation" {
		t.Fatalf("factory params must not leak into the user preset: %s", r.out)
	}
	r = f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "-p", "aspect_ratio=1:1", "--dry-run")
	if got := presetDryBody(t, r)["model"]; got != "google/imagen4" {
		t.Errorf("model=%v", got)
	}
}

func TestRunPresetSubmitOutputs(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--no-wait")
	if r.code != 0 || r.m["preset"] != "imagen-rapida" || r.m["model"] != "google/imagen4-fast" {
		t.Fatalf("no-wait: code=%d out=%s", r.code, r.out)
	}
	r = f.run(t, "run", "@imagen-rapida", "-p", "prompt=gato", "--out", t.TempDir())
	if r.code != 0 || r.m["preset"] != "imagen-rapida" {
		t.Fatalf("wait: code=%d out=%s", r.code, r.out)
	}
}

func TestPresetListNoHome(t *testing.T) {
	f := newFake(t)
	r := f.runWith(t, func(d *cli.Deps) { d.Env = func(string) string { return "" } }, "preset", "list")
	if r.code != 1 || errField(t, r.m, "code") != "io" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}
