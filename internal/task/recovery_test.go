package task_test

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/result"
	"github.com/mik3dev/kie-cli/internal/task"
)

func TestWaitRecovery201Then200(t *testing.T) {
	var n atomic.Int32
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/suno/recovery/record-info" || r.URL.Query().Get("task_id") != "r1" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if n.Add(1) <= 2 {
			_, _ = io.WriteString(w, `{"code":201,"msg":"running","data":null}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":[{"id":"a1","audio_url":"https://c/1.mp3"},{"id":"a2","audio_url":"https://c/2.mp3"}]}`)
	})
	w, _ := newWaiter(nil)
	rec, err := w.WaitRecovery(context.Background(), c, "r1", time.Hour)
	if err != nil || rec.State != "success" {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	ids := result.Extract("r1", rec.Response)
	if len(ids.AudioIDs) != 2 || ids.AudioIDs[0] != "a1" || ids.AudioIDs[1] != "a2" {
		t.Errorf("ids = %+v", ids)
	}
	if n.Load() != 3 {
		t.Errorf("requests = %d, want 3", n.Load())
	}
}

func TestWaitRecovery500IsTaskFailed(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":500,"msg":"all failed"}`)
	})
	w, _ := newWaiter(nil)
	rec, err := w.WaitRecovery(context.Background(), c, "r1", time.Hour)
	oe, ok := err.(*output.Error)
	if !ok || oe.Code != output.CodeTaskFailed || rec.State != "fail" {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestWaitRecoveryTimeout(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":201,"msg":"running","data":null}`)
	})
	w, _ := newWaiter(nil)
	_, err := w.WaitRecovery(context.Background(), c, "r1", 20*time.Second)
	oe, ok := err.(*output.Error)
	if !ok || oe.Code != output.CodeTimeout {
		t.Fatalf("err=%v", err)
	}
}

func TestGetRecoveryOtherErrorPropagates(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":401,"msg":"bad key"}`)
	})
	if _, err := task.GetRecovery(context.Background(), c, "r1"); err == nil {
		t.Fatal("want error")
	}
}
