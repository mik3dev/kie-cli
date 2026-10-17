package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func newDeps(out, errw *bytes.Buffer) cli.Deps {
	return cli.Deps{
		Stdout: out,
		Stderr: errw,
		Stdin:  strings.NewReader(""),
		Env:    func(string) string { return "" },
		Now:    time.Now,
	}
}

func TestVersionCommand(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Execute(newDeps(&out, &errw), []string{"version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out.String() != `{"ok":true,"version":"dev"}`+"\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Execute(newDeps(&out, &errw), []string{"version", "--nope"}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout not single JSON: %v (%q)", err, out.String())
	}
	if m["ok"] != false || m["error"].(map[string]any)["code"] != "usage" {
		t.Errorf("m = %v", m)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Execute(newDeps(&out, &errw), []string{"nope"}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), `"usage"`) {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestPersistentFlagsAccepted(t *testing.T) {
	var out, errw bytes.Buffer
	if code := cli.Execute(newDeps(&out, &errw), []string{"--quiet", "--debug", "version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}
