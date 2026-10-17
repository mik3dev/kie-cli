package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
	"github.com/mik3dev/kie-cli/internal/ledger"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\nfake-image-bytes")

const imageModel = "google/imagen4"

// fakeKie combines the kie API, the upload host and a TLS CDN.
type fakeKie struct {
	api, cdn *httptest.Server
	home     string

	recordHits, createHits, uploadHits atomic.Int32
	createBody                         atomic.Value // map[string]any
	// behaviour knobs
	createReply func() string // full envelope JSON; "" = success
	recordReply func(n int32) string
	cdnStatus   int
}

func envelope(data any) string {
	b, _ := json.Marshal(map[string]any{"code": 200, "msg": "ok", "data": data})
	return string(b)
}

func newFake(t *testing.T) *fakeKie {
	t.Helper()
	f := &fakeKie{home: filepath.Join(t.TempDir(), "home"), cdnStatus: 200}
	f.cdn = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.cdnStatus != 200 {
			http.Error(w, "gone", f.cdnStatus)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	t.Cleanup(f.cdn.Close)
	f.api = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.api.Close)
	return f
}

func (f *fakeKie) serve(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	reply := func(s string) { _, _ = io.WriteString(w, s) }
	switch {
	case p == "/api/v1/models":
		reply(envelope(readLive2(r, "models.json")))
	case strings.HasSuffix(p, "/schema"):
		model := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/models/"), "/schema")
		reply(envelope(readLive2(r, "schemas/"+strings.ReplaceAll(model, "/", "__")+".json")))
	case p == "/api/file-stream-upload":
		f.uploadHits.Add(1)
		reply(envelope(map[string]string{"downloadUrl": "https://files.example/uploaded.png"}))
	case p == "/api/v1/jobs/createTask":
		f.createHits.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.createBody.Store(body)
		if f.createReply != nil {
			reply(f.createReply())
			return
		}
		reply(envelope(map[string]string{"taskId": "task-1"}))
	case p == "/api/v1/jobs/recordInfo":
		n := f.recordHits.Add(1)
		if f.recordReply != nil {
			reply(f.recordReply(n))
			return
		}
		if n == 1 {
			reply(f.recordInfo("waiting"))
			return
		}
		reply(f.recordInfo("success"))
	case p == "/api/v1/common/download-url":
		reply(`{"code":404,"msg":"no such file"}`)
	default:
		reply(`{"code":404,"msg":"nf"}`)
	}
}

// readLive2 reads a fixture inside a handler goroutine (no *testing.T needed).
func readLive2(_ *http.Request, rel string) json.RawMessage {
	b, err := os.ReadFile("../../testdata/live/" + rel)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}

func (f *fakeKie) recordInfo(state string) string {
	rj, _ := json.Marshal(map[string]any{"resultUrls": []string{f.cdn.URL + "/out.png"}})
	return envelope(map[string]any{
		"taskId": "task-1", "model": imageModel, "state": state, "resultJson": string(rj),
		"creditsConsumed": 2, "costTime": 5, "failCode": nil,
	})
}

type cliResult struct {
	code        int
	out, stderr string
	m           map[string]any
}

func (f *fakeKie) run(t *testing.T, args ...string) cliResult {
	t.Helper()
	return f.runWith(t, nil, args...)
}

func (f *fakeKie) runWith(t *testing.T, mod func(*cli.Deps), args ...string) cliResult {
	t.Helper()
	var out, errw bytes.Buffer
	env := map[string]string{
		"KIE_API_KEY": "k-secret-123456", "KIE_BASE_URL": f.api.URL, "KIE_UPLOAD_URL": f.api.URL,
		"KIE_CLI_HOME": f.home,
	}
	deps := cli.Deps{
		Stdout: &out, Stderr: &errw, Stdin: strings.NewReader(""),
		Env: func(k string) string { return env[k] }, Now: time.Now,
		Transport: f.cdn.Client().Transport,
		Sleep:     func(_ context.Context, _ time.Duration) error { return nil },
	}
	if mod != nil {
		mod(&deps)
	}
	code := cli.Execute(deps, args)
	res := cliResult{code: code, out: out.String(), stderr: errw.String()}
	dec := json.NewDecoder(strings.NewReader(res.out))
	if err := dec.Decode(&res.m); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, res.out)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout must hold exactly one JSON value, got more: %q", res.out)
	}
	if strings.Contains(res.out+res.stderr, "k-secret-123456") {
		t.Fatal("API key leaked")
	}
	return res
}

