package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	seedance = "bytedance/seedance-2-5"
	klingI2V = "kling-3.0-omni/image-to-video"
)

func TestRunValidationErrorBlocksSubmit(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "resolution=4k")
	if r.code != 2 || errField(t, r.m, "code") != "validation" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	ps, _ := errField(t, r.m, "problems").([]any)
	if len(ps) == 0 || ps[0].(map[string]any)["field"] != "resolution" {
		t.Fatalf("problems = %v", ps)
	}
	if f.createHits.Load() != 0 || f.uploadHits.Load() != 0 {
		t.Fatalf("create=%d upload=%d", f.createHits.Load(), f.uploadHits.Load())
	}
}

func TestRunValidationTruncatesLongGot(t *testing.T) {
	f := newFake(t)
	long := strings.Repeat("é", 300)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "resolution="+long)
	ps, _ := errField(t, r.m, "problems").([]any)
	if r.code != 2 || len(ps) == 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	got, _ := ps[0].(map[string]any)["got"].(string)
	if n := utf8.RuneCountInString(got); n == 0 || n > 100 {
		t.Fatalf("got has %d runes, want 1..100", n)
	}
}

func TestRunCoercesIntegerParam(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "duration=5", "--no-wait")
	if r.code != 0 {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	body, _ := f.createBody.Load().(map[string]any)
	in, _ := body["input"].(map[string]any)
	if in["duration"] != 5.0 {
		t.Fatalf("duration = %#v", in["duration"])
	}
}

func TestRunNoValidateSendsWithWarnings(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "resolution=4k", "--no-validate", "--no-wait")
	if r.code != 0 || f.createHits.Load() != 1 {
		t.Fatalf("code=%d hits=%d out=%s", r.code, f.createHits.Load(), r.out)
	}
	if !strings.Contains(r.stderr, "warning: validation: resolution:") {
		t.Fatalf("stderr = %q", r.stderr)
	}
}

func TestRunDryRunWithLocalFileInPatternField(t *testing.T) {
	f := newFake(t)
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", klingI2V, "-p", "prompt=x", "-p", "image_urls="+img, "--dry-run")
	if r.code != 0 || r.m["dryRun"] != true {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	body, _ := r.m["body"].(map[string]any)
	in, _ := body["input"].(map[string]any)
	urls, _ := in["image_urls"].([]any)
	if len(urls) != 1 || !strings.HasPrefix(urls[0].(string), "https://pending-upload.invalid/") {
		t.Fatalf("image_urls = %v", in["image_urls"])
	}
	pend, _ := r.m["pendingUploads"].([]any)
	if len(pend) != 1 || pend[0].(map[string]any)["path"] != img {
		t.Fatalf("pendingUploads = %v", r.m["pendingUploads"])
	}
	if f.createHits.Load() != 0 || f.uploadHits.Load() != 0 {
		t.Fatalf("create=%d upload=%d", f.createHits.Load(), f.uploadHits.Load())
	}
	if _, err := os.Stat(filepath.Join(f.home, "tasks.jsonl")); err == nil {
		t.Error("dry-run must not write the ledger")
	}
}

func TestRunDryRunEmptyPendingAndNoProblems(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "--dry-run")
	pend, ok := r.m["pendingUploads"].([]any)
	if r.code != 0 || !ok || len(pend) != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if _, has := r.m["problems"]; has {
		t.Error("problems must be absent without --no-validate")
	}
}

func TestRunDryRunNoValidateListsProblems(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "resolution=4k", "--dry-run", "--no-validate")
	ps, _ := r.m["problems"].([]any)
	if r.code != 0 || len(ps) == 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunDryRunIncludesPricing(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "aspect_ratio=1:1", "-p", "prompt=a cat", "--dry-run")
	if p, _ := r.m["pricing"].(string); r.code != 0 || p == "" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	r = f.run(t, "run", seedance, "-p", "prompt=x", "--dry-run")
	if _, has := r.m["pricing"]; has || r.code != 0 || !strings.Contains(r.stderr, "warning") {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
}

func TestRunNullSchemaSkipsValidation(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "google/gemini-3-8-flash-tts", "-p", "text=hola", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stderr, "schema not synced") {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
}

