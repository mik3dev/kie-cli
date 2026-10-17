package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mik3dev/kie-cli/internal/output"
)

func TestExitCodes(t *testing.T) {
	cases := map[string]int{"usage": 2, "validation": 2, "auth": 3, "credits": 4, "task_failed": 5, "timeout": 6, "rate_limit": 7, "submit_uncertain": 8, "api": 1, "io": 1}
	for code, want := range cases {
		if got := (&output.Error{Code: code}).ExitCode(); got != want {
			t.Errorf("%s: got %d want %d", code, got, want)
		}
	}
	if got := (&output.Error{Code: "bogus"}).ExitCode(); got != 1 {
		t.Errorf("unknown code: got %d want 1", got)
	}
}

func TestErrorString(t *testing.T) {
	e := &output.Error{Code: "auth", Message: "no key"}
	if e.Error() != "no key" {
		t.Errorf("got %q", e.Error())
	}
}

func decodeOne(t *testing.T, b *bytes.Buffer) map[string]any {
	t.Helper()
	dec := json.NewDecoder(b)
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("trailing data after object: %v", err)
	}
	return m
}

func TestFailWritesSingleJSON(t *testing.T) {
	var b bytes.Buffer
	exit := output.Fail(&b, &output.Error{Code: "auth", Message: "no key"})
	if exit != 3 {
		t.Errorf("exit = %d", exit)
	}
	m := decodeOne(t, &b)
	if m["ok"] != false {
		t.Errorf("ok = %v", m["ok"])
	}
	e := m["error"].(map[string]any)
	if e["code"] != "auth" || e["message"] != "no key" {
		t.Errorf("error = %v", e)
	}
	if _, has := e["details"]; has {
		t.Errorf("details should be omitted when empty")
	}
}

func TestFailWithDetails(t *testing.T) {
	var b bytes.Buffer
	output.Fail(&b, &output.Error{Code: "timeout", Message: "m", Details: map[string]any{"taskId": "t1"}})
	e := decodeOne(t, &b)["error"].(map[string]any)
	if e["details"].(map[string]any)["taskId"] != "t1" {
		t.Errorf("details = %v", e["details"])
	}
}

func TestFailPlainErrorIsAPI(t *testing.T) {
	var b bytes.Buffer
	exit := output.Fail(&b, errors.New("boom"))
	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	e := decodeOne(t, &b)["error"].(map[string]any)
	if e["code"] != "api" || e["message"] != "boom" {
		t.Errorf("error = %v", e)
	}
}

func TestFailUnwrapsWrappedError(t *testing.T) {
	var b bytes.Buffer
	wrapped := errors.Join(errors.New("ctx"), &output.Error{Code: "credits", Message: "low"})
	if exit := output.Fail(&b, wrapped); exit != 4 {
		t.Errorf("exit = %d", exit)
	}
}

func TestSuccess(t *testing.T) {
	var b bytes.Buffer
	in := map[string]any{"version": "x"}
	if err := output.Success(&b, in); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		t.Errorf("no trailing newline")
	}
	m := decodeOne(t, &b)
	if m["ok"] != true || m["version"] != "x" {
		t.Errorf("m = %v", m)
	}
	if _, mutated := in["ok"]; mutated {
		t.Errorf("input map mutated")
	}
}

func TestSuccessNil(t *testing.T) {
	var b bytes.Buffer
	if err := output.Success(&b, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != `{"ok":true}` {
		t.Errorf("got %q", b.String())
	}
}
