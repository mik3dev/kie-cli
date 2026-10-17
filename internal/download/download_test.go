package download_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/download"
	"github.com/mik3dev/kie-cli/internal/output"
)

type fake struct {
	srv *httptest.Server
	c   *api.Client
	mu  sync.Mutex
	// authOnGET records Authorization headers seen on non-API (CDN) requests.
	authOnGET []string
	dlCalls   int
}

func newFake(t *testing.T, cdn http.HandlerFunc) *fake {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/common/download-url", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.dlCalls++
		f.mu.Unlock()
		var in struct {
			URL string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": f.srv.URL + "/signed"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.authOnGET = append(f.authOnGET, r.Header.Get("Authorization"))
		f.mu.Unlock()
		cdn(w, r)
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	f.c = api.New("sk-secret-key", func(k string) string {
		if k == "KIE_BASE_URL" {
			return f.srv.URL
		}
		return ""
	})
	f.c.HTTP = f.srv.Client()
	return f
}

func TestDownloadNamesAndKind(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("vid"))
	})
	dir := filepath.Join(t.TempDir(), "out")
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/media/abc"}, dir, "kling-3.0/video", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0].Path) != "kling-3.0-video_t1_0.mp4" || files[0].Kind != "video" {
		t.Fatalf("got %+v", files)
	}
	if b, _ := os.ReadFile(files[0].Path); string(b) != "vid" {
		t.Fatalf("content %q", b)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o755 {
		t.Fatalf("dir perm %v", fi.Mode().Perm())
	}
}

func TestDownloadExtensionFromURLAndBinFallback(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-unknown-thing")
		_, _ = w.Write([]byte("x"))
	})
	files, err := download.All(context.Background(), f.c,
		[]string{f.srv.URL + "/a/pic.png?sig=1", f.srv.URL + "/a/noext"}, t.TempDir(), "m", "t")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(files[0].Path) != ".png" || files[0].Kind != "image" {
		t.Fatalf("0: %+v", files[0])
	}
	if filepath.Ext(files[1].Path) != ".bin" || files[1].Kind != "other" {
		t.Fatalf("1: %+v", files[1])
	}
}

func TestDownloadNeverOverwrites(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("new")) })
	dir := t.TempDir()
	orig := filepath.Join(dir, "m_t_0.png")
	if err := os.WriteFile(orig, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, dir, "m", "t")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(files[0].Path) != "m_t_0-1.png" {
		t.Fatalf("got %s", files[0].Path)
	}
	if b, _ := os.ReadFile(orig); string(b) != "orig" {
		t.Fatalf("original modified: %q", b)
	}
}

func TestDownloadFallbackDownloadURL(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signed" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Error(w, "no", http.StatusForbidden)
	})
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, t.TempDir(), "m", "t")
	if err != nil {
		t.Fatal(err)
	}
	if f.dlCalls != 1 {
		t.Fatalf("download-url calls = %d", f.dlCalls)
	}
	if b, _ := os.ReadFile(files[0].Path); string(b) != "ok" {
		t.Fatalf("content %q", b)
	}
	if files[0].URL != f.srv.URL+"/x.png" {
		t.Fatalf("url = %s", files[0].URL)
	}
}

func TestResultGETNeverCarriesAuthorization(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signed" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Error(w, "no", http.StatusForbidden)
	})
	if _, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png", f.srv.URL + "/y.png"}, t.TempDir(), "m", "t"); err != nil {
		t.Fatal(err)
	}
	if len(f.authOnGET) < 3 {
		t.Fatalf("expected CDN GETs, saw %d", len(f.authOnGET))
	}
	for _, a := range f.authOnGET {
		if a != "" {
			t.Fatalf("CDN received Authorization %q", a)
		}
	}
}

func TestDownloadRejectsHTTP(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { t.Error("must not fetch") })
	_, err := download.All(context.Background(), f.c, []string{"http://example.com/a.png"}, t.TempDir(), "m", "t")
	assertIOError(t, err, "http://example.com/a.png")
}

func TestDownloadFailureReturnsIOError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusNotFound) })
	u := f.srv.URL + "/x.png"
	_, err := download.All(context.Background(), f.c, []string{u}, t.TempDir(), "m", "t")
	assertIOError(t, err, u)
}

