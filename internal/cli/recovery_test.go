package cli_test

import (
	"reflect"
	"testing"

	"github.com/mik3dev/kie-cli/internal/task"
)

func TestRunRecoveryDownloadsTracks(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", task.RecoveryModel, "-p", "task_id=old1", "--out", t.TempDir())
	if r.code != 0 || r.m["state"] != "success" || r.m["taskId"] != "task-1" {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	if f.createHits.Load() != 1 || f.recordHits.Load() != 0 || f.recoveryHits.Load() != 2 {
		t.Errorf("hits create=%d record=%d recovery=%d", f.createHits.Load(), f.recordHits.Load(), f.recoveryHits.Load())
	}
	if files, _ := r.m["files"].([]any); len(files) != 2 {
		t.Errorf("files = %v", r.m["files"])
	}
	want := map[string]any{"task_id": "task-1", "audio_ids": []any{"a1", "a2"}}
	if !reflect.DeepEqual(r.m["ids"], want) {
		t.Errorf("ids = %v, want %v", r.m["ids"], want)
	}
}

func TestRunRecoveryFailed(t *testing.T) {
	f := newFake(t)
	f.recoveryReply = func(int32) string { return `{"code":500,"msg":"all failed"}` }
	r := f.run(t, "run", task.RecoveryModel, "-p", "task_id=old1", "--out", t.TempDir())
	if r.code != 5 || errField(t, r.m, "code") != "task_failed" || errField(t, r.m, "taskId") != "task-1" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.recordHits.Load() != 0 {
		t.Error("recordInfo must not be used")
	}
}

func TestTaskWaitRecoveryUsesLedgerModel(t *testing.T) {
	f := newFake(t)
	if sub := f.run(t, "run", task.RecoveryModel, "-p", "task_id=old1", "--no-wait", "--out", t.TempDir()); sub.code != 0 {
		t.Fatalf("submit: %s", sub.out)
	}
	r := f.run(t, "task", "wait", "task-1")
	if r.code != 0 || r.m["state"] != "success" || r.m["model"] != task.RecoveryModel {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.recordHits.Load() != 0 || f.recoveryHits.Load() == 0 {
		t.Errorf("record=%d recovery=%d", f.recordHits.Load(), f.recoveryHits.Load())
	}
}

func TestTaskGetRecoveryStates(t *testing.T) {
	cases := []struct{ name, reply, state string }{
		{"running", `{"code":201,"msg":"running","data":null}`, "generating"},
		{"done", envelope([]map[string]string{{"id": "a1", "audio_url": "https://c/1.mp3"}}), "success"},
		{"failed", `{"code":500,"msg":"boom"}`, "fail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			f.recoveryReply = func(int32) string { return tc.reply }
			if sub := f.run(t, "run", task.RecoveryModel, "-p", "task_id=old1", "--no-wait"); sub.code != 0 {
				t.Fatalf("submit: %s", sub.out)
			}
			r := f.run(t, "task", "get", "task-1")
			if r.code != 0 || r.m["state"] != tc.state || r.m["model"] != task.RecoveryModel {
				t.Fatalf("code=%d out=%s", r.code, r.out)
			}
			// a 500 envelope is retried by api.Client (GET policy), so only require >=1 hits.
			if f.recoveryHits.Load() < 1 || f.recordHits.Load() != 0 {
				t.Errorf("recovery=%d record=%d", f.recoveryHits.Load(), f.recordHits.Load())
			}
		})
	}
}
