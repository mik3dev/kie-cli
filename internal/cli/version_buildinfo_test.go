package cli_test

import (
	"runtime/debug"
	"testing"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func stubBuild(t *testing.T, ldflags, mod string, ok bool) {
	t.Helper()
	restore := cli.SetVersionSourcesForTest(ldflags, func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: mod}}, ok
	})
	t.Cleanup(restore)
}

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name, ldflags, mod string
		ok                 bool
		want               string
	}{
		{"ldflags wins", "v1.2.3", "v9.9.9", true, "v1.2.3"},
		{"dev falls back to module version", "dev", "v0.3.0", true, "v0.3.0"},
		{"devel ignored", "dev", "(devel)", true, "dev"},
		{"empty ignored", "dev", "", true, "dev"},
		{"no build info", "dev", "v0.3.0", false, "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubBuild(t, tc.ldflags, tc.mod, tc.ok)
			if got := cli.ResolveVersion(); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
