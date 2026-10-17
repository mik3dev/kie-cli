package task_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/task"
)

func stateBody(state string) string {
	return fmt.Sprintf(`{"code":200,"msg":"ok","data":{"taskId":"t1","state":%q}}`, state)
}

func newWaiter(progress io.Writer) (task.Waiter, *[]time.Duration) {
	fc := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	var sleeps []time.Duration
	return task.Waiter{
		Now: fc.now,
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return fc.sleep(ctx, d)
		},
		Progress: progress,
	}, &sleeps
}

func TestWaitWalksStates(t *testing.T) {
	seq := []string{"waiting", "queuing", "generating", "generating", "success"}
	var n atomic.Int32
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		i := int(n.Add(1)) - 1
		_, _ = io.WriteString(w, stateBody(seq[i]))
	})
	var progress bytes.Buffer
	w, sleeps := newWaiter(&progress)
	rec, err := w.Wait(context.Background(), c, "t1", 10*time.Minute)
	if err != nil || rec.State != "success" {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	lines := strings.Split(strings.TrimSpace(progress.String()), "\n")
	want := []string{"state: waiting", "state: queuing", "state: generating", "state: success"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("progress = %q", lines)
	}
	wantSleeps := []time.Duration{3 * time.Second, 4500 * time.Millisecond, 6750 * time.Millisecond, 10125 * time.Millisecond}
	if fmt.Sprint(*sleeps) != fmt.Sprint(wantSleeps) {
		t.Errorf("sleeps = %v, want %v", *sleeps, wantSleeps)
	}
}

func TestWaitBackoffCapsAt15s(t *testing.T) {
	var n atomic.Int32
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		state := "generating"
		if n.Add(1) == 9 {
			state = "success"
		}
		_, _ = io.WriteString(w, stateBody(state))
	})
	w, sleeps := newWaiter(nil)
	if _, err := w.Wait(context.Background(), c, "t1", time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, d := range *sleeps {
		if d > 15*time.Second {
			t.Fatalf("sleep %v exceeds cap", d)
		}
	}
	if last := (*sleeps)[len(*sleeps)-1]; last != 15*time.Second {
		t.Errorf("last sleep = %v, want 15s", last)
	}
}

func TestWaitFail(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":{"taskId":"t1","state":"fail","failCode":501,"failMsg":"content policy"}}`))
	w, _ := newWaiter(nil)
	rec, err := w.Wait(context.Background(), c, "t1", time.Minute)
	oe := asOutErr(t, err)
	if oe.Code != output.CodeTaskFailed || oe.ExitCode() != 5 {
		t.Fatalf("err = %v", err)
	}
	if oe.Details["failMsg"] != "content policy" || oe.Details["failCode"] != "501" {
		t.Errorf("details = %v", oe.Details)
	}
	if rec.State != "fail" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestWaitTimeout(t *testing.T) {
	c := serve(t, reply(200, stateBody("generating")))
	w, _ := newWaiter(nil)
	rec, err := w.Wait(context.Background(), c, "t1", time.Minute)
	oe := asOutErr(t, err)
	if oe.Code != output.CodeTimeout || oe.ExitCode() != 6 || oe.Details["taskId"] != "t1" {
		t.Fatalf("err = %v details=%v", err, oe.Details)
	}
	if rec.State != "generating" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestWaitContextCancel(t *testing.T) {
	c := serve(t, reply(200, stateBody("generating")))
	ctx, cancel := context.WithCancel(context.Background())
	w, _ := newWaiter(nil)
	w.Sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	rec, err := w.Wait(ctx, c, "t1", time.Hour)
	oe := asOutErr(t, err)
	if oe.Code != output.CodeTimeout || oe.Details["taskId"] != "t1" {
		t.Fatalf("err = %v", err)
	}
	if rec.State != "generating" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestWaitContextCancelledDuringGet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := serve(t, func(http.ResponseWriter, *http.Request) { cancel(); <-ctx.Done() })
	w, _ := newWaiter(nil)
	rec, err := w.Wait(ctx, c, "t1", time.Hour)
	if oe := asOutErr(t, err); oe.Code != output.CodeTimeout || oe.Details["taskId"] != "t1" {
		t.Fatalf("err = %v", err)
	}
	if rec.TaskID != "t1" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestWaitPropagatesAPIError(t *testing.T) {
	c := serve(t, reply(200, `{"code":422,"msg":"bad id"}`))
	w, _ := newWaiter(nil)
	if _, err := w.Wait(context.Background(), c, "t1", time.Minute); err == nil {
		t.Fatal("want error")
	}
}

func TestWaitDefaultsRealClock(t *testing.T) {
	c := serve(t, reply(200, stateBody("success")))
	if rec, err := (task.Waiter{}).Wait(context.Background(), c, "t1", time.Minute); err != nil || rec.State != "success" {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}
