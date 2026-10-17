package task

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
)

const (
	pollStart  = 3 * time.Second
	pollMax    = 15 * time.Second
	pollFactor = 1.5
)

// Waiter polls a task until it is terminal. Zero fields get real-time defaults.
type Waiter struct {
	Sleep    func(context.Context, time.Duration) error
	Now      func() time.Time
	Progress io.Writer
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Wait polls with 3s..15s backoff (x1.5). Failure returns the Record and task_failed;
// timeout or cancellation returns the last Record and timeout with the taskId.
func (w Waiter) Wait(ctx context.Context, c *api.Client, id string, timeout time.Duration) (Record, error) {
	sleep, now := w.Sleep, w.Now
	if sleep == nil {
		sleep = sleepCtx
	}
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(timeout)
	delay := pollStart
	last := Record{TaskID: id}
	prev := ""
	for {
		rec, err := Get(ctx, c, id)
		if err != nil {
			if ctx.Err() != nil {
				return last, timeoutErr(id)
			}
			return last, err
		}
		last = rec
		if rec.State != prev {
			prev = rec.State
			if w.Progress != nil {
				fmt.Fprintf(w.Progress, "state: %s\n", rec.State)
			}
		}
		switch rec.State {
		case stateSuccess:
			return rec, nil
		case stateFail:
			return rec, &output.Error{
				Code:    output.CodeTaskFailed,
				Message: fmt.Sprintf("task %s failed: %s", id, rec.FailMsg),
				Details: map[string]any{"failCode": rec.FailCode, "failMsg": rec.FailMsg},
			}
		}
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return rec, timeoutErr(id)
		}
		if err := sleep(ctx, min(delay, remaining)); err != nil {
			return rec, timeoutErr(id)
		}
		delay = min(time.Duration(float64(delay)*pollFactor), pollMax)
	}
}

func timeoutErr(id string) error {
	return &output.Error{
		Code:    output.CodeTimeout,
		Message: fmt.Sprintf("timed out waiting for task %s; resume with: kie-cli task wait %s", id, id),
		Details: map[string]any{"taskId": id},
	}
}
