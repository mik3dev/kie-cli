package cli

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAuthCmd(deps Deps) *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage the kie.ai API key"}
	auth.AddCommand(newAuthLoginCmd(deps), newAuthStatusCmd(deps))
	return auth
}

func newAuthLoginCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Store the API key (prompt on a TTY, first stdin line otherwise)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := config.ResolvePaths(deps.Env)
			if err != nil {
				return err
			}
			key, err := readKey(deps)
			if err != nil {
				return err
			}
			cfg, err := config.Load(paths, warnWriter(cmd, deps))
			if err != nil {
				return err
			}
			cfg.APIKey = key // cfg is a local copy
			if err := config.Save(paths, cfg); err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"saved": paths.Config})
		},
	}
}

func newAuthStatusCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show where the API key comes from (redacted)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := config.ResolvePaths(deps.Env)
			if err != nil {
				return err
			}
			cfg, err := config.Load(paths, warnWriter(cmd, deps))
			if err != nil {
				return err
			}
			key, source, err := config.ResolveKey(deps.Env, cfg)
			if err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"source": source, "key": config.Redact(key)})
		},
	}
}

// warnWriter returns stderr, or a discard writer when --quiet is set.
func warnWriter(cmd *cobra.Command, deps Deps) io.Writer {
	if quiet, _ := cmd.Flags().GetBool("quiet"); quiet {
		return io.Discard
	}
	return deps.Stderr
}

func readKey(deps Deps) (string, error) {
	var raw string
	if f, ok := deps.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		io.WriteString(deps.Stderr, "API key: ")
		b, err := term.ReadPassword(int(f.Fd()))
		io.WriteString(deps.Stderr, "\n")
		if err != nil {
			return "", &output.Error{Code: output.CodeIO, Message: "read key: " + err.Error()}
		}
		raw = string(b)
	} else {
		line, err := bufio.NewReader(deps.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", &output.Error{Code: output.CodeIO, Message: "read key: " + err.Error()}
		}
		raw = line
	}
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", &output.Error{Code: output.CodeUsage, Message: "empty API key: pipe it on stdin or run on a TTY"}
	}
	return key, nil
}
