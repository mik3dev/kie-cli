// Package api is the kie.ai HTTP client: envelope decoding, safe retries and rate limiting.
package api

import (
	"fmt"

	"github.com/mik3dev/kie-cli/internal/output"
)

// Error is a kie.ai failure: either an envelope code != 200 or a non-2xx HTTP status.
type Error struct {
	KieCode int
	Msg     string
}

func (e *Error) Error() string {
	return fmt.Sprintf("kie.ai error %d: %s", e.KieCode, e.Msg)
}

// AsOutput maps the kie code to the CLI output contract.
func (e *Error) AsOutput() *output.Error {
	code := output.CodeAPI
	switch e.KieCode {
	case 401:
		code = output.CodeAuth
	case 402:
		code = output.CodeCredits
	case 429, 455, 433:
		code = output.CodeRateLimit
	}
	return &output.Error{
		Code:    code,
		Message: e.Error(),
		Details: map[string]any{"kie_code": e.KieCode, "kie_msg": e.Msg},
	}
}

// NetError is a transport failure. Sent=false means the request provably never
// reached the server (safe to retry); when in doubt Sent is true.
type NetError struct {
	Sent bool
	Err  error
}

func (e *NetError) Error() string { return "network error: " + e.Err.Error() }
func (e *NetError) Unwrap() error { return e.Err }
