// Package download saves task result files locally without ever overwriting.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
)

// File is one downloaded result.
type File struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

// All downloads every url into outDir as <model>_<taskID>_<n><ext>.
// Result GETs go through a plain request: never the Authorization header,
// since result files live on third-party CDN hosts. Only the download-url
// fallback is authenticated, and it goes to the kie API host.
func All(ctx context.Context, c *api.Client, urls []string, outDir, model, taskID string) ([]File, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, ioError(err.Error(), "")
	}
	files := make([]File, 0, len(urls))
	for i, u := range urls {
		f, err := one(ctx, c, u, outDir, baseName(model, taskID, i))
		if err != nil {
			return files, err
		}
		files = append(files, f)
	}
	return files, nil
}

func baseName(model, taskID string, n int) string {
	return fmt.Sprintf("%s_%s_%d", safeToken(model), safeToken(taskID), n)
}

// safeToken keeps [A-Za-z0-9._-], maps everything else to "-" and removes
// ".." so the token can never act as a path component.
func safeToken(s string) string {
	b := []rune(s)
	for i, r := range b {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			b[i] = '-'
		}
	}
	out := string(b)
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "-")
	}
	if out == "" || out == "." {
		return "x"
	}
	return out
}

func one(ctx context.Context, c *api.Client, u, outDir, base string) (File, error) {
	if err := requireHTTPS(u); err != nil {
		return File{}, ioError(err.Error(), u)
	}
	hc := downloadClient(c.HTTP)
	resp, err := get(ctx, hc, u)
	if err == nil && !ok(resp) {
		_ = resp.Body.Close()
		resp, err = viaDownloadURL(ctx, c, hc, u)
	}
	if err != nil {
		return File{}, ioError(scrub(c, err.Error()), u)
	}
	defer func() { _ = resp.Body.Close() }()
	name, kind := nameAndKind(u, resp.Header.Get("Content-Type"))
	dest, err := save(resp.Body, outDir, base, name)
	if err != nil {
		return File{}, ioError(scrub(c, err.Error()), u)
	}
	return File{Path: dest, URL: u, Kind: kind}, nil
}

func requireHTTPS(u string) error {
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "https" || p.Host == "" {
		return errors.New("only https result URLs are downloaded")
	}
	return nil
}

func ok(r *http.Response) bool { return r.StatusCode >= 200 && r.StatusCode <= 299 }

// get issues a bare GET: no Authorization, no API key in any form.
func get(ctx context.Context, hc *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL: signed URLs carry tokens
		}
		return nil, err
	}
	return resp, nil
}

func viaDownloadURL(ctx context.Context, c *api.Client, hc *http.Client, u string) (*http.Response, error) {
	var signed string
	err := c.Do(ctx, api.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/common/download-url",
		Body:   map[string]string{"url": u},
	}, &signed)
	if err != nil {
		return nil, err
	}
	if err := requireHTTPS(signed); err != nil {
		return nil, err
	}
	resp, err := get(ctx, hc, signed)
	if err != nil {
		return nil, err
	}
	if !ok(resp) {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// headerTimeout bounds the wait for response headers. It is a variable so
// tests can shorten it.
var headerTimeout = 60 * time.Second

// downloadClient derives the CDN client from hc. Unlike the API client it has
// no overall Timeout (that would cut long body reads); the transfer is bound
// by ctx and a response-header timeout instead. Redirects to non-https URLs
// are refused. hc's transport is cloned (never mutated); a custom non-stdlib
// RoundTripper is used as is.
func downloadClient(hc *http.Client) *http.Client {
	cp := http.Client{}
	rt := http.DefaultTransport
	if hc != nil {
		cp = *hc
		if hc.Transport != nil {
			rt = hc.Transport
		}
	}
	if t, ok := rt.(*http.Transport); ok {
		clone := t.Clone()
		clone.ResponseHeaderTimeout = headerTimeout
		rt = clone
	}
	cp.Transport = rt
	cp.Timeout = 0
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("refusing redirect to non-https URL")
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &cp
}

// nameAndKind returns the file extension (with dot) and the kind.
// The URL path extension wins over Content-Type.
func nameAndKind(u, contentType string) (ext, kind string) {
	mt, _, _ := mime.ParseMediaType(contentType)
	if p, err := url.Parse(u); err == nil {
		ext = path.Ext(p.Path)
	}
	if ext == "" && mt != "" {
		ext = extForType(mt)
	}
	if ext == "" {
		ext = ".bin"
	}
	if k := kindOf(mt); k != "other" {
		return ext, k
	}
	return ext, kindOf(mime.TypeByExtension(ext))
}

// preferredExt pins the extension for common types, since
// mime.ExtensionsByType returns candidates in no guaranteed order.
var preferredExt = map[string]string{
	"video/mp4": ".mp4", "audio/mpeg": ".mp3", "image/jpeg": ".jpg",
	"image/png": ".png", "image/webp": ".webp", "audio/wav": ".wav",
}

func extForType(mt string) string {
	if e, ok := preferredExt[mt]; ok {
		return e
	}
	exts, _ := mime.ExtensionsByType(mt)
	if len(exts) == 0 {
		return ""
	}
	sort.Strings(exts)
	return exts[0]
}

func kindOf(mt string) string {
	for _, k := range []string{"image", "video", "audio"} {
		if strings.HasPrefix(mt, k+"/") {
			return k
		}
	}
	return "other"
}

// save streams body to a temp .part file, then hard-links it to the first free
// name (base+ext, base-1+ext, ...). Link fails if the target exists, so an
// existing file is never replaced, even under a race.
func save(body io.Reader, outDir, base, ext string) (string, error) {
	part, err := os.CreateTemp(outDir, ".kie-*.part")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(part.Name()) }()
	if _, err := io.Copy(part, body); err != nil {
		_ = part.Close()
		return "", err
	}
	if err := part.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(part.Name(), 0o644); err != nil {
		return "", err
	}
	for n := 0; ; n++ {
		name := base + ext
		if n > 0 {
			name = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		dest := filepath.Join(outDir, name)
		if filepath.Dir(dest) != filepath.Clean(outDir) {
			return "", errors.New("refusing to write outside the output directory")
		}
		err := linkFile(part.Name(), dest)
		if err != nil && !errors.Is(err, os.ErrExist) {
			// No hard links here (FAT, some network mounts): exclusive copy.
			err = copyExclusive(part.Name(), dest)
		}
		if err == nil {
			return dest, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}

// linkFile is a variable so tests can simulate filesystems without hard links.
var linkFile = os.Link

// copyExclusive creates dest with O_EXCL (never replaces) and copies src into it.
func copyExclusive(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func scrub(c *api.Client, s string) string {
	if c.Key == "" {
		return s
	}
	return strings.ReplaceAll(s, c.Key, "****")
}

func ioError(msg, u string) *output.Error {
	e := &output.Error{Code: output.CodeIO, Message: "download failed: " + msg}
	if u != "" {
		e.Details = map[string]any{"url": u}
	}
	return e
}
