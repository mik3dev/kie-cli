package task_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/ledger"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/task"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }
func (f *fakeClock) sleep(_ context.Context, d time.Duration) error {
	f.t = f.t.Add(d)
	return nil
}

func newClient(t *testing.T, url string) *api.Client {
	t.Helper()
	c := api.New("secret-key-123", func(k string) string {
		if k == "KIE_BASE_URL" {
			return url
		}
		return ""
	})
	fc := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	c.Now, c.Sleep = fc.now, fc.sleep
	return c
}

func serve(t *testing.T, h http.HandlerFunc) *api.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return newClient(t, srv.URL)
}

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func ledgerPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "home", "tasks.jsonl")
}

func entries(t *testing.T, p string) []ledger.Entry {
	t.Helper()
	es, err := ledger.List(p, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func asOutErr(t *testing.T, err error) *output.Error {
	t.Helper()
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("err = %v (%T), want *output.Error", err, err)
	}
	return oe
}

var submitNow = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

func TestSubmitOK(t *testing.T) {
	var gotBody string
	var gotPath string
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotPath = string(b), r.URL.Path
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":{"taskId":"t1"}}`)
	})
	lp := ledgerPath(t)
	id, err := task.Submit(context.Background(), c, lp, "m", map[string]any{"prompt": "x"}, "out", submitNow)
	if err != nil || id != "t1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if gotPath != "/api/v1/jobs/createTask" || gotBody != `{"input":{"prompt":"x"},"model":"m"}` {
		t.Errorf("path=%s body=%s", gotPath, gotBody)
	}
	if strings.Contains(gotBody, "callBackUrl") {
		t.Error("callBackUrl must not be sent")
	}
	es := entries(t, lp)
	if len(es) != 1 || es[0].State != "submitted" || es[0].TaskID != "t1" || es[0].OutDir != "out" {
		t.Fatalf("ledger = %+v", es)
	}
}

func TestSubmitLedgerFailureStillReturnsTaskID(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":{"taskId":"t9"}}`))
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	id, err := task.Submit(context.Background(), c, filepath.Join(blocker, "tasks.jsonl"), "m", nil, "", submitNow, &warn)
	if err != nil || id != "t9" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if !strings.Contains(warn.String(), "t9") || !strings.Contains(warn.String(), "warning") {
		t.Errorf("warn = %q", warn.String())
	}
}

func TestSubmitNoLedgerForRejections(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		code       string
		exit       int
	}{
		{"402 envelope", `{"code":402,"msg":"insufficient"}`, 200, output.CodeCredits, 4},
		{"401", `{"code":401,"msg":"unauthorized"}`, 200, output.CodeAuth, 3},
		{"422", `{"code":422,"msg":"bad input"}`, 200, output.CodeAPI, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := serve(t, reply(tc.status, tc.body))
			lp := ledgerPath(t)
			_, err := task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
			oe := asOutErr(t, err)
			if oe.Code != tc.code || oe.ExitCode() != tc.exit {
				t.Errorf("code=%s exit=%d", oe.Code, oe.ExitCode())
			}
			if es := entries(t, lp); len(es) != 0 {
				t.Errorf("ledger = %+v, want empty", es)
			}
		})
	}
}

func TestSubmit402NoLedger(t *testing.T) {
	c := serve(t, reply(402, `{"code":402,"msg":"no credits"}`))
	lp := ledgerPath(t)
	_, err := task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
	if oe := asOutErr(t, err); oe.ExitCode() != 4 {
		t.Errorf("exit = %d", oe.ExitCode())
	}
	if es := entries(t, lp); len(es) != 0 {
		t.Errorf("ledger = %+v", es)
	}
}

func TestSubmitUncertainCases(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500":             reply(500, `{"code":500,"msg":"boom"}`),
		"envelope 455":    reply(200, `{"code":455,"msg":"maintenance"}`),
		"envelope 500":    reply(200, `{"code":500,"msg":"boom"}`),
		"bad envelope":    reply(200, `not json`),
		"no code":         reply(200, `{"msg":"x"}`),
		"bad data shape":  reply(200, `{"code":200,"msg":"ok","data":"oops"}`),
		"missing data":    reply(200, `{"code":200,"msg":"ok"}`),
		"empty taskId":    reply(200, `{"code":200,"msg":"ok","data":{"taskId":""}}`),
		"no taskId field": reply(200, `{"code":200,"msg":"ok","data":{}}`),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			c := serve(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); h(w, r) })
			lp := ledgerPath(t)
			_, err := task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
			oe := asOutErr(t, err)
			if oe.Code != output.CodeSubmitUncertain || oe.ExitCode() != 8 {
				t.Fatalf("code=%s exit=%d", oe.Code, oe.ExitCode())
			}
			if !strings.Contains(oe.Message, "https://kie.ai/logs") {
				t.Errorf("msg = %q", oe.Message)
			}
			if calls.Load() != 1 {
				t.Errorf("createTask called %d times, want 1 (never retry)", calls.Load())
			}
			es := entries(t, lp)
			if len(es) != 1 || es[0].State != "unknown" || es[0].TaskID != "" || es[0].Note == "" {
				t.Fatalf("ledger = %+v", es)
			}
		})
	}
}

