package cli

import (
	"encoding/json"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/spf13/cobra"
)

func newCreditsCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "credits",
		Short: "Show the remaining credit balance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := newAPIClient(cmd, deps)
			if err != nil {
				return err
			}
			var credits json.Number
			req := api.Request{Method: "GET", Path: "/api/v1/chat/credit"}
			if err := c.Do(cmd.Context(), req, &credits); err != nil {
				return err
			}
			return output.Success(deps.Stdout, map[string]any{"credits": credits})
		},
	}
}
