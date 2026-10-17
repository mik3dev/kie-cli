package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/catalog"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/ledger"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/result"
	"github.com/mik3dev/kie-cli/internal/task"
	"github.com/spf13/cobra"
)

const defaultTaskListLimit = 20

func newTaskCmd(deps Deps) *cobra.Command {
	task := &cobra.Command{Use: "task", Short: "Inspect locally recorded tasks", RunE: groupRunE}
	task.AddCommand(newTaskListCmd(deps), newTaskGetCmd(deps), newTaskWaitCmd(deps))
	return task
}

func newTaskListCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent tasks from the local ledger (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			limit, _ := cmd.Flags().GetInt("limit")
			if limit < 1 {
				return &output.Error{Code: output.CodeUsage, Message: fmt.Sprintf("--limit must be >= 1, got %d", limit)}
			}
			paths, err := config.ResolvePaths(deps.Env)
			if err != nil {
				return err
			}
			tasks, err := ledger.List(paths.Ledger, limit, warnWriter(cmd, deps))
			if err != nil {
				return &output.Error{Code: output.CodeIO, Message: err.Error()}
			}
			return output.Success(deps.Stdout, map[string]any{"tasks": tasks})
		},
	}
	cmd.Flags().Int("limit", defaultTaskListLimit, "maximum number of tasks to list")
	return cmd
}

func newTaskGetCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "get <taskId>",
		Short: "Fetch the current state and result of a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newAPIClient(cmd, deps)
			if err != nil {
				return err
			}
			rec, err := task.Get(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{
				"taskId":          rec.TaskID,
				"model":           rec.Model,
				"state":           rec.State,
				"result":          rec.Response,
				"ids":             result.Extract(rec.TaskID, rec.Response),
				"creditsConsumed": rec.CreditsConsumed,
				"costTime":        rec.CostTime,
				"failCode":        rec.FailCode,
				"failMsg":         rec.FailMsg,
			})
		},
	}
}

func newTaskWaitCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait <taskId>",
		Short: "Wait for a task, then download its results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout < 0 {
				return &output.Error{Code: output.CodeUsage, Message: "--timeout must be positive"}
			}
			c, paths, cfg, err := newAPIClientCfg(cmd, deps)
			if err != nil {
				return err
			}
			known := lookupLedger(paths.Ledger, args[0])
			a := finishArgs{id: args[0], model: known.Model, outDir: resolveOutDir(cmd, cfg), timeout: timeout, createdAt: known.CreatedAt}
			if out, _ := cmd.Flags().GetString("out"); out == "" && known.OutDir != "" && cfg.OutDir == "" {
				a.outDir = known.OutDir
			}
			if a.createdAt.IsZero() {
				a.createdAt = nowFn(deps)()
			}
			if a.timeout == 0 {
				a.timeout = guessTimeout(cmd, deps, c, paths, a.model)
			}
			return finishTask(cmd, deps, c, paths, a)
		},
	}
	addFinishFlags(cmd)
	return cmd
}

// lookupLedger returns the latest ledger entry of the task, or a zero Entry.
func lookupLedger(path, id string) ledger.Entry {
	entries, err := ledger.List(path, 0, nil)
	if err != nil {
		return ledger.Entry{}
	}
	for _, e := range entries {
		if e.TaskID == id {
			return e
		}
	}
	return ledger.Entry{}
}

// guessTimeout uses the model's catalog task types (errors ignored), else its name.
func guessTimeout(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, model string) time.Duration {
	if model == "" {
		return defaultTimeout
	}
	svc := &catalog.Service{API: c, CacheDir: paths.Cache, Now: deps.Now}
	if m, found, err := svc.Find(cmd.Context(), model); err == nil && found {
		return defaultTimeoutFor(m.TaskType)
	}
	if strings.Contains(strings.ToLower(model), "video") {
		return videoTimeout
	}
	return defaultTimeout
}
