package cli

import (
	"runtime/debug"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

// Version is overridden at build time via -ldflags.
var Version = "dev"

// readBuildInfo is injectable for tests.
var readBuildInfo = debug.ReadBuildInfo

// resolveVersion prefers the ldflags value; for "dev" builds (e.g. go install
// @version) it falls back to the module version embedded by the Go toolchain.
func resolveVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := readBuildInfo(); ok && info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Version
}

func newVersionCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kie-cli version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return output.Success(deps.Stdout, map[string]any{"version": resolveVersion()})
		},
	}
}
