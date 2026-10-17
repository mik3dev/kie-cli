package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
)

const (
	// RecoveryModel is the Suno recovery model; it is created through createTask
	// but polled on its own endpoint.
	RecoveryModel = "ai-music-api/suno-recovery-audio"

	recoveryPath = "/api/v1/suno/recovery/record-info"

	stateGenerating = "generating"
	kieRunning      = 201
	kieAllFailed    = 500
)

// GetRecovery fetches a Suno recovery task once. kie code 201 maps to state
// "generating", 200 to "success" (Response = {"data": data}) and 500 to "fail".
func GetRecovery(ctx context.Context, c *api.Client, id string) (Record, error) {
	req := api.Request{
		Method: http.MethodGet, Path: recoveryPath, Class: api.ClassRecord,
		Query: url.Values{"task_id": {id}},
	}
	var data json.RawMessage
	err := c.Do(ctx, req, &data)
	var ae *api.Error
	switch {
	case err == nil:
		resp, merr := json.Marshal(map[string]json.RawMessage{"data": nullIfEmpty(data)})
		if merr != nil {
			return Record{}, merr
		}
		return Record{TaskID: id, Model: RecoveryModel, State: stateSuccess, Response: resp}, nil
	case errors.As(err, &ae) && ae.KieCode == kieRunning:
		return Record{TaskID: id, Model: RecoveryModel, State: stateGenerating}, nil
	case errors.As(err, &ae) && ae.KieCode == kieAllFailed:
		return Record{
			TaskID: id, Model: RecoveryModel, State: stateFail,
			FailCode: strconv.Itoa(ae.KieCode), FailMsg: ae.Msg,
		}, nil
	}
	return Record{}, err
}

func nullIfEmpty(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

// WaitRecovery polls the recovery endpoint with the same backoff, timeout and
// cancellation behavior as Wait.
func (w Waiter) WaitRecovery(ctx context.Context, c *api.Client, taskID string, timeout time.Duration) (Record, error) {
	return w.poll(ctx, taskID, timeout, func() (Record, error) { return GetRecovery(ctx, c, taskID) })
}
