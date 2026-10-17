package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func authDeps(t *testing.T, stdin string, env map[string]string) (cli.Deps, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errw bytes.Buffer
	if _, ok := env["KIE_CLI_HOME"]; !ok {
		env["KIE_CLI_HOME"] = filepath.Join(t.TempDir(), "home")
	}
	return cli.Deps{
		Stdout: &out, Stderr: &errw, Stdin: strings.NewReader(stdin),
		Env: func(k string) string { return env[k] }, Now: time.Now,
	}, &out, &errw
}

func decode(t *testing.T, b *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b.Bytes(), &m); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, b.String())
	}
	return m
}

func TestAuthLoginFromStdinPipe(t *testing.T) {
	env := map[string]string{}
	deps, out, _ := authDeps(t, "abc123456789xyz\n", env)
	if code := cli.Execute(deps, []string{"auth", "login"}); code != 0 {
		t.Fatalf("login exit = %d, out=%s", code, out)
	}
	m := decode(t, out)
	if m["ok"] != true || m["saved"] != filepath.Join(env["KIE_CLI_HOME"], "config.json") {
		t.Errorf("login = %v", m)
	}
	var out2, err2 bytes.Buffer
	deps2 := deps
	deps2.Stdout, deps2.Stderr = &out2, &err2
	if code := cli.Execute(deps2, []string{"auth", "status"}); code != 0 {
		t.Fatalf("status exit = %d, out=%s", code, out2.String())
	}
	st := decode(t, &out2)
	if st["source"] != "config" || !strings.HasSuffix(st["key"].(string), "…9xyz") {
		t.Errorf("status = %v", st)
	}
	if strings.Contains(out.String()+out2.String()+err2.String(), "abc123456789xyz") {
		t.Errorf("key leaked")
	}
}

func TestAuthLoginEmptyStdinIsUsageError(t *testing.T) {
	deps, out, _ := authDeps(t, "\n", map[string]string{})
	if code := cli.Execute(deps, []string{"auth", "login"}); code != 2 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	if !strings.Contains(out.String(), `"usage"`) {
		t.Errorf("out = %q", out.String())
	}
}

func TestAuthLoginNoNewlineStdin(t *testing.T) {
	env := map[string]string{}
	deps, out, _ := authDeps(t, "  abc123456789xyz  ", env)
	if code := cli.Execute(deps, []string{"auth", "login"}); code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	data, _ := os.ReadFile(filepath.Join(env["KIE_CLI_HOME"], "config.json"))
	if !strings.Contains(string(data), `"abc123456789xyz"`) {
		t.Errorf("config = %s", data)
	}
}

func TestAuthStatusFromEnv(t *testing.T) {
	deps, out, _ := authDeps(t, "", map[string]string{"KIE_API_KEY": "envkey-0123456789"})
	if code := cli.Execute(deps, []string{"auth", "status"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	m := decode(t, out)
	if m["source"] != "env" || m["key"] != "…6789" {
		t.Errorf("m = %v", m)
	}
}

func TestAuthStatusShortKeyShowsNothing(t *testing.T) {
	deps, out, _ := authDeps(t, "", map[string]string{"KIE_API_KEY": "short"})
	if code := cli.Execute(deps, []string{"auth", "status"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "short") {
		t.Errorf("leaked: %s", out)
	}
}

func TestAuthStatusNoKeyIsAuthError(t *testing.T) {
	deps, out, _ := authDeps(t, "", map[string]string{})
	if code := cli.Execute(deps, []string{"auth", "status"}); code != 3 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), `"auth"`) {
		t.Errorf("out = %q", out.String())
	}
}

func TestAuthStatusWarnsLoosePermsOnStderrUnlessQuiet(t *testing.T) {
	env := map[string]string{}
	deps, _, _ := authDeps(t, "abc123456789xyz\n", env)
	cli.Execute(deps, []string{"auth", "login"})
	cfg := filepath.Join(env["KIE_CLI_HOME"], "config.json")
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	d := deps
	d.Stdout, d.Stderr = &out, &errw
	if code := cli.Execute(d, []string{"auth", "status"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errw.String(), "should be 0600") {
		t.Errorf("stderr = %q", errw.String())
	}
	var out2, err2 bytes.Buffer
	d.Stdout, d.Stderr = &out2, &err2
	cli.Execute(d, []string{"--quiet", "auth", "status"})
	if err2.Len() != 0 {
		t.Errorf("quiet stderr = %q", err2.String())
	}
}
