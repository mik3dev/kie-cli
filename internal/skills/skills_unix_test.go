//go:build unix

package skills_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/mik3dev/kie-cli/internal/skills"
)

func TestInstallPermissions(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	src := fstest.MapFS{"s/k/SKILL.md": {Data: []byte("skill")}, "s/k/references/a.md": {Data: []byte("ref")}}
	target := filepath.Join(t.TempDir(), "skills")
	got, err := skills.Install(src, "s/k", target)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(target, "k", "references"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("dir perm %v", fi.Mode().Perm())
	}
	fi, err = os.Stat(got[0])
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("file perm %v", fi.Mode().Perm())
	}
}
