// Package skills installs the bundled agent skill into a skills directory.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// Install copies src/root/** to target/<base(root)>/** and returns the written
// paths, sorted. Existing files are overwritten; nothing else in target is
// touched.
func Install(src fs.FS, root, target string) ([]string, error) {
	dest := filepath.Join(target, path.Base(root))
	var written []string
	err := fs.WalkDir(src, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := p[len(root):]
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, dirPerm)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, data, filePerm); err != nil {
			return err
		}
		written = append(written, out)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("install skill %s into %s: %w", root, target, err)
	}
	sort.Strings(written)
	return written, nil
}
