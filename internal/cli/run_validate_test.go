package cli_test

import (
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
