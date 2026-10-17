// Package task submits, fetches and waits for kie.ai generation tasks.
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/ledger"
	"github.com/mik3dev/kie-cli/internal/output"
)

const (
	createPath = "/api/v1/jobs/createTask"
	recordPath = "/api/v1/jobs/recordInfo"

	stateSuccess = "success"
	stateFail    = "fail"

	uncertainMsg  = "task may have been created; check https://kie.ai/logs before resubmitting"
	ledgerDirPerm = 0o700
)

// Record is the recordInfo view of a task.
type Record struct {
	TaskID          string
	Model           string
	State           string
	ResultJSON      string
	Response        json.RawMessage
	FailCode        string
	FailMsg         string
	CostTime        int64
	CreditsConsumed float64
	CreateTime      int64
	CompleteTime    int64
}

// UnmarshalJSON accepts failCode as number, string or null.
func (r *Record) UnmarshalJSON(b []byte) error {
	var aux struct {
		TaskID          string          `json:"taskId"`
		Model           string          `json:"model"`
		State           string          `json:"state"`
		ResultJSON      string          `json:"resultJson"`
		Response        json.RawMessage `json:"response"`
		FailCode        json.RawMessage `json:"failCode"`
		FailMsg         string          `json:"failMsg"`
		CostTime        int64           `json:"costTime"`
		CreditsConsumed float64         `json:"creditsConsumed"`
		CreateTime      int64           `json:"createTime"`
		CompleteTime    int64           `json:"completeTime"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	code := strings.TrimSpace(string(aux.FailCode))
	if code == "null" {
		code = ""
	} else if strings.HasPrefix(code, `"`) {
		var s string
		if err := json.Unmarshal(aux.FailCode, &s); err != nil {
			return err
		}
		code = s
	}
	*r = Record{
		TaskID: aux.TaskID, Model: aux.Model, State: aux.State, ResultJSON: aux.ResultJSON,
		Response: aux.Response, FailCode: code, FailMsg: aux.FailMsg, CostTime: aux.CostTime,
		CreditsConsumed: aux.CreditsConsumed, CreateTime: aux.CreateTime, CompleteTime: aux.CompleteTime,
	}
	return nil
}

// Terminal reports whether state is final.
func Terminal(state string) bool { return state == stateSuccess || state == stateFail }

// Get fetches one task. An empty Response is filled from ResultJSON when that parses.
func Get(ctx context.Context, c *api.Client, id string) (Record, error) {
	var rec Record
	req := api.Request{
		Method: http.MethodGet, Path: recordPath, Class: api.ClassRecord,
		Query: url.Values{"taskId": {id}},
	}
	if err := c.Do(ctx, req, &rec); err != nil {
		return Record{}, err
	}
	if (len(rec.Response) == 0 || string(rec.Response) == "null") && rec.ResultJSON != "" {
		if json.Valid([]byte(rec.ResultJSON)) {
			rec.Response = json.RawMessage(rec.ResultJSON)
		}
	}
	if string(rec.Response) == "null" {
		rec.Response = nil
	}
	return rec, nil
}

// Submit creates a task. Never retries beyond api.Client's safe policy. Outcomes
// where the task may exist (sent-and-unanswered, 5xx/455, malformed 2xx, no
// taskId) are recorded as "unknown" and reported as submit_uncertain.
// warn (optional, default stderr) receives ledger-write warnings.
func Submit(ctx context.Context, c *api.Client, ledgerPath, model string, input map[string]any, outDir string, now time.Time, warn ...io.Writer) (string, error) {
	w := io.Writer(os.Stderr)
	if len(warn) > 0 && warn[0] != nil {
		w = warn[0]
	}
	req := api.Request{
		Method: http.MethodPost, Path: createPath, Class: api.ClassCreate,
		Body: map[string]any{"model": model, "input": input},
	}
	var data struct {
		TaskID string `json:"taskId"`
	}
	err := c.Do(ctx, req, &data)
	if err == nil && data.TaskID == "" {
		err = &api.Error{KieCode: 200, Msg: "response lacks taskId"}
		return "", uncertain(ledgerPath, model, outDir, now, err, w)
	}
	if err != nil {
		return "", classifySubmitErr(ledgerPath, model, outDir, now, err, w)
	}
	entry := ledger.Entry{TaskID: data.TaskID, Model: model, State: "submitted", OutDir: outDir, CreatedAt: now}
	if aerr := appendEntry(ledgerPath, entry); aerr != nil {
		_, _ = fmt.Fprintf(w, "warning: task %s was created but could not be recorded in the ledger: %v\n", data.TaskID, aerr)
	}
	return data.TaskID, nil
}

func classifySubmitErr(ledgerPath, model, outDir string, now time.Time, err error, w io.Writer) error {
	var ne *api.NetError
	if errors.As(err, &ne) {
		if ne.Sent {
			return uncertain(ledgerPath, model, outDir, now, err, w)
		}
		return &output.Error{Code: output.CodeAPI, Message: err.Error()}
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		if ae.KieCode >= 500 || ae.KieCode == 455 || malformed2xx(ae) {
			return uncertain(ledgerPath, model, outDir, now, err, w)
		}
		return ae.AsOutput()
	}
	return err
}

// malformed2xx: kie answered 2xx but the body is unusable, so the task may exist.
func malformed2xx(ae *api.Error) bool {
	return ae.KieCode >= 200 && ae.KieCode < 300 &&
		(strings.HasPrefix(ae.Msg, "invalid response envelope") || strings.HasPrefix(ae.Msg, "unexpected data shape"))
}

func uncertain(ledgerPath, model, outDir string, now time.Time, cause error, w io.Writer) error {
	entry := ledger.Entry{Model: model, State: "unknown", OutDir: outDir, CreatedAt: now, Note: cause.Error()}
	if aerr := appendEntry(ledgerPath, entry); aerr != nil {
		_, _ = fmt.Fprintf(w, "warning: could not record uncertain submit in the ledger: %v\n", aerr)
	}
	return &output.Error{Code: output.CodeSubmitUncertain, Message: uncertainMsg}
}

func appendEntry(path string, e ledger.Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), ledgerDirPerm); err != nil {
		return err
	}
	return ledger.Append(path, e)
}
