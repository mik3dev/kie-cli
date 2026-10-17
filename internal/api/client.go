package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mik3dev/kie-cli/internal/config"
)

const (
	defaultBaseURL   = "https://api.kie.ai"
	defaultUploadURL = "https://kieai.redpandaai.co"
	maxRetries       = 3
	maxBody          = 32 << 20
)

type Host int

const (
	HostAPI Host = iota
	HostUpload
)

// Request describes one API call.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
	// BodyFactory, when set, replaces Body: it is called once per attempt to
	// open a fresh (e.g. multipart) body, so retries can re-read the source.
	BodyFactory func() (contentType string, body io.ReadCloser, err error)
	Host        Host
	Class       Class
}

// Client talks to kie.ai. Safe for concurrent use.
type Client struct {
	BaseURL, UploadURL, Key string
	HTTP, UploadHTTP        *http.Client
	Debug                   io.Writer
	Sleep                   func(context.Context, time.Duration) error
	Now                     func() time.Time

	limOnce sync.Once
	lim     *limiter
}

// New builds a client; env resolves KIE_BASE_URL / KIE_UPLOAD_URL overrides.
func New(key string, env func(string) string) *Client {
	pick := func(name, def string) string {
		if env != nil {
			if v := env(name); v != "" {
				return strings.TrimRight(v, "/")
			}
		}
		return def
	}
	return &Client{
		BaseURL:    pick("KIE_BASE_URL", defaultBaseURL),
		UploadURL:  pick("KIE_UPLOAD_URL", defaultUploadURL),
		Key:        key,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		UploadHTTP: &http.Client{Timeout: 10 * time.Minute},
		Sleep:      sleepCtx,
		Now:        time.Now,
	}
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

type envelope struct {
	Code *int            `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// Do performs the request with limiter and safe retries, decoding data into out.
func (c *Client) Do(ctx context.Context, req Request, out any) error {
	var payload []byte
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		payload = b
	}
	c.limOnce.Do(func() { c.lim = newLimiter(c.nowFn(), c.sleepFn()) })
	var err error
	for attempt := 0; ; attempt++ {
		if werr := c.lim.wait(ctx, req.Class); werr != nil {
			return werr
		}
		err = c.once(ctx, req, payload, out)
		if err == nil || attempt >= maxRetries || ctx.Err() != nil || !retryable(req.Method, err) {
			return err
		}
		if serr := c.sleepFn()(ctx, time.Second<<attempt); serr != nil {
			return err
		}
	}
}

func (c *Client) nowFn() func() time.Time {
	if c.Now != nil {
		return c.Now
	}
	return time.Now
}

func (c *Client) sleepFn() func(context.Context, time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep
	}
	return sleepCtx
}

// retryable: a POST (createTask, uploads) is only retried when kie rejected it
// with 429 or when the request provably never left (NetError.Sent==false).
// Never on 5xx/455 or Sent==true: the task may already exist and cost credits.
// GET is idempotent, so it also retries 5xx/455 and ambiguous network errors.
func retryable(method string, err error) bool {
	var ne *NetError
	if errors.As(err, &ne) {
		return !ne.Sent || method == http.MethodGet
	}
	var ae *Error
	if errors.As(err, &ae) {
		if ae.KieCode == 429 {
			return true
		}
		return method == http.MethodGet && (ae.KieCode == 455 || ae.KieCode >= 500)
	}
	return false
}

func (c *Client) once(ctx context.Context, r Request, payload []byte, out any) error {
	base, hc := c.BaseURL, c.HTTP
	if r.Host == HostUpload {
		base, hc = c.UploadURL, c.UploadHTTP
	}
	u := base + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var body io.Reader
	contentType := ""
	if payload != nil {
		body = bytes.NewReader(payload)
		contentType = "application/json"
	}
	if r.BodyFactory != nil {
		ct, rc, ferr := r.BodyFactory()
		if ferr != nil {
			return fmt.Errorf("open request body: %w", ferr)
		}
		defer rc.Close()
		body, contentType = rc, ct
	}
	hreq, err := http.NewRequestWithContext(ctx, r.Method, u, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	hreq.Header.Set("Authorization", "Bearer "+c.Key)
	hreq.Header.Set("Accept", "application/json")
	if contentType != "" {
		hreq.Header.Set("Content-Type", contentType)
	}
	var wroteHeaders atomic.Bool
	hreq = hreq.WithContext(httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteHeaders: func() { wroteHeaders.Store(true) },
	}))

	resp, err := hc.Do(hreq)
	if err != nil {
		c.debugf("%s %s -> error\n", r.Method, u)
		return &NetError{Sent: wroteHeaders.Load(), Err: c.scrubErr(err)}
	}
	defer resp.Body.Close()
	c.debugf("%s %s -> %d\n", r.Method, u, resp.StatusCode)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return &NetError{Sent: true, Err: c.scrubErr(err)}
	}
	return c.decode(resp.StatusCode, raw, out)
}

func (c *Client) decode(status int, raw []byte, out any) error {
	var env envelope
	jerr := json.Unmarshal(raw, &env)
	if status < 200 || status > 299 {
		msg := env.Msg
		if jerr != nil || msg == "" {
			msg = http.StatusText(status)
		}
		return &Error{KieCode: status, Msg: c.scrub(msg)}
	}
	if jerr != nil || env.Code == nil {
		return &Error{KieCode: status, Msg: "invalid response envelope"}
	}
	if *env.Code != 200 {
		return &Error{KieCode: *env.Code, Msg: c.scrub(env.Msg)}
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return &Error{KieCode: status, Msg: "unexpected data shape: " + err.Error()}
	}
	return nil
}

func (c *Client) debugf(format string, args ...any) {
	if c.Debug == nil {
		return
	}
	fmt.Fprintf(c.Debug, "Authorization: Bearer ****\n"+format, args...)
}

func (c *Client) scrub(s string) string {
	if c.Key == "" {
		return s
	}
	return strings.ReplaceAll(s, c.Key, config.Redact(c.Key))
}

func (c *Client) scrubErr(err error) error {
	if c.Key == "" || !strings.Contains(err.Error(), c.Key) {
		return err
	}
	return errors.New(c.scrub(err.Error()))
}
