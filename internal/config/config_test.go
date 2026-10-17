package config_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/output"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolvePathsHonorsKIE_CLI_HOME(t *testing.T) {
	p, err := config.ResolvePaths(envOf(map[string]string{"KIE_CLI_HOME": "/x/kie", "HOME": "/h"}))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Paths{
		Home: "/x/kie", Config: "/x/kie/config.json", Presets: "/x/kie/presets.yaml",
		Cache: "/x/kie/cache", Ledger: "/x/kie/tasks.jsonl",
	}
	if p != want {
		t.Errorf("got %+v want %+v", p, want)
	}
}

func TestResolvePathsDefaultsToHomeDir(t *testing.T) {
	p, err := config.ResolvePaths(envOf(map[string]string{"HOME": "/h"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != "/h/.kie-cli" {
		t.Errorf("home = %q", p.Home)
	}
}

func TestResolvePathsNoHomeFails(t *testing.T) {
	if _, err := config.ResolvePaths(envOf(nil)); err == nil {
		t.Fatal("expected error")
	}
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	p, err := config.ResolvePaths(envOf(map[string]string{"KIE_CLI_HOME": filepath.Join(t.TempDir(), "home")}))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSaveWrites0600AndHome0700(t *testing.T) {
	p := testPaths(t)
	if err := config.Save(p, config.Config{APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p.Config)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("config perm = %o", fi.Mode().Perm())
	}
	hi, err := os.Stat(p.Home)
	if err != nil {
		t.Fatal(err)
	}
	if hi.Mode().Perm() != 0o700 {
		t.Errorf("home perm = %o", hi.Mode().Perm())
	}
	entries, _ := os.ReadDir(p.Home)
	if len(entries) != 1 {
		t.Errorf("leftover temp files: %v", entries)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	var w bytes.Buffer
	c, err := config.Load(testPaths(t), &w)
	if err != nil || c != (config.Config{}) || w.Len() != 0 {
		t.Errorf("c=%+v err=%v warn=%q", c, err, w.String())
	}
}

func TestLoadRoundTrip(t *testing.T) {
	p := testPaths(t)
	in := config.Config{APIKey: "k", OutDir: "/o"}
	if err := config.Save(p, in); err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	out, err := config.Load(p, &w)
	if err != nil || out != in || w.Len() != 0 {
		t.Errorf("out=%+v err=%v warn=%q", out, err, w.String())
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	p := testPaths(t)
	if err := config.EnsureHome(p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Config, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadWarnsOnLoosePerms(t *testing.T) {
	p := testPaths(t)
	if err := config.Save(p, config.Config{APIKey: "secretkey"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p.Config, 0o644); err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	c, err := config.Load(p, &w)
	if err != nil || c.APIKey != "secretkey" {
		t.Fatalf("c=%+v err=%v", c, err)
	}
	if !strings.Contains(w.String(), "should be 0600") || !strings.Contains(w.String(), "0644") {
		t.Errorf("warn = %q", w.String())
	}
}

func TestResolveKeyEnvBeatsConfig(t *testing.T) {
	k, src, err := config.ResolveKey(envOf(map[string]string{"KIE_API_KEY": "envkey"}), config.Config{APIKey: "cfgkey"})
	if err != nil || k != "envkey" || src != "env" {
		t.Errorf("k=%q src=%q err=%v", k, src, err)
	}
}

func TestResolveKeyEmptyEnvFallsBackToConfig(t *testing.T) {
	k, src, err := config.ResolveKey(envOf(map[string]string{"KIE_API_KEY": ""}), config.Config{APIKey: "cfgkey"})
	if err != nil || k != "cfgkey" || src != "config" {
		t.Errorf("k=%q src=%q err=%v", k, src, err)
	}
}

func TestResolveKeyMissing(t *testing.T) {
	_, _, err := config.ResolveKey(envOf(nil), config.Config{})
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "auth" {
		t.Fatalf("err = %v", err)
	}
	if oe.Message != "no API key: set KIE_API_KEY or run 'kie-cli auth login'" {
		t.Errorf("msg = %q", oe.Message)
	}
}

func TestRedactShortKey(t *testing.T) {
	if got := config.Redact("short"); got != "****" {
		t.Errorf("got %q", got)
	}
	if got := config.Redact("elevenchars"); got != "****" {
		t.Errorf("11 chars: got %q", got)
	}
}

func TestRedactLongKeyShowsLast4(t *testing.T) {
	if got := config.Redact("abc123456789xyz"); got != "…9xyz" {
		t.Errorf("got %q", got)
	}
}

func TestSaveFailsWhenHomeIsAFile(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.Home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Home, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(p, config.Config{APIKey: "k"}); err == nil {
		t.Fatal("expected error")
	}
	if err := config.EnsureHome(p); err == nil {
		t.Fatal("expected EnsureHome error")
	}
}

func TestLoadFailsWhenConfigIsADirectory(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(p.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}
