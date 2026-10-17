package api_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
)

func newClient(t *testing.T, url string) (*api.Client, *[]time.Duration) {
	t.Helper()
	c := api.New("k-secret", func(string) string { return "" })
	c.BaseURL, c.UploadURL = url, url
	var sleeps []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	return c, &sleeps
}

func TestDoDecodesData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":200,"msg":"ok","data":{"taskId":"t1"}}`))
	}))
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	var out struct {
		TaskID string `json:"taskId"`
	}
	if err := c.Do(context.Background(), api.Request{Method: "GET", Path: "/x"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.TaskID != "t1" {
		t.Fatalf("out = %+v", out)
	}
}

func TestHTTP200WithErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":402,"msg":"insufficient"}`))
	}))
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: map[string]any{}}, nil)
	ae, ok := err.(*api.Error)
	if !ok {
		t.Fatalf("err = %T %v", err, err)
	}
	oe := ae.AsOutput()
	if oe.Code != "credits" || oe.ExitCode() != 4 {
		t.Fatalf("out = %+v exit %d", oe, oe.ExitCode())
	}
	if oe.Details["kie_code"] != 402 || oe.Details["kie_msg"] != "insufficient" {
		t.Fatalf("details = %v", oe.Details)
	}
}

func TestAsOutputMapping(t *testing.T) {
	cases := map[int]string{401: "auth", 402: "credits", 422: "api", 400: "api", 404: "api",
		429: "rate_limit", 455: "rate_limit", 433: "rate_limit", 500: "api", 503: "api"}
	for kc, want := range cases {
		if got := (&api.Error{KieCode: kc}).AsOutput().Code; got != want {
			t.Errorf("%d: got %s want %s", kc, got, want)
		}
	}
	var _ = output.CodeAPI
}

func TestAuthHeaderAndNoKeyLeak(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(401)
		w.Write([]byte(`{"code":401,"msg":"bad key k-secret"}`))
	}))
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	var dbg bytes.Buffer
	c.Debug = &dbg
	err := c.Do(context.Background(), api.Request{Method: "GET", Path: "/x"}, nil)
	if gotAuth != "Bearer k-secret" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if err == nil || strings.Contains(err.Error(), "k-secret") || strings.Contains(dbg.String(), "k-secret") {
		t.Fatalf("leak: err=%v dbg=%q", err, dbg.String())
	}
	if !strings.Contains(dbg.String(), "Bearer ****") || !strings.Contains(dbg.String(), "401") {
		t.Fatalf("debug = %q", dbg.String())
	}
	oe := err.(*api.Error).AsOutput()
	if oe.Code != "auth" || strings.Contains(oe.Message, "k-secret") {
		t.Fatalf("out = %+v", oe)
	}
}

func seqServer(t *testing.T, hits *atomic.Int32, bodies ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(hits.Add(1)) - 1
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		w.Write([]byte(bodies[i]))
	}))
}

func TestGetRetries429ThenOK(t *testing.T) {
	var hits atomic.Int32
	srv := seqServer(t, &hits, `{"code":429,"msg":"slow"}`, `{"code":429,"msg":"slow"}`, `{"code":200,"data":{}}`)
	defer srv.Close()
	c, sleeps := newClient(t, srv.URL)
	if err := c.Do(context.Background(), api.Request{Method: "GET", Path: "/x"}, nil); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d", hits.Load())
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != time.Second || (*sleeps)[1] != 2*time.Second {
		t.Fatalf("sleeps = %v", *sleeps)
	}
}

func TestRetryLimitIsThree(t *testing.T) {
	var hits atomic.Int32
	srv := seqServer(t, &hits, `{"code":429,"msg":"slow"}`)
	defer srv.Close()
	c, sleeps := newClient(t, srv.URL)
	err := c.Do(context.Background(), api.Request{Method: "GET", Path: "/x"}, nil)
	if err == nil || hits.Load() != 4 || len(*sleeps) != 3 || (*sleeps)[2] != 4*time.Second {
		t.Fatalf("err=%v hits=%d sleeps=%v", err, hits.Load(), *sleeps)
	}
}

func TestPostCreateNoRetryOn500(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(500)
		w.Write([]byte(`{"code":500,"msg":"boom"}`))
	}))
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: map[string]any{"a": 1}, Class: api.ClassCreate}, nil)
	ae, ok := err.(*api.Error)
	if !ok || ae.KieCode != 500 || hits.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
}

func TestPostNoRetryOn455(t *testing.T) {
	var hits atomic.Int32
	srv := seqServer(t, &hits, `{"code":455,"msg":"maintenance"}`, `{"code":200}`)
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: 1}, nil)
	if err == nil || hits.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
}

func TestPostCreateRetries429(t *testing.T) {
	var hits atomic.Int32
	srv := seqServer(t, &hits, `{"code":429,"msg":"slow"}`, `{"code":200,"data":{}}`)
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	if err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestDialErrorIsNotSent(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	c, sleeps := newClient(t, "http://"+addr)
	err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: 1}, nil)
	ne, ok := err.(*api.NetError)
	if !ok || ne.Sent {
		t.Fatalf("err = %T %v", err, err)
	}
	if len(*sleeps) != 3 {
		t.Fatalf("not-sent POST should retry 3 times, sleeps=%v", *sleeps)
	}
}

func TestServerHangupAfterBodyIsSent(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		buf := make([]byte, 1024)
		r.Body.Read(buf)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer srv.Close()
	c, sleeps := newClient(t, srv.URL)
	err := c.Do(context.Background(), api.Request{Method: "POST", Path: "/x", Body: map[string]any{"a": 1}}, nil)
	ne, ok := err.(*api.NetError)
	if !ok || !ne.Sent {
		t.Fatalf("err = %T %v", err, err)
	}
	if hits.Load() != 1 || len(*sleeps) != 0 {
		t.Fatalf("POST sent must not retry: hits=%d sleeps=%v", hits.Load(), *sleeps)
	}
}

func TestUploadHostAndNon2xxNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()
	c := api.New("k", func(n string) string {
		if n == "KIE_UPLOAD_URL" {
			return srv.URL + "/"
		}
		return ""
	})
	err := c.Do(context.Background(), api.Request{Method: "GET", Path: "/u", Host: api.HostUpload}, nil)
	ae, ok := err.(*api.Error)
	if !ok || ae.KieCode != 404 {
		t.Fatalf("err = %T %v", err, err)
	}
}

func TestContextCancelStopsRetry(t *testing.T) {
	var hits atomic.Int32
	srv := seqServer(t, &hits, `{"code":429}`)
	defer srv.Close()
	c, _ := newClient(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	c.Sleep = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	err := c.Do(ctx, api.Request{Method: "GET", Path: "/x"}, nil)
	if err == nil || hits.Load() != 1 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
}