func TestNoPartFileLeftOnError(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	})
	dir := t.TempDir()
	_, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, dir, "m", "t")
	if err == nil {
		t.Fatal("expected error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestDownloadRejectsRedirectToHTTP(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/a.png", http.StatusFound)
	})
	_, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, t.TempDir(), "m", "t")
	if err == nil {
		t.Fatal("expected error")
	}
}

func assertIOError(t *testing.T, err error, url string) {
	t.Helper()
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeIO || oe.Details["url"] != url {
		t.Fatalf("err = %#v", err)
	}
}

func okServer(t *testing.T) *fake {
	return newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("new")) })
}

func TestTraversalStaysInsideOutDir(t *testing.T) {
	f := okServer(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "out")
	for _, tc := range [][2]string{{"kling", "a/../../x"}, {`..\..\evil/..`, ".."}, {"", ""}} {
		files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, dir, tc[0], tc[1])
		if err != nil {
			t.Fatalf("%v: %v", tc, err)
		}
		if filepath.Dir(files[0].Path) != dir {
			t.Fatalf("%v escaped: %s", tc, files[0].Path)
		}
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Fatalf("wrote outside outDir: %v", entries)
	}
}

func TestNormalNameUnchanged(t *testing.T) {
	f := okServer(t)
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.mp4"}, t.TempDir(), "kling-3.0/video", "t1")
	if err != nil || filepath.Base(files[0].Path) != "kling-3.0-video_t1_0.mp4" {
		t.Fatalf("%v %+v", err, files)
	}
}

func TestNoHardLinkFallback(t *testing.T) {
	defer download.SetLink(func(string, string) error { return syscall.ENOTSUP })()
	f := okServer(t)
	dir := t.TempDir()
	orig := filepath.Join(dir, "m_t_0.png")
	if err := os.WriteFile(orig, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/x.png"}, dir, "m", "t")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(files[0].Path) != "m_t_0-1.png" {
		t.Fatalf("got %s", files[0].Path)
	}
	if b, _ := os.ReadFile(files[0].Path); string(b) != "new" {
		t.Fatalf("content %q", b)
	}
	if b, _ := os.ReadFile(orig); string(b) != "orig" {
		t.Fatalf("original modified: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("leftovers: %v", entries)
	}
}

// slowBody writes part of the body, waits, then the rest; it stops early when
// the client goes away.
func slowBody(pause time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		select {
		case <-time.After(pause):
			_, _ = w.Write([]byte("second"))
		case <-r.Context().Done():
		}
	}
}

func TestDownloadBodyOutlivesAPIClientTimeout(t *testing.T) {
	f := newFake(t, slowBody(300*time.Millisecond))
	f.c.HTTP.Timeout = 100 * time.Millisecond // would kill the body read
	files, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/media/a.mp4"}, t.TempDir(), "m", "t")
	if err != nil {
		t.Fatalf("slow body must not hit the API client timeout: %v", err)
	}
	if b, _ := os.ReadFile(files[0].Path); string(b) != "firstsecond" {
		t.Fatalf("content %q", b)
	}
}

func TestDownloadCancelAbortsSlowBody(t *testing.T) {
	f := newFake(t, slowBody(10*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := download.All(ctx, f.c, []string{f.srv.URL + "/media/a.mp4"}, t.TempDir(), "m", "t")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("cancel did not abort the download: %v", time.Since(start))
	}
}

func TestDownloadHeaderTimeout(t *testing.T) {
	defer download.SetHeaderTimeout(50 * time.Millisecond)()
	f := newFake(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})
	_, err := download.All(context.Background(), f.c, []string{f.srv.URL + "/media/a.mp4"}, t.TempDir(), "m", "t")
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeIO {
		t.Fatalf("want io error, got %v", err)
	}
}

func TestDownloadClientHasNoOverallTimeout(t *testing.T) {
	hc := &http.Client{Timeout: time.Minute}
	if got := download.NewDownloadClient(hc); got.Timeout != 0 {
		t.Fatalf("download client Timeout=%v, want 0", got.Timeout)
	}
	if hc.Timeout != time.Minute {
		t.Fatal("input client must not be mutated")
	}
}
