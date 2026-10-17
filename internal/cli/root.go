// Package cli wires the cobra command tree of kie-cli.
package cli

import (
	"io"
	"time"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

// Deps holds the injectable process dependencies of the CLI.
type Deps struct {
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	Env            func(string) string
	Now            func() time.Time
}

// NewRoot builds the root command. stdout carries JSON only.
func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "kie-cli",
		Short:         "Generate media via kie.ai (JSON output for agents)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(deps.Stderr)
	root.SetErr(deps.Stderr)
	root.PersistentFlags().Bool("quiet", false, "silence progress and warnings on stderr")
	root.PersistentFlags().Bool("debug", false, "verbose diagnostics on stderr")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &output.Error{Code: output.CodeUsage, Message: err.Error()}
	})
	root.AddCommand(newVersionCmd(deps))
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute(deps Deps, args []string) int {
	root := NewRoot(deps)
	root.SetArgs(args)
	started := false
	root.PersistentPreRun = func(*cobra.Command, []string) { started = true }
	err := root.Execute()
	if err == nil {
		return 0
	}
	// Errors raised before any command ran (unknown command, bad args) are usage errors.
	if _, ok := err.(*output.Error); !ok && !started {
		err = &output.Error{Code: output.CodeUsage, Message: err.Error()}
	}
	return output.Fail(deps.Stdout, err)
}
