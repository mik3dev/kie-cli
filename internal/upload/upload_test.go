package upload_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/upload"
)

func fakeUp(calls *[]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, p string) (string, error) {
		*calls = append(*calls, p)
		return "https://up/" + filepath.Base(p), nil
	}
}

func inDir(t *testing.T, files ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
}

func TestResolveLocalUploadsFileKeys(t *testing.T) {
	inDir(t, "a.png")
	var calls []string
	got, uploaded, err := upload.ResolveLocal(context.Background(),
		map[string]any{"image_urls": []any{"./a.png", "https://x/b.png"}}, nil, fakeUp(&calls))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"image_urls": []any{"https://up/a.png", "https://x/b.png"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if !reflect.DeepEqual(uploaded, []string{"./a.png"}) {
		t.Fatalf("uploaded = %v", uploaded)
	}
}

func TestPromptMatchingFileIsNotUploaded(t *testing.T) {
	inDir(t, "a.png")
	var calls []string
	in := map[string]any{"prompt": "a.png"}
	got, uploaded, err := upload.ResolveLocal(context.Background(), in, nil, fakeUp(&calls))
	if err != nil || len(calls) != 0 || len(uploaded) != 0 || !reflect.DeepEqual(got, in) {
		t.Fatalf("got=%v calls=%v err=%v", got, calls, err)
	}
}

func TestNestedObjectsUnderFileKey(t *testing.T) {
	inDir(t, "v.mp4")
	var calls []string
	got, _, err := upload.ResolveLocal(context.Background(),
		map[string]any{"video_list": []any{map[string]any{"url": "./v.mp4", "n": 1}}}, nil, fakeUp(&calls))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"video_list": []any{map[string]any{"url": "https://up/v.mp4", "n": 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestMissingPathIsValidationError(t *testing.T) {
	inDir(t)
	var calls []string
	for _, p := range []string{"./nope.png", "nope.jpg", "/no/such/file"} {
		_, _, err := upload.ResolveLocal(context.Background(), map[string]any{"image_url": p}, nil, fakeUp(&calls))
		var oe *output.Error
		if !errors.As(err, &oe) || oe.Code != "validation" || oe.Message != "file not found: "+p {
			t.Fatalf("%s: err = %v", p, err)
		}
	}
	// plain non-path text under a file key is left alone
	if _, _, err := upload.ResolveLocal(context.Background(), map[string]any{"file_note": "hello"}, nil, fakeUp(&calls)); err != nil {
		t.Fatal(err)
	}
}

func TestResolveLocalDoesNotMutateInput(t *testing.T) {
	inDir(t, "a.png")
	var calls []string
	list := []any{"./a.png"}
	in := map[string]any{"image_urls": list, "prompt": "p"}
	_, _, err := upload.ResolveLocal(context.Background(), in, nil, fakeUp(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if list[0] != "./a.png" || !reflect.DeepEqual(in, map[string]any{"image_urls": []any{"./a.png"}, "prompt": "p"}) {
		t.Fatalf("input mutated: %v", in)
	}
}

func TestSchemesAreSkipped(t *testing.T) {
	inDir(t)
	var calls []string
	in := map[string]any{"image_urls": []any{"oss://b/k.png", "asset://abc.png", "http://x/y.png"}}
	got, _, err := upload.ResolveLocal(context.Background(), in, nil, fakeUp(&calls))
	if err != nil || !reflect.DeepEqual(got, in) || len(calls) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestTildeExpands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "h.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	_, _, err := upload.ResolveLocal(context.Background(), map[string]any{"image_url": "~/h.png"}, nil, fakeUp(&calls))
	if err != nil || len(calls) != 1 || calls[0] != filepath.Join(home, "h.png") {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
}

func TestFileKey(t *testing.T) {
	for k, want := range map[string]bool{"image_url": true, "Video": true, "mask_x": true, "audio": true, "input_file": true, "prompt": false, "duration": false} {
		if upload.FileKey(k) != want {
			t.Errorf("FileKey(%q) != %v", k, want)
		}
	}
}

func uploadClient(t *testing.T, h http.HandlerFunc) *api.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := api.New("k-secret", func(string) string { return "" })
	c.BaseURL, c.UploadURL = srv.URL, srv.URL
	return c
}

func TestUploadMultipartFields(t *testing.T) {
	inDir(t, "a.png")
	re := regexp.MustCompile(`^kie-cli/\d{8}/[0-9a-f]{16}$`)
	c := uploadClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file-stream-upload" || r.Header.Get("Authorization") != "Bearer k-secret" {
			t.Errorf("path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if !re.MatchString(r.FormValue("uploadPath")) || r.FormValue("fileName") != "a.png" {
			t.Errorf("uploadPath=%q fileName=%q", r.FormValue("uploadPath"), r.FormValue("fileName"))
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		_ = f.Close()
		_, _ = w.Write([]byte(`{"success":true,"code":200,"msg":"ok","data":{"downloadUrl":"https://tmp/a.png"}}`))
	})
	got, err := upload.File(context.Background(), c, "a.png")
	if err != nil || got != "https://tmp/a.png" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestUploadAPIErrorMapped(t *testing.T) {
	inDir(t, "a.png")
	c := uploadClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":401,"msg":"bad key"}`))
	})
	_, err := upload.File(context.Background(), c, "a.png")
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "auth" {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadTooLarge(t *testing.T) {
	inDir(t)
	var hits atomic.Int32
	c := uploadClient(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
	f, err := os.Create("big.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(101 << 20); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	_, err = upload.File(context.Background(), c, "big.mp4")
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "validation" || hits.Load() != 0 {
		t.Fatalf("err=%v hits=%d", err, hits.Load())
	}
}

func TestResolveLocalCustomPredicate(t *testing.T) {
	inDir(t, "a.png", "b.png")
	var calls []string
	in := map[string]any{"ref": "./a.png", "image_url": "./b.png"}
	got, uploaded, err := upload.ResolveLocal(context.Background(), in,
		func(k string) bool { return k == "ref" }, fakeUp(&calls))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"ref": "https://up/a.png", "image_url": "./b.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if !reflect.DeepEqual(uploaded, []string{"./a.png"}) {
		t.Fatalf("uploaded = %v", uploaded)
	}
}

func TestFileKeyExcludesNonFileNames(t *testing.T) {
	for _, k := range []string{"audio_id", "audio_ids", "audio_weight", "image_size", "audio_setting", "audio_settings", "max_images", "generate_audio"} {
		if upload.FileKey(k) {
			t.Errorf("FileKey(%q) should be false", k)
		}
	}
	for _, k := range []string{"image_urls", "image", "audio_url", "video_url", "mask"} {
		if !upload.FileKey(k) {
			t.Errorf("FileKey(%q) should be true", k)
		}
	}
}

func TestSchemaFileKeyNestedURI(t *testing.T) {
	sch := map[string]any{"properties": map[string]any{
		"video_list": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"url": map[string]any{"type": "string", "format": "uri"}}}},
		"image_size": map[string]any{"type": "string", "enum": []any{"a"}},
	}}
	if !upload.SchemaFileKey(sch, "video_list") || upload.SchemaFileKey(sch, "image_size") {
		t.Fatal("SchemaFileKey wrong")
	}
}

func TestResolveLocalNestedObjects(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"video_list": []any{map[string]any{"url": f}}}
	out, given, err := upload.ResolveLocal(context.Background(), in, func(k string) bool { return k == "video_list" },
		func(context.Context, string) (string, error) { return "https://u/x.png", nil })
	if err != nil || len(given) != 1 {
		t.Fatalf("err=%v given=%v", err, given)
	}
	got := out["video_list"].([]any)[0].(map[string]any)["url"]
	if got != "https://u/x.png" {
		t.Fatalf("got %v", got)
	}
}
