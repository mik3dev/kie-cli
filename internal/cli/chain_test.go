package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sunoModel = "ai-music-api/generate"

// sunoReply makes recordInfo answer with a finished two-track Suno task.
func sunoReply(f *fakeKie) func(int32) string {
	return func(int32) string {
		rj, _ := json.Marshal(map[string]any{"data": []map[string]any{
			{"id": "a1", "audio_url": f.cdn.URL + "/1.mp3"},
			{"id": "a2", "audio_url": f.cdn.URL + "/2.mp3"},
		}})
		return envelope(map[string]any{
			"taskId": "task-1", "model": sunoModel, "state": "success", "resultJson": string(rj),
			"creditsConsumed": 10, "costTime": 30,
		})
	}
}

func sunoRunArgs(out string) []string {
	return []string{"run", sunoModel, "-p", "prompt=x", "-p", "custom_mode=false",
		"-p", "instrumental=true", "-p", "model=V5", "--out", out}
}

func idsOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	ids, ok := m["ids"].(map[string]any)
	if !ok {
		t.Fatalf("ids missing or not an object: %v", m)
	}
	return ids
}

func TestRunImageIdsOnlyTaskID(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "aspect_ratio=1:1", "-p", "prompt=x", "--out", t.TempDir())
	if r.code != 0 || !reflect.DeepEqual(idsOf(t, r.m), map[string]any{"task_id": "task-1"}) {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunNoWaitHasIds(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "aspect_ratio=1:1", "-p", "prompt=x", "--no-wait")
	if r.code != 0 || !reflect.DeepEqual(idsOf(t, r.m), map[string]any{"task_id": "task-1"}) {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestRunSunoTwoTracks(t *testing.T) {
	f := newFake(t)
	f.recordReply = sunoReply(f)
	out := t.TempDir()
	r := f.run(t, sunoRunArgs(out)...)
	if r.code != 0 {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	files, _ := r.m["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files = %v", r.m["files"])
	}
	p0 := files[0].(map[string]any)["path"].(string)
	p1 := files[1].(map[string]any)["path"].(string)
	if p0 == p1 || !strings.HasSuffix(strings.TrimSuffix(p0, filepath.Ext(p0)), "_0") ||
		!strings.HasSuffix(strings.TrimSuffix(p1, filepath.Ext(p1)), "_1") {
		t.Errorf("paths not distinct _0/_1: %s %s", p0, p1)
	}
	if filepath.Ext(p0) != ".mp3" {
		t.Errorf("ext = %s, want .mp3", filepath.Ext(p0))
	}
	for _, p := range []string{p0, p1} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing file: %v", err)
		}
	}
	want := map[string]any{"task_id": "task-1", "audio_ids": []any{"a1", "a2"}}
	if got := idsOf(t, r.m); !reflect.DeepEqual(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func TestTaskGetIds(t *testing.T) {
	f := newFake(t)
	pending := func(int32) string {
		return envelope(map[string]any{"taskId": "task-1", "model": sunoModel, "state": "generating", "resultJson": ""})
	}
	f.recordReply = pending
	r := f.run(t, "task", "get", "task-1")
	if r.code != 0 || !reflect.DeepEqual(idsOf(t, r.m), map[string]any{"task_id": "task-1"}) {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	f.recordReply = sunoReply(f)
	r = f.run(t, "task", "get", "task-1")
	if r.code != 0 || !reflect.DeepEqual(idsOf(t, r.m)["audio_ids"], []any{"a1", "a2"}) {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestTaskWaitIds(t *testing.T) {
	f := newFake(t)
	f.recordReply = sunoReply(f)
	r := f.run(t, "task", "wait", "task-1", "--out", t.TempDir())
	if r.code != 0 || !reflect.DeepEqual(idsOf(t, r.m)["audio_ids"], []any{"a1", "a2"}) {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
}

func dryBody(t *testing.T, r cliResult) map[string]any {
	t.Helper()
	body, _ := r.m["body"].(map[string]any)
	in, _ := body["input"].(map[string]any)
	if r.code != 0 || in == nil {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	return in
}

func TestChainSunoExtendFromIds(t *testing.T) {
	f := newFake(t)
	f.recordReply = sunoReply(f)
	r := f.run(t, sunoRunArgs(t.TempDir())...)
	if r.code != 0 {
		t.Fatalf("generate: %s", r.out)
	}
	audioIDs, _ := idsOf(t, r.m)["audio_ids"].([]any)
	if len(audioIDs) == 0 {
		t.Fatalf("no audio_ids: %s", r.out)
	}
	ext := f.run(t, "run", "ai-music-api/extend", "-p", "audio_id="+audioIDs[0].(string), "-p", "model=V5", "--dry-run")
	if in := dryBody(t, ext); in["audio_id"] != "a1" {
		t.Errorf("input = %v", in)
	}
}

func TestChainVeoExtendFromIds(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "aspect_ratio=1:1", "-p", "prompt=x", "--out", t.TempDir())
	if r.code != 0 {
		t.Fatalf("image: %s", r.out)
	}
	id, _ := idsOf(t, r.m)["task_id"].(string)
	ext := f.run(t, "run", "veo/extend", "-p", "task_id="+id, "-p", "prompt=more", "--dry-run")
	if in := dryBody(t, ext); in["task_id"] != "task-1" {
		t.Errorf("input = %v", in)
	}
}
