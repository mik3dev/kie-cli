package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func omniBody(t *testing.T, f *fakeKie) map[string]any {
	t.Helper()
	b, _ := f.omniBody.Load().(map[string]any)
	if b == nil {
		t.Fatal("no omni request recorded")
	}
	return b
}

func omniHits(f *fakeKie) int32 { return f.audioHits.Load() + f.characterHits.Load() }

func TestRunChatModelIsUsageError(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "claude-sonnet-4-6", "-p", "x=y")
	if r.code != 2 || errField(t, r.m, "code") != "usage" ||
		!strings.Contains(r.out, "chat models are out of scope for kie-cli; the agent is the LLM") {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.createHits.Load() != 0 || omniHits(f) != 0 {
		t.Error("no POST may be sent for a chat model")
	}
}

func TestRunSyncOmniAudio(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "gemini-omni-audio", "-p", "audio_id=kore", "-p", "name=voz", "--out", t.TempDir())
	if r.code != 0 || r.m["ok"] != true || r.m["sync"] != true || r.m["state"] != "success" || r.m["model"] != "gemini-omni-audio" {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	if _, has := r.m["taskId"]; has {
		t.Error("sync output must not carry taskId")
	}
	ids, _ := json.Marshal(r.m["ids"])
	if string(ids) != `{"audio_ids":["au1"]}` {
		t.Errorf("ids = %s", ids)
	}
	if files, _ := r.m["files"].([]any); files == nil || len(files) != 0 {
		t.Errorf("files = %v", r.m["files"])
	}
	body := omniBody(t, f)
	if _, has := body["model"]; has || body["audio_id"] != "kore" || body["name"] != "voz" {
		t.Errorf("body = %v", body)
	}
	if _, has := body["input"]; has {
		t.Errorf("body must not be wrapped: %v", body)
	}
	if f.createHits.Load() != 0 || len(f.ledger(t)) != 0 {
		t.Error("sync must not use createTask nor write the ledger")
	}
}

func TestRunSyncCharacterDownloadsImages(t *testing.T) {
	f := newFake(t)
	out := t.TempDir()
	r := f.run(t, "run", "gemini-omni-character", "-p", "descriptions=silver hair",
		"-p", "image_urls=[\"https://files.example/p.png\"]", "--out", out)
	if r.code != 0 || r.m["sync"] != true {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	files, _ := r.m["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files = %v", r.m["files"])
	}
	f0, _ := files[0].(map[string]any)
	if f0["url"] != f.cdn.URL+"/c.png" {
		t.Errorf("file = %v", f0)
	}
	if p, _ := f0["path"].(string); p == "" || !strings.HasPrefix(p, out) {
		t.Errorf("path = %v", f0["path"])
	} else if _, err := os.Stat(p); err != nil {
		t.Error(err)
	}
	ids, _ := json.Marshal(r.m["ids"])
	if string(ids) != `{"character_ids":["ch1"]}` {
		t.Errorf("ids = %s", ids)
	}
}

func TestRunSyncCreditsError(t *testing.T) {
	f := newFake(t)
	f.omniReply = func(string) string { return `{"code":402,"msg":"no credits"}` }
	r := f.run(t, "run", "gemini-omni-audio", "-p", "audio_id=kore", "-p", "name=voz")
	if r.code != 4 || errField(t, r.m, "code") != "credits" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if _, err := os.Stat(filepath.Join(f.home, "tasks.jsonl")); err == nil && len(f.ledger(t)) != 0 {
		t.Error("ledger must stay empty")
	}
}

func TestRunSyncAmbiguousIsUncertainWithoutLedger(t *testing.T) {
	f := newFake(t)
	f.omniReply = func(string) string { return `{"code":500,"msg":"boom"}` }
	r := f.run(t, "run", "gemini-omni-audio", "-p", "audio_id=kore", "-p", "name=voz")
	if r.code != 8 || errField(t, r.m, "code") != "submit_uncertain" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if len(f.ledger(t)) != 0 {
		t.Error("ledger must stay empty")
	}
}

func TestRunSyncDryRun(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "gemini-omni-audio", "-p", "audio_id=kore", "-p", "name=voz", "--dry-run")
	if r.code != 0 || r.m["dryRun"] != true || r.m["path"] != "/api/v1/omni/audio/create" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	body, _ := r.m["body"].(map[string]any)
	if body["audio_id"] != "kore" || body["name"] != "voz" || body["input"] != nil {
		t.Errorf("body = %v", body)
	}
	if omniHits(f) != 0 {
		t.Error("dry-run must not hit the endpoint")
	}
}

func TestRunSyncValidationStillApplies(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "gemini-omni-audio", "-p", "name=voz")
	if r.code != 2 || errField(t, r.m, "code") != "validation" || omniHits(f) != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunSyncNoWaitWarns(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "gemini-omni-audio", "-p", "audio_id=kore", "-p", "name=voz", "--no-wait")
	if r.code != 0 || !strings.Contains(r.stderr, "warning") || omniHits(f) != 1 {
		t.Fatalf("code=%d stderr=%s hits=%d", r.code, r.stderr, omniHits(f))
	}
}

func TestRunSyncNoDownloadKeepsResult(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "gemini-omni-character", "-p", "descriptions=d",
		"-p", "image_urls=[\"https://files.example/p.png\"]", "--no-download")
	files, _ := r.m["files"].([]any)
	if r.code != 0 || files == nil || len(files) != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunSyncDownloadFailureKeepsResultUrls(t *testing.T) {
	f := newFake(t)
	f.cdnStatus = 500
	r := f.run(t, "run", "gemini-omni-character", "-p", "descriptions=d",
		"-p", "image_urls=[\"https://files.example/p.png\"]", "--out", t.TempDir())
	if r.code == 0 || errField(t, r.m, "code") != "io" || errField(t, r.m, "resultUrls") == nil || errField(t, r.m, "result") == nil {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunSegmentMapDownloadsMasks(t *testing.T) {
	f := newFake(t)
	f.recordReply = func(int32) string {
		rj, _ := json.Marshal(map[string]any{"resultObject": map[string]any{
			"segments": []any{map[string]any{"maskUrl": f.cdn.URL + "/0.png", "index": 0}},
		}})
		return envelope(map[string]any{
			"taskId": "task-1", "model": "grok-imagine-image-2-0/segment-map", "state": "success",
			"resultJson": string(rj), "failCode": nil,
		})
	}
	r := f.run(t, "run", "grok-imagine-image-2-0/segment-map", "-p", "task_id=t0", "--out", t.TempDir())
	if r.code != 0 {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	files, _ := r.m["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("files = %v", r.m["files"])
	}
	if f0, _ := files[0].(map[string]any); f0["url"] != f.cdn.URL+"/0.png" {
		t.Errorf("file = %v", f0)
	}
	if r.m["taskId"] != "task-1" {
		t.Errorf("taskId = %v", r.m["taskId"])
	}
}
