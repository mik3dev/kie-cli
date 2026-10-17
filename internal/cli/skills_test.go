package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func skillFiles(t *testing.T, m map[string]any) []string {
	t.Helper()
	raw, _ := m["files"].([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(string))
	}
	return out
}

func TestSkillsInstallTarget(t *testing.T) {
	f := newFake(t)
	tmp := t.TempDir()
	r := f.run(t, "skills", "install", "--target", tmp)
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if r.m["target"] != tmp {
		t.Errorf("target=%v", r.m["target"])
	}
	files := skillFiles(t, r.m)
	have := map[string]bool{}
	for _, p := range files {
		have[p] = true
	}
	for _, rel := range []string{"SKILL.md", "references/image.md", "references/video.md", "references/audio.md"} {
		p := filepath.Join(tmp, "kie-generate", filepath.FromSlash(rel))
		if !have[p] {
			t.Errorf("missing %s in %v", p, files)
		}
		if _, err := os.Stat(p); err != nil {
			t.Errorf("not on disk: %v", err)
		}
	}
}

func TestSkillsInstallDefaultHome(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	r := f.runWith(t, func(d *cli.Deps) {
		d.Env = func(k string) string {
			if k == "HOME" {
				return home
			}
			return map[string]string{"KIE_API_KEY": "k-secret-123456", "KIE_CLI_HOME": f.home}[k]
		}
	}, "skills", "install")
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "kie-generate", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSkillsInstallUserProfile(t *testing.T) {
	f := newFake(t)
	tmp := t.TempDir()
	r := f.runWith(t, func(d *cli.Deps) {
		d.Env = func(k string) string {
			return map[string]string{"HOME": "", "USERPROFILE": tmp, "KIE_API_KEY": "k-secret-123456", "KIE_CLI_HOME": f.home}[k]
		}
	}, "skills", "install")
	if r.code != 0 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".claude", "skills", "kie-generate", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSkillsInstallNoHome(t *testing.T) {
	f := newFake(t)
	r := f.runWith(t, func(d *cli.Deps) {
		d.Env = func(k string) string { return map[string]string{"KIE_CLI_HOME": f.home}[k] }
	}, "skills", "install")
	if r.code != 2 {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
	if errCode(r.m) != "usage" {
		t.Errorf("out=%s", r.out)
	}
}

func TestSkillsInstallIOError(t *testing.T) {
	f := newFake(t)
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, "skills", "install", "--target", file)
	if r.code != 1 || errCode(r.m) != "io" {
		t.Fatalf("code=%d out=%s", r.code, r.out)
	}
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}