func TestSubmit500IsUncertain(t *testing.T) {
	c := serve(t, reply(500, `{"code":500,"msg":"boom"}`))
	lp := ledgerPath(t)
	_, err := task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
	if oe := asOutErr(t, err); oe.Code != "submit_uncertain" || oe.ExitCode() != 8 {
		t.Fatalf("err = %v", err)
	}
	if es := entries(t, lp); len(es) != 1 || es[0].State != "unknown" {
		t.Fatalf("ledger = %+v", es)
	}
}

func TestSubmitSentNetErrorIsUncertain(t *testing.T) {
	// Server accepts the request then drops the connection without answering.
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	})
	lp := ledgerPath(t)
	_, err := task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
	if oe := asOutErr(t, err); oe.Code != output.CodeSubmitUncertain {
		t.Fatalf("err = %v", err)
	}
	if es := entries(t, lp); len(es) != 1 || es[0].State != "unknown" {
		t.Fatalf("ledger = %+v", es)
	}
}

func TestSubmitDialErrorRetriesThenFailsNotUncertain(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	c := newClient(t, "http://"+addr)
	lp := ledgerPath(t)
	_, err = task.Submit(context.Background(), c, lp, "m", nil, "", submitNow)
	oe := asOutErr(t, err)
	if oe.Code != output.CodeAPI {
		t.Fatalf("code = %s, want api", oe.Code)
	}
	if es := entries(t, lp); len(es) != 0 {
		t.Errorf("ledger = %+v, want empty", es)
	}
}

func TestGetParsesResultJSONWhenResponseMissing(t *testing.T) {
	var gotQuery string
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":{"taskId":"t1","model":"m","state":"success","resultJson":"{\"resultUrls\":[\"http://x/a.png\"]}","creditsConsumed":1.5,"costTime":12}}`)
	})
	rec, err := task.Get(context.Background(), c, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "taskId=t1" {
		t.Errorf("query = %s", gotQuery)
	}
	var resp struct {
		ResultUrls []string `json:"resultUrls"`
	}
	if err := json.Unmarshal(rec.Response, &resp); err != nil || len(resp.ResultUrls) != 1 {
		t.Fatalf("response = %s err=%v", rec.Response, err)
	}
	if rec.CreditsConsumed != 1.5 || rec.CostTime != 12 || rec.State != "success" {
		t.Errorf("rec = %+v", rec)
	}
}

func TestGetKeepsExistingResponse(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":{"taskId":"t1","state":"success","resultJson":"{\"a\":1}","response":{"b":2}}}`))
	rec, err := task.Get(context.Background(), c, "t1")
	if err != nil || string(rec.Response) != `{"b":2}` {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestGetNullResponseAndBadResultJSON(t *testing.T) {
	c := serve(t, reply(200, `{"code":200,"msg":"ok","data":{"taskId":"t1","state":"generating","response":null,"resultJson":"not json"}}`))
	rec, err := task.Get(context.Background(), c, "t1")
	if err != nil || rec.Response != nil {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestGetError(t *testing.T) {
	c := serve(t, reply(200, `{"code":422,"msg":"bad id"}`))
	if _, err := task.Get(context.Background(), c, "x"); err == nil {
		t.Fatal("want error")
	}
}

func TestFailCodeNumberOrString(t *testing.T) {
	cases := map[string]string{
		`{"failCode":500}`:     "500",
		`{"failCode":"E_BAD"}`: "E_BAD",
		`{"failCode":null}`:    "",
		`{}`:                   "",
	}
	for in, want := range cases {
		var r task.Record
		if err := json.Unmarshal([]byte(in), &r); err != nil || r.FailCode != want {
			t.Errorf("%s -> %q err=%v, want %q", in, r.FailCode, err, want)
		}
	}
	var r task.Record
	if err := json.Unmarshal([]byte(`[`), &r); err == nil {
		t.Error("want syntax error")
	}
}

func TestTerminal(t *testing.T) {
	for s, want := range map[string]bool{"success": true, "fail": true, "waiting": false, "generating": false, "": false} {
		if task.Terminal(s) != want {
			t.Errorf("Terminal(%q) != %v", s, want)
		}
	}
}
