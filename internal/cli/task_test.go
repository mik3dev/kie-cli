package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
	"github.com/mik3dev/kie-cli/internal/ledger"
)

func TestTaskListEmitsNewestFirstWithLimit(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(home, "tasks.jsonl")
	for _, id := range []string{"a", "b", "c"} {
		if err := ledger.Append(ledgerPath, ledger.Entry{TaskID: id, Model: "m", State: "submitted", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	deps, out, _ := authDeps(t, "", map[string]string{"KIE_CLI_HOME": home})
	if code := cli.Execute(deps, []string{"task", "list", "--limit", "2"}); code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	m := decode(t, out)
	tasks, ok := m["tasks"].([]any)
	if m["ok"] != true || !ok || len(tasks) != 2 {
		t.Fatalf("m = %v", m)
	}
	if first := tasks[0].(map[string]any); first["taskId"] != "c" {
		t.Errorf("first taskId = %v, want c", first["taskId"])
	}
}

func TestTaskListEmptyLedgerReturnsEmptyArray(t *testing.T) {
	deps, out, _ := authDeps(t, "", map[string]string{})
	if code := cli.Execute(deps, []string{"task", "list"}); code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	m := decode(t, out)
	if tasks, ok := m["tasks"].([]any); !ok || len(tasks) != 0 {
		t.Fatalf("tasks = %v, want []", m["tasks"])
	}
}

func TestTaskListRejectsNonPositiveLimit(t *testing.T) {
	deps, out, _ := authDeps(t, "", map[string]string{})
	if code := cli.Execute(deps, []string{"task", "list", "--limit", "0"}); code != 2 {
		t.Fatalf("exit = %d, want 2, out=%s", code, out)
	}
}
