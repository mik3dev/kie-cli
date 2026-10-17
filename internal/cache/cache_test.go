package cache_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cache"
)

func TestPutGetRoundTripAndSlashName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	if err := cache.Put(dir, "a/b", map[string]int{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a__b.json")); err != nil {
		t.Fatalf("file name: %v", err)
	}
	var got map[string]int
	hit, err := cache.Get(dir, "a/b", time.Hour, time.Now(), &got)
	if err != nil || !hit || got["x"] != 1 {
		t.Fatalf("hit=%v err=%v got=%v", hit, err, got)
	}
}

func TestGetMissAndExpired(t *testing.T) {
	dir := t.TempDir()
	var v int
	if hit, err := cache.Get(dir, "none", time.Hour, time.Now(), &v); hit || err != nil {
		t.Fatalf("miss: hit=%v err=%v", hit, err)
	}
	if err := cache.Put(dir, "n", 5); err != nil {
		t.Fatal(err)
	}
	if hit, err := cache.Get(dir, "n", time.Hour, time.Now().Add(2*time.Hour), &v); hit || err != nil {
		t.Fatalf("expired: hit=%v err=%v", hit, err)
	}
}

func TestCorruptFileIsMiss(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{nope"), 0o600)
	var v int
	if hit, err := cache.Get(dir, "bad", time.Hour, time.Now(), &v); hit || err != nil {
		t.Fatalf("corrupt: hit=%v err=%v", hit, err)
	}
}

func TestPutErrors(t *testing.T) {
	if err := cache.Put(t.TempDir(), "x", make(chan int)); err == nil {
		t.Error("unencodable value: want error")
	}
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	if err := cache.Put(f, "x", 1); err == nil {
		t.Error("dir is a file: want error")
	}
}

func TestGetStatError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	var v int
	if _, err := cache.Get(f, "x", time.Hour, time.Now(), &v); err == nil {
		t.Error("ENOTDIR: want error")
	}
}