func (f *fakeKie) ledger(t *testing.T) []ledger.Entry {
	t.Helper()
	es, err := ledger.List(filepath.Join(f.home, "tasks.jsonl"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func errField(t *testing.T, m map[string]any, key string) any {
	t.Helper()
	e, _ := m["error"].(map[string]any)
	if key == "code" || key == "message" {
		return e[key]
	}
	d, _ := e["details"].(map[string]any)
	return d[key]
}

func TestRunE2EImage(t *testing.T) {
	f := newFake(t)
	outDir := t.TempDir()
	r := f.run(t, "run", imageModel, "-p", "prompt=a cat", "-p", "num_images=2", "--out", outDir)
	if r.code != 0 || r.m["ok"] != true || r.m["taskId"] != "task-1" || r.m["state"] != "success" {
		t.Fatalf("code=%d out=%s stderr=%s", r.code, r.out, r.stderr)
	}
	files, _ := r.m["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["kind"] != "image" {
		t.Fatalf("files = %v", r.m["files"])
	}
	got, err := os.ReadFile(files[0].(map[string]any)["path"].(string))
	if err != nil || !bytes.Equal(got, pngBytes) {
		t.Fatalf("downloaded file mismatch: %v", err)
	}
	if entries, _ := os.ReadDir(outDir); len(entries) != 1 {
		t.Errorf("outDir has %d entries, want 1", len(entries))
	}
	body, _ := f.createBody.Load().(map[string]any)
	in, _ := body["input"].(map[string]any)
	if body["model"] != imageModel || in["prompt"] != "a cat" || in["num_images"] != 2.0 {
		t.Errorf("createTask body = %v", body)
	}
	if r.m["creditsConsumed"] != 2.0 || r.m["costTime"] != 5.0 || r.m["result"] == nil {
		t.Errorf("m = %v", r.m)
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "success" || es[0].TaskID != "task-1" {
		t.Errorf("ledger = %+v", es)
	}
	if !strings.Contains(r.stderr, "state:") {
		t.Errorf("progress missing from stderr: %q", r.stderr)
	}
}

func TestRunQuietSilencesStderr(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "prompt=x", "--out", t.TempDir(), "--quiet")
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("code=%d stderr=%q", r.code, r.stderr)
	}
}

func TestRunE2EUploadsLocalImage(t *testing.T) {
	f := newFake(t)
	img := filepath.Join(t.TempDir(), "in.png")
	if err := os.WriteFile(img, pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", imageModel, "-p", "image_urls="+img, "--out", t.TempDir())
	if r.code != 0 || f.uploadHits.Load() != 1 {
		t.Fatalf("code=%d uploads=%d out=%s", r.code, f.uploadHits.Load(), r.out)
	}
	body, _ := f.createBody.Load().(map[string]any)
	in, _ := body["input"].(map[string]any)
	urls, _ := in["image_urls"].(string)
	if urls != "https://files.example/uploaded.png" {
		t.Errorf("input = %v", in)
	}
	if up, _ := r.m["uploaded"].([]any); len(up) != 1 || up[0] != img {
		t.Errorf("uploaded = %v", r.m["uploaded"])
	}
}

func TestRunNoWait(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", imageModel, "-p", "prompt=x", "--no-wait")
	if r.code != 0 || r.m["state"] != "submitted" || r.m["taskId"] != "task-1" || r.m["model"] != imageModel {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.recordHits.Load() != 0 {
		t.Errorf("recordInfo called %d times with --no-wait", f.recordHits.Load())
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "submitted" {
		t.Errorf("ledger = %+v", es)
	}
}

func TestRunUnknownModel(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "run", "no/such-model")
	if r.code != 2 || errField(t, r.m, "code") != "validation" || !strings.Contains(r.out, "models search") {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if f.createHits.Load() != 0 {
		t.Error("createTask must not be called")
	}
}

func TestRunPresetAndSyncModelsAreUsageErrors(t *testing.T) {
	f := newFake(t)
	if r := f.run(t, "run", "@mypreset"); r.code != 2 || !strings.Contains(r.out, "stage 5") {
		t.Fatalf("preset: code=%d out=%s", r.code, r.out)
	}
	// gemini-3-pro is a Chat model: hidden from list but found by Find; non-job.
	if r := f.run(t, "run", "gemini-3-pro"); r.code != 2 || !strings.Contains(r.out, "stage 4") || errField(t, r.m, "code") != "usage" {
		t.Fatalf("chat: code=%d out=%s", r.code, r.out)
	}
	if f.createHits.Load() != 0 {
		t.Error("createTask must not be called")
	}
}

func TestRunCredits402(t *testing.T) {
	f := newFake(t)
	f.createReply = func() string { return `{"code":402,"msg":"insufficient credits"}` }
	r := f.run(t, "run", imageModel, "-p", "prompt=x")
	if r.code != 4 || errField(t, r.m, "code") != "credits" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if es := f.ledger(t); len(es) != 0 {
		t.Errorf("ledger must be empty, got %+v", es)
	}
}

func TestTaskWaitResumes(t *testing.T) {
	f := newFake(t)
	outDir := t.TempDir()
	sub := f.run(t, "run", imageModel, "-p", "prompt=x", "--no-wait", "--out", outDir)
	if sub.code != 0 {
		t.Fatalf("submit: %s", sub.out)
	}
	r := f.run(t, "task", "wait", "task-1")
	if r.code != 0 || r.m["state"] != "success" || r.m["model"] != imageModel {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	files, _ := r.m["files"].([]any)
	if len(files) != 1 || !strings.HasPrefix(files[0].(map[string]any)["path"].(string), outDir) {
		t.Fatalf("files = %v (want inside the ledger outDir %s)", r.m["files"], outDir)
	}
	es := f.ledger(t)
	if len(es) != 1 || es[0].State != "success" || es[0].Model != imageModel {
		t.Errorf("ledger = %+v", es)
	}
}

func TestTaskWaitNoDownload(t *testing.T) {
	f := newFake(t)
	r := f.run(t, "task", "wait", "task-1", "--no-download", "--out", t.TempDir())
	files, ok := r.m["files"].([]any)
	if r.code != 0 || !ok || len(files) != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func TestWaitTimeoutKeepsSubmittedAndReportsTaskID(t *testing.T) {
	f := newFake(t)
	f.recordReply = func(int32) string { return f.recordInfo("waiting") }
	slow := func(d *cli.Deps) {
		d.Sleep = func(_ context.Context, _ time.Duration) error { time.Sleep(5 * time.Millisecond); return nil }
	}
	r := f.runWith(t, slow, "run", imageModel, "-p", "prompt=x", "--timeout", "50ms")
	if r.code != 6 || errField(t, r.m, "code") != "timeout" || errField(t, r.m, "taskId") != "task-1" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "submitted" {
		t.Errorf("ledger = %+v", es)
	}
}

func TestWaitTaskFailedRecordsFailAndTaskID(t *testing.T) {
	f := newFake(t)
	f.recordReply = func(int32) string {
		return envelope(map[string]any{"taskId": "task-1", "model": imageModel, "state": "fail", "failCode": "500", "failMsg": "boom"})
	}
	r := f.run(t, "run", imageModel, "-p", "prompt=x")
	if r.code != 5 || errField(t, r.m, "taskId") != "task-1" || errField(t, r.m, "failMsg") != "boom" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "fail" {
		t.Errorf("ledger = %+v", es)
	}
}

func TestWaitPlainAPIErrorStillCarriesTaskID(t *testing.T) {
	f := newFake(t)
	f.recordReply = func(int32) string { return `{"code":422,"msg":"bad record"}` }
	r := f.run(t, "run", imageModel, "-p", "prompt=x")
	if r.code != 1 || errField(t, r.m, "code") != "api" || errField(t, r.m, "taskId") != "task-1" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if !strings.Contains(fmt.Sprint(errField(t, r.m, "message")), "bad record") {
		t.Errorf("message lost: %s", r.out)
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "submitted" {
		t.Errorf("ledger = %+v", es)
	}
}

func TestDownloadFailureReturnsIOWithTaskIDAndURLs(t *testing.T) {
	f := newFake(t)
	f.cdnStatus = 404
	r := f.run(t, "run", imageModel, "-p", "prompt=x", "--out", t.TempDir())
	if r.code != 1 || errField(t, r.m, "code") != "io" || errField(t, r.m, "taskId") != "task-1" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	urls, _ := errField(t, r.m, "resultUrls").([]any)
	if len(urls) != 1 || urls[0] != f.cdn.URL+"/out.png" {
		t.Errorf("resultUrls = %v", urls)
	}
	if es := f.ledger(t); len(es) != 1 || es[0].State != "success" {
		t.Errorf("ledger = %+v", es)
	}
}

func TestRunInputFileOverriddenByParams(t *testing.T) {
	f := newFake(t)
	in := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(in, []byte(`{"prompt":"from file","seed":7}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "run", imageModel, "--input", in, "-p", "prompt=override", "--no-wait")
	body, _ := f.createBody.Load().(map[string]any)
	got, _ := body["input"].(map[string]any)
	if r.code != 0 || got["prompt"] != "override" || got["seed"] != "7" { // imagen4 declares seed as string
		t.Fatalf("code=%d input=%v", r.code, got)
	}
}

func TestRunBadInputIsUsageError(t *testing.T) {
	f := newFake(t)
	for _, args := range [][]string{
		{"run", imageModel, "--input", "/no/such/file.json"},
		{"run", imageModel, "-p", "noequals"},
		{"run", imageModel, "--timeout", "-5s"},
	} {
		if r := f.run(t, args...); r.code != 2 {
			t.Errorf("%v: code=%d out=%s", args, r.code, r.out)
		}
	}
	if f.createHits.Load() != 0 {
		t.Error("createTask must not be called")
	}
}

func TestRunInputFromStdin(t *testing.T) {
	f := newFake(t)
	r := f.runWith(t, func(d *cli.Deps) { d.Stdin = strings.NewReader(`{"prompt":"stdin"}`) },
		"run", imageModel, "--input", "-", "--no-wait")
	body, _ := f.createBody.Load().(map[string]any)
	if got, _ := body["input"].(map[string]any); r.code != 0 || got["prompt"] != "stdin" {
		t.Fatalf("code=%d body=%v", r.code, body)
	}
}
