// Package cache stores JSON values on disk with a TTL based on file mtime.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func path(dir, name string) string {
	return filepath.Join(dir, strings.ReplaceAll(name, "/", "__")+".json")
}

// Get decodes the cached value into `into` when it is younger than ttl at `now`.
// A missing, expired or corrupt entry is a miss (hit=false, nil error).
func Get(dir, name string, ttl time.Duration, now time.Time, into any) (bool, error) {
	p := path(dir, name)
	st, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat cache %s: %w", p, err)
	}
	if now.Sub(st.ModTime()) >= ttl {
		return false, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false, fmt.Errorf("read cache %s: %w", p, err)
	}
	if json.Unmarshal(b, into) != nil {
		return false, nil
	}
	return true, nil
}

// Put writes v atomically (dir created 0700, file 0600).
func Put(dir, name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	if err := os.Rename(tmp.Name(), path(dir, name)); err != nil {
		return fmt.Errorf("replace cache: %w", err)
	}
	return nil
}
