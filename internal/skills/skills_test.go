package skills_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/mik3dev/kie-cli/internal/skills"
)

func TestInstallCopiesTree(t *testing.T) {
	src := fstest.MapFS{
		"s/k/SKILL.md":        {Data: []byte("skill")},
		"s/k/references/a.md": {Data: []byte("ref")},
		"s/other/ignored.md":  {Data: []byte("no")},
	}
	target := filepath.Join(t.TempDir(), "skills")
	got, err := skills.Install(src, "s/k", target)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(target, "k", "SKILL.md"), filepath.Join(target, "k", "references", "a.md")}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	b, err := os.ReadFile(want[0])
	if err != nil || len(b) == 0 {
		t.Fatalf("read: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "other")); err == nil {
		t.Fatal("copied outside root")
	}
	fi, _ := os.Stat(filepath.Join(target, "k", "references"))
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("dir perm %v", fi.Mode().Perm())
	}
	fi, _ = os.Stat(want[0])
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("file perm %v", fi.Mode().Perm())
	}
}

func TestInstallOverwritesKeepsSiblings(t *testing.T) {
	target := t.TempDir()
	old := filepath.Join(target, "k", "SKILL.md")
	sibling := filepath.Join(target, "otra-skill", "x.md")
	for _, p := range []string{old, sibling} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := fstest.MapFS{"s/k/SKILL.md": {Data: []byte("new")}}
	if _, err := skills.Install(src, "s/k", target); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(old); string(b) != "new" {
		t.Errorf("not overwritten: %q", b)
	}
	if b, _ := os.ReadFile(sibling); string(b) != "old" {
		t.Errorf("sibling touched: %q", b)
	}
}

func TestInstallTargetIsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := fstest.MapFS{"s/k/SKILL.md": {Data: []byte("new")}}
	if _, err := skills.Install(src, "s/k", f); err == nil {
		t.Fatal("want error")
	}
}

func TestInstallMissingRoot(t *testing.T) {
	if _, err := skills.Install(fstest.MapFS{}, "s/k", t.TempDir()); err == nil {
		t.Fatal("want error")
	}
}
