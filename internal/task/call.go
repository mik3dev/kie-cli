package task

import (
	"context"
	"encoding/json"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/output"
)

// Call sends a synchronous (non-task) request and returns the envelope data.
// It shares Submit's error policy: retries only 429 or never-sent requests,
// and an ambiguous outcome is submit_uncertain. There is no task, so nothing
// is written to the ledger.
func Call(ctx context.Context, c *api.Client, method, path string, body map[string]any) (json.RawMessage, error) {
	req := api.Request{Method: method, Path: path, Class: api.ClassCreate, Body: body}
	var data json.RawMessage
	if err := c.Do(ctx, req, &data); err != nil {
		mapped, ambiguous := classifyCreateErr(err)
		if ambiguous {
			return nil, &output.Error{Code: output.CodeSubmitUncertain, Message: uncertainMsg}
		}
		return nil, mapped
	}
	return data, nil
}