func writeFiles(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(dir, n)
		if err := os.WriteFile(paths[i], pngBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestRunDryRunPendingUploadsAreSorted(t *testing.T) {
	ps := writeFiles(t, "a.png", "b.png", "c.png")
	for i := 0; i < 8; i++ {
		f := newFake(t)
		r := f.run(t, "run", seedance, "-p", "prompt=x", "--dry-run", "--no-validate",
			"-p", "zeta_image="+ps[0], "-p", "mid_image="+ps[2], "-p", "alpha_image="+ps[1])
		pend, _ := r.m["pendingUploads"].([]any)
		if r.code != 0 || len(pend) != 3 {
			t.Fatalf("code=%d out=%s", r.code, r.out)
		}
		want := []string{ps[0], ps[1], ps[2]}
		for j, w := range want {
			if pend[j].(map[string]any)["path"] != w {
				t.Fatalf("run %d: pendingUploads = %v, want order %v", i, pend, want)
			}
		}
	}
}

func TestRunInvalidInputWithLocalFileDoesNotUpload(t *testing.T) {
	f := newFake(t)
	img := writeFiles(t, "a.png")[0]
	r := f.run(t, "run", klingI2V, "-p", "prompt=x", "-p", "image_urls="+img, "-p", "bogus_field=1")
	if r.code != 2 || errField(t, r.m, "code") != "validation" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.uploadHits.Load() != 0 || f.createHits.Load() != 0 {
		t.Fatalf("upload=%d create=%d", f.uploadHits.Load(), f.createHits.Load())
	}
}

func TestRunValidationMessageSingular(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "-p", "resolution=4k")
	msg, _ := errField(t, r.m, "message").(string)
	if !strings.Contains(msg, "(1 problem)") {
		t.Fatalf("message = %q", msg)
	}
}

// breakPatterns returns the schema fixture with every "pattern" turned into an invalid regex.
func breakPatterns(t *testing.T, model string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../../testdata/live/schemas/" + strings.ReplaceAll(model, "/", "__") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["pattern"]; ok {
				x["pattern"] = "("
			}
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	out, _ := json.Marshal(doc)
	return out
}

func TestRunUncompilableSchemaWarnsAndContinues(t *testing.T) {
	f := newFake(t)
	f.schemaFn = func(model string) json.RawMessage { return breakPatterns(t, model) }
	r := f.run(t, "run", klingI2V, "-p", "prompt=x", "-p", "image_urls=https://x/a.png", "--no-wait")
	if r.code != 0 || f.createHits.Load() != 1 {
		t.Fatalf("code=%d hits=%d out=%s stderr=%s", r.code, f.createHits.Load(), r.out, r.stderr)
	}
	if !strings.Contains(r.stderr, "warning: cannot validate the input") {
		t.Fatalf("stderr = %q", r.stderr)
	}
}

func createInput(t *testing.T, f *fakeKie) map[string]any {
	t.Helper()
	body, _ := f.createBody.Load().(map[string]any)
	in, _ := body["input"].(map[string]any)
	return in
}

func TestRunNilSchemaKeepsInputJSONStringsLiteral(t *testing.T) {
	f := newFake(t)
	in := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(in, []byte(`{"text":"42"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", "google/gemini-3-8-flash-tts", "--input", in, "--no-wait")
	if got := createInput(t, f)["text"]; r.code != 0 || got != "42" {
		t.Fatalf("code=%d text=%#v out=%s", r.code, got, r.out)
	}
}

func TestRunNilSchemaKeepsFileContentLiteral(t *testing.T) {
	f := newFake(t)
	txt := filepath.Join(t.TempDir(), "t.txt")
	if err := os.WriteFile(txt, []byte("[1,2]"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", "google/gemini-3-8-flash-tts", "-p", "text=@"+txt, "--no-wait")
	if got := createInput(t, f)["text"]; r.code != 0 || got != "[1,2]" {
		t.Fatalf("code=%d text=%#v out=%s", r.code, got, r.out)
	}
}

func TestRunNilSchemaStillGuessesPlainParams(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "google/gemini-3-8-flash-tts", "-p", "n=5", "-p", "text=hola", "--no-wait")
	in := createInput(t, f)
	if r.code != 0 || in["n"] != 5.0 || in["text"] != "hola" {
		t.Fatalf("code=%d in=%#v out=%s", r.code, in, r.out)
	}
}

func TestRunSchemaDeclaredStringKeepsFormat(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=1.50", "--no-wait")
	if got := createInput(t, f)["prompt"]; r.code != 0 || got != "1.50" {
		t.Fatalf("code=%d prompt=%#v out=%s", r.code, got, r.out)
	}
}

func TestRunDryRunPendingPathIsAsGiven(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "a.png"), pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFake(t)
	r := f.run(t, "run", seedance, "-p", "prompt=x", "--dry-run", "--no-validate", "-p", "first_image=~/a.png")
	pend, _ := r.m["pendingUploads"].([]any)
	if r.code != 0 || len(pend) != 1 || pend[0].(map[string]any)["path"] != "~/a.png" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}
