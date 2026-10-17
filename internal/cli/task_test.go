package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestTaskGetEmitsRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("taskId") != "t1" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":{"taskId":"t1","model":"m","state":"success","resultJson":"{\"resultUrls\":[\"u\"]}","creditsConsumed":2,"costTime":5,"failCode":null}}`)
	}))
	defer srv.Close()
	deps, out, _ := authDeps(t, "", map[string]string{"KIE_API_KEY": "k-123456789", "KIE_BASE_URL": srv.URL})
	if code := cli.Execute(deps, []string{"task", "get", "t1"}); code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	m := decode(t, out)
	res, _ := m["result"].(map[string]any)
	if m["ok"] != true || m["taskId"] != "t1" || m["state"] != "success" || m["creditsConsumed"] != float64(2) || res["resultUrls"] == nil {
		t.Fatalf("m = %v", m)
	}
}

func TestTaskGetNoResultEmitsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":{"taskId":"t1","state":"generating"}}`)
	}))
	defer srv.Close()
	deps, out, _ := authDeps(t, "", map[string]string{"KIE_API_KEY": "k-123456789", "KIE_BASE_URL": srv.URL})
	if code := cli.Execute(deps, []string{"task", "get", "t1"}); code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	if m := decode(t, out); m["result"] != nil || m["state"] != "generating" {
		t.Fatalf("m = %v", m)
	}
}

func TestTaskGetRequiresArg(t *testing.T) {
	deps, _, _ := authDeps(t, "", map[string]string{})
	if code := cli.Execute(deps, []string{"task", "get"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}
