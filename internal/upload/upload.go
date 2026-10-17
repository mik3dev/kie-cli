// Package upload sends local files referenced in a task input to kie's temp file service.
package upload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
)

const (
	maxFileSize    = 100 << 20
	uploadEndpoint = "/api/file-stream-upload"
)

var (
	fileKeyWords = []string{"url", "image", "video", "audio", "file", "mask"}
	// nonFileSuffixes/nonFilePrefixes name keys that contain a file word but never hold a file.
	// ponytail: small explicit list; ceiling = a new non-file key with a file word (and no schema
	// to say so) is uploaded when its value is an existing local file. Extend the list then.
	nonFileSuffixes = []string{"_id", "_ids", "_size", "_weight", "_setting", "_settings"}
	nonFilePrefixes = []string{"max_", "generate_"}
	remotePrefix    = []string{"http://", "https://", "oss://", "asset://"}
	mediaExts       = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true,
		".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
		".mp3": true, ".wav": true, ".m4a": true, ".aac": true, ".ogg": true, ".flac": true,
	}
)

func validation(format string, args ...any) *output.Error {
	return &output.Error{Code: output.CodeValidation, Message: fmt.Sprintf(format, args...)}
}

// FileKey reports whether an input key may hold a file reference.
// ponytail: name heuristic; stage 2 switches to schema-based detection.
func FileKey(key string) bool {
	k := strings.ToLower(key)
	for _, x := range nonFileSuffixes {
		if strings.HasSuffix(k, x) {
			return false
		}
	}
	for _, x := range nonFilePrefixes {
		if strings.HasPrefix(k, x) {
			return false
		}
	}
	for _, w := range fileKeyWords {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// File uploads one local file and returns its temporary download URL.
func File(ctx context.Context, c *api.Client, path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return "", validation("file not found: %s", path)
	}
	if st.Size() > maxFileSize {
		return "", validation("file too large (%d bytes, max 100MB): %s", st.Size(), path)
	}
	dir, err := uploadDir()
	if err != nil {
		return "", err
	}
	name := filepath.Base(path)
	req := api.Request{
		Method: http.MethodPost, Path: uploadEndpoint, Host: api.HostUpload,
		BodyFactory: func() (string, io.ReadCloser, error) { return openMultipart(path, dir, name) },
	}
	var out struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := c.Do(ctx, req, &out); err != nil {
		var ae *api.Error
		if errors.As(err, &ae) {
			return "", ae.AsOutput()
		}
		return "", err
	}
	if out.DownloadURL == "" {
		return "", &output.Error{Code: output.CodeAPI, Message: "upload response missing downloadUrl"}
	}
	return out.DownloadURL, nil
}

func uploadDir() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random upload path: %w", err)
	}
	return "kie-cli/" + time.Now().UTC().Format("20060102") + "/" + hex.EncodeToString(b), nil
}

// openMultipart streams the file through a pipe; each call re-opens the file so retries work.
func openMultipart(path, uploadPath, name string) (string, io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer func() { _ = f.Close() }()
		err := mw.WriteField("uploadPath", uploadPath)
		if err == nil {
			err = mw.WriteField("fileName", name)
		}
		if err == nil {
			var part io.Writer
			if part, err = mw.CreateFormFile("file", name); err == nil {
				_, err = io.Copy(part, f)
			}
		}
		if err == nil {
			err = mw.Close()
		}
		pw.CloseWithError(err)
	}()
	return mw.FormDataContentType(), pr, nil
}

// ResolveLocal returns a new input map with local file references replaced by uploaded URLs,
// plus the local paths that were uploaded. Only keys for which isFileKey is true
// are scanned; a nil isFileKey falls back to FileKey.
func ResolveLocal(ctx context.Context, input map[string]any, isFileKey func(string) bool, up func(context.Context, string) (string, error)) (map[string]any, []string, error) {
	if isFileKey == nil {
		isFileKey = FileKey
	}
	r := &resolver{ctx: ctx, up: up, done: map[string]string{}}
	out := make(map[string]any, len(input))
	for k, v := range input {
		if !isFileKey(k) {
			out[k] = v
			continue
		}
		nv, err := r.walk(v)
		if err != nil {
			return nil, nil, err
		}
		out[k] = nv
	}
	return out, r.uploaded, nil
}

type resolver struct {
	ctx      context.Context
	up       func(context.Context, string) (string, error)
	done     map[string]string
	uploaded []string
}

func (r *resolver) walk(v any) (any, error) {
	switch t := v.(type) {
	case string:
		return r.str(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			ne, err := r.walk(e)
			if err != nil {
				return nil, err
			}
			out[i] = ne
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			ne, err := r.walk(e)
			if err != nil {
				return nil, err
			}
			out[k] = ne
		}
		return out, nil
	}
	return v, nil
}

func (r *resolver) str(s string) (any, error) {
	for _, p := range remotePrefix {
		if strings.HasPrefix(s, p) {
			return s, nil
		}
	}
	path := expandHome(s)
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		if looksLikePath(s) {
			return nil, validation("file not found: %s", s)
		}
		return s, nil
	}
	if u, ok := r.done[path]; ok {
		return u, nil
	}
	u, err := r.up(r.ctx, path)
	if err != nil {
		return nil, err
	}
	r.done[path] = u
	r.uploaded = append(r.uploaded, s)
	return u, nil
}

func expandHome(s string) string {
	if strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, s[2:])
		}
	}
	return s
}

func looksLikePath(s string) bool {
	for _, p := range []string{"./", "../", "/", "~/"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return mediaExts[strings.ToLower(filepath.Ext(s))]
}
