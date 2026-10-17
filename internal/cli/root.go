// Package cli wires the cobra command tree of kie-cli.
package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

// Deps holds the injectable process dependencies of the CLI.
type Deps struct {
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	Env            func(string) string
	Now            func() time.Time
	// Optional test seams: HTTP transport for the API/CDN client (e.g. a TLS
	// fake's certs) and the sleep used by rate limiting and polling.
	Transport http.RoundTripper
	Sleep     func(context.Context, time.Duration) error
}

// NewRoot builds the root command. stdout carries JSON only.
func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "kie-cli",
		Short:         "Generate media via kie.ai (JSON output for agents)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          groupRunE,
	}
	root.SetOut(deps.Stderr)
	root.SetErr(deps.Stderr)
	root.PersistentFlags().Bool("quiet", false, "silence progress and warnings on stderr")
	root.PersistentFlags().Bool("debug", false, "verbose diagnostics on stderr")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &output.Error{Code: output.CodeUsage, Message: err.Error()}
	})
	root.AddCommand(newVersionCmd(deps), newAuthCmd(deps), newModelsCmd(deps), newCreditsCmd(deps), newTaskCmd(deps), newRunCmd(deps))
	return root
}

// groupRunE makes a non-leaf command fail with a usage error (exit 2) instead
// of cobra's default of printing help and exiting 0 with empty stdout.
func groupRunE(_ *cobra.Command, args []string) error {
	return &output.Error{Code: output.CodeUsage, Message: "unknown or missing subcommand: " + strings.Join(args, " ")}
}

// Execute runs the CLI and returns the process exit code.
func Execute(deps Deps, args []string) int {
	// Ctrl-C cancels the context so a running wait can still report its taskId.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	root := NewRoot(deps)
	root.SetContext(ctx)
	return run(root, deps, args)
}

func run(root *cobra.Command, deps Deps, args []string) int {
	root.SetArgs(args)
	// Run every PersistentPreRun in the chain so the root one always fires,
	// even when a subcommand defines its own.
	cobra.EnableTraverseRunHooks = true
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
	var ae *api.Error
	if errors.As(err, &ae) {
		err = ae.AsOutput()
	}
	return output.Fail(deps.Stdout, err)
}
