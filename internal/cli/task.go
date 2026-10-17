package cli

import (
	"fmt"

	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/ledger"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/task"
	"github.com/spf13/cobra"
)

const defaultTaskListLimit = 20

func newTaskCmd(deps Deps) *cobra.Command {
	task := &cobra.Command{Use: "task", Short: "Inspect locally recorded tasks"}
	task.AddCommand(newTaskListCmd(deps), newTaskGetCmd(deps))
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
				"creditsConsumed": rec.CreditsConsumed,
				"costTime":        rec.CostTime,
				"failCode":        rec.FailCode,
				"failMsg":         rec.FailMsg,
			})
		},
	}
}
