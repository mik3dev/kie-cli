// Package output defines the JSON output contract and exit codes of kie-cli.
package output

import (
	"encoding/json"
	"errors"
	"io"
)

// Error codes exposed in the "error.code" field.
const (
	CodeValidation      = "validation"
	CodeAuth            = "auth"
	CodeCredits         = "credits"
	CodeRateLimit       = "rate_limit"
	CodeSubmitUncertain = "submit_uncertain"
	CodeTaskFailed      = "task_failed"
	CodeTimeout         = "timeout"
	CodeAPI             = "api"
	CodeIO              = "io"
	CodeUsage           = "usage"
)

var exitCodes = map[string]int{
	CodeUsage:           2,
	CodeValidation:      2,
	CodeAuth:            3,
	CodeCredits:         4,
	CodeTaskFailed:      5,
	CodeTimeout:         6,
	CodeRateLimit:       7,
	CodeSubmitUncertain: 8,
	CodeAPI:             1,
	CodeIO:              1,
}

// Error is a structured, agent-readable failure.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// ExitCode maps the error code to the process exit code (1 for unknown codes).
func (e *Error) ExitCode() int {
	if c, ok := exitCodes[e.Code]; ok {
		return c
	}
	return 1
}

// Success writes {"ok":true,...v} as a single JSON line. v is not mutated.
func Success(w io.Writer, v map[string]any) error {
	out := make(map[string]any, len(v)+1)
	for k, val := range v {
		out[k] = val
	}
	out["ok"] = true
	return json.NewEncoder(w).Encode(out)
}

// Fail writes {"ok":false,"error":{...}} as a single JSON line and returns the exit code.
// Errors that are not *Error are reported as CodeAPI.
func Fail(w io.Writer, err error) int {
	var oe *Error
	if !errors.As(err, &oe) {
		oe = &Error{Code: CodeAPI, Message: err.Error()}
	}
	_ = json.NewEncoder(w).Encode(struct {
		OK    bool   `json:"ok"`
		Error *Error `json:"error"`
	}{false, oe})
	return oe.ExitCode()
}
