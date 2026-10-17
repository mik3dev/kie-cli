package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

func runWithSub(t *testing.T, runErr error) (int, string) {
	t.Helper()
	var out, errw bytes.Buffer
	deps := Deps{Stdout: &out, Stderr: &errw, Stdin: strings.NewReader(""), Env: func(string) string { return "" }, Now: time.Now}
	root := NewRoot(deps)
	root.AddCommand(&cobra.Command{
		Use:              "sub",
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE:             func(*cobra.Command, []string) error { return runErr },
	})
	return run(root, deps, []string{"sub"}), out.String()
}

func TestPlainRunErrorWithOwnPreRunIsAPI(t *testing.T) {
	code, out := runWithSub(t, errors.New("boom"))
	if code != 1 || !strings.Contains(out, `"code":"api"`) {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestTypedRunErrorKeepsExitCode(t *testing.T) {
	code, out := runWithSub(t, &output.Error{Code: output.CodeCredits, Message: "low"})
	if code != 4 || !strings.Contains(out, `"code":"credits"`) {
		t.Errorf("code=%d out=%q", code, out)
	}
}
