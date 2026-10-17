package cli

import (
	"time"

	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/schema"
	"github.com/spf13/cobra"
)

const nullSchemaWarning = "schema not synced by kie; run without validation"

func newModelsSchemaCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema <model>",
		Short: "Show the input keys of a model (cached 24h)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, paths, err := newAPIClient(cmd, deps)
			if err != nil {
				return err
			}
			refresh, _ := cmd.Flags().GetBool("refresh")
			raw, _ := cmd.Flags().GetBool("raw")
			now := deps.Now
			if now == nil {
				now = time.Now
			}
			doc, err := schema.Fetch(cmd.Context(), c, paths.Cache, args[0], refresh, now())
			if err != nil {
				return err
			}
			var out map[string]any
			if raw {
				out = map[string]any{"model": args[0], "openapi": doc.OpenAPI}
			} else {
				sum, err := schema.Summarize(doc)
				if err != nil {
					return err
				}
				out = map[string]any{"schema": sum}
			}
			if doc.Null() {
				out["warning"] = nullSchemaWarning
			}
			return output.Success(deps.Stdout, out)
		},
	}
	cmd.Flags().Bool("raw", false, "emit the full OpenAPI document instead of the summary")
	cmd.Flags().Bool("refresh", false, "bypass the cache")
	return cmd
}
