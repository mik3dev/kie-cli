package ledger_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/ledger"
)

var base = time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)

func tempLedger(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tasks.jsonl")
}

func mustAppend(t *testing.T, path string, e ledger.Entry) {
	t.Helper()
	if err := ledger.Append(path, e); err != nil {
		t.Fatal(err)
	}
}

func TestAppendAndListNewestFirst(t *testing.T) {
	p := tempLedger(t)
	mustAppend(t, p, ledger.Entry{TaskID: "a", Model: "m1", State: "submitted", CreatedAt: base})
	mustAppend(t, p, ledger.Entry{TaskID: "b", Model: "m2", State: "submitted", CreatedAt: base.Add(time.Minute)})
	mustAppend(t, p, ledger.Entry{TaskID: "c", Model: "m3", State: "submitted", CreatedAt: base.Add(2 * time.Minute)})

	got, err := ledger.List(p, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"c", "b", "a"} {
		if got[i].TaskID != want {
			t.Errorf("got[%d].TaskID = %q, want %q", i, got[i].TaskID, want)
		}
	}
	if !got[0].CreatedAt.Equal(base.Add(2 * time.Minute)) {
		t.Errorf("CreatedAt not round-tripped: %v", got[0].CreatedAt)
	}
}

func TestListCollapsesByTaskID(t *testing.T) {
	p := tempLedger(t)
	mustAppend(t, p, ledger.Entry{TaskID: "t1", Model: "m", State: "submitted", CreatedAt: base})
	mustAppend(t, p, ledger.Entry{TaskID: "t1", Model: "m", State: "success", OutDir: "/out", CreatedAt: base})

	got, err := ledger.List(p, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].State != "success" || got[0].OutDir != "/out" {
		t.Errorf("entry = %+v, want state success with outDir", got[0])
	}
}

func TestListKeepsAllEntriesWithoutTaskID(t *testing.T) {
	p := tempLedger(t)
	mustAppend(t, p, ledger.Entry{Model: "m", State: "unknown", Note: "first"})
	mustAppend(t, p, ledger.Entry{Model: "m", State: "unknown", Note: "second"})

	got, err := ledger.List(p, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Note != "second" {
		t.Fatalf("got %+v, want 2 entries newest first", got)
	}
}

func TestListSkipsCorruptLine(t *testing.T) {
	p := tempLedger(t)
	mustAppend(t, p, ledger.Entry{TaskID: "ok1", Model: "m", State: "submitted"})
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	mustAppend(t, p, ledger.Entry{TaskID: "ok2", Model: "m", State: "success"})

	var warn bytes.Buffer
	got, err := ledger.List(p, 0, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TaskID != "ok2" || got[1].TaskID != "ok1" {
		t.Fatalf("got %+v, want ok2, ok1", got)
	}
	if !strings.Contains(warn.String(), "line 2") {
		t.Errorf("warning = %q, want mention of line 2", warn.String())
	}
}

func TestLimit(t *testing.T) {
	p := tempLedger(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		mustAppend(t, p, ledger.Entry{TaskID: id, Model: "m", State: "submitted"})
	}
	got, err := ledger.List(p, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TaskID != "d" || got[1].TaskID != "c" {
		t.Fatalf("got %+v, want d, c", got)
	}
}

func TestListMissingFileIsEmpty(t *testing.T) {
	got, err := ledger.List(filepath.Join(t.TempDir(), "none.jsonl"), 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil slice", got)
	}
}

func TestFilePerm0600(t *testing.T) {
	p := tempLedger(t)
	mustAppend(t, p, ledger.Entry{TaskID: "a", Model: "m", State: "submitted"})
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %04o, want 0600", perm)
	}
}
