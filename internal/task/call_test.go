package task_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/mik3dev/kie-cli/internal/task"
)

func TestCallReturnsData(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":{"audioId":"au1"}}`))
	data, err := task.Call(context.Background(), c, http.MethodPost, "/api/v1/omni/audio/create", map[string]any{"audio_id": "kore"})
	if err != nil || string(data) != `{"audioId":"au1"}` {
		t.Fatalf("data=%s err=%v", data, err)
	}
}

func TestCallMapsCredits(t *testing.T) {
	c := serve(t, reply(200, `{"code":402,"msg":"insufficient credits"}`))
	_, err := task.Call(context.Background(), c, http.MethodPost, "/x", nil)
	if oe := asOutErr(t, err); oe.Code != "credits" || oe.ExitCode() != 4 {
		t.Fatalf("err = %v", err)
	}
}

func TestCallAmbiguousIsSubmitUncertain(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	})
	_, err := task.Call(context.Background(), c, http.MethodPost, "/x", map[string]any{})
	if oe := asOutErr(t, err); oe.Code != "submit_uncertain" || oe.ExitCode() != 8 {
		t.Fatalf("err = %v", err)
	}
}

func TestCall5xxAndMalformedAreUncertain(t *testing.T) {
	for _, body := range []string{`{"code":500,"msg":"boom"}`, `not json`} {
		c := serve(t, reply(200, body))
		_, err := task.Call(context.Background(), c, http.MethodPost, "/x", nil)
		if oe := asOutErr(t, err); oe.Code != "submit_uncertain" {
			t.Fatalf("%s: err = %v", body, err)
		}
	}
}

func TestCallNullDataIsEmptyNotError(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":null}`))
	data, err := task.Call(context.Background(), c, http.MethodPost, "/x", nil)
	if err != nil || len(data) != 0 {
		t.Fatalf("data=%q err=%v", data, err)
	}
}
