package cli

import (
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

// Version is overridden at build time via -ldflags.
var Version = "dev"

func newVersionCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kie-cli version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return output.Success(deps.Stdout, map[string]any{"version": Version})
		},
	}
}
