package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/catalog"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/download"
	"github.com/mik3dev/kie-cli/internal/ledger"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/result"
	"github.com/mik3dev/kie-cli/internal/schema"
	"github.com/mik3dev/kie-cli/internal/task"
	"github.com/mik3dev/kie-cli/internal/upload"
	"github.com/spf13/cobra"
)

const (
	videoTimeout   = 30 * time.Minute
	defaultTimeout = 10 * time.Minute
	fallbackOutDir = "./kie-output"
)

func nowFn(deps Deps) func() time.Time {
	if deps.Now != nil {
		return deps.Now
	}
	return time.Now
}

func usageErr(format string, args ...any) *output.Error {
	return &output.Error{Code: output.CodeUsage, Message: fmt.Sprintf(format, args...)}
}

func newRunCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <model>",
		Short: "Submit a generation task, wait for it and download the results",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runModel(cmd, deps, args[0]) },
	}
	f := cmd.Flags()
	f.StringArrayP("param", "p", nil, "input field as key=value (repeatable; @file reads a file, @@ escapes @)")
	f.String("input", "", "JSON object file with the input ('-' for stdin); -p overrides it")
	addFinishFlags(cmd)
	f.Bool("no-wait", false, "submit and return the taskId without waiting")
	return cmd
}

// addFinishFlags adds the flags shared by `run` and `task wait`.
func addFinishFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("out", "", "download directory (default: config out_dir or ./kie-output)")
	f.Bool("no-download", false, "do not download result files")
	f.Duration("timeout", 0, "max time to wait (default 10m, 30m for video models)")
}

func runModel(cmd *cobra.Command, deps Deps, model string) error {
	if strings.HasPrefix(model, "@") {
		return usageErr("presets arrive in stage 5")
	}
	flags := cmd.Flags()
	timeoutFlag, _ := flags.GetDuration("timeout")
	if timeoutFlag < 0 {
		return usageErr("--timeout must be positive, got %s", timeoutFlag)
	}
	c, paths, cfg, err := newAPIClientCfg(cmd, deps)
	if err != nil {
		return err
	}
	warn := warnWriter(cmd, deps)
	m, err := findModel(cmd, deps, c, paths, model, warn)
	if err != nil {
		return err
	}
	doc, err := schema.Fetch(cmd.Context(), c, paths.Cache, model, false, nowFn(deps)())
	if err != nil {
		return err
	}
	if !doc.IsJob() {
		return usageErr("%s is a synchronous model: synchronous models arrive in stage 4", model)
	}
	if doc.Null() {
		_, _ = fmt.Fprintln(warn, "warning: "+nullSchemaWarning)
	}
	input, err := buildInput(cmd, deps)
	if err != nil {
		return err
	}
	input, uploaded, err := upload.ResolveLocal(cmd.Context(), input, func(ctx context.Context, p string) (string, error) {
		return upload.File(ctx, c, p)
	})
	if err != nil {
		return err
	}
	sort.Strings(uploaded)
	if err := config.EnsureHome(paths); err != nil {
		return &output.Error{Code: output.CodeIO, Message: err.Error()}
	}
	outDir := resolveOutDir(cmd, cfg)
	now := nowFn(deps)()
	id, err := task.Submit(cmd.Context(), c, paths.Ledger, model, input, outDir, now, warn)
	if err != nil {
		return err
	}
	if noWait, _ := flags.GetBool("no-wait"); noWait {
		return output.Success(deps.Stdout, map[string]any{
			"taskId": id, "model": model, "state": "submitted", "uploaded": nonNil(uploaded),
		})
	}
	timeout := timeoutFlag
	if timeout == 0 {
		timeout = defaultTimeoutFor(m.TaskType)
	}
	return finishTask(cmd, deps, c, paths, finishArgs{
		id: id, model: model, outDir: outDir, timeout: timeout, createdAt: now, uploaded: uploaded,
	})
}

// findModel checks the catalog; a network failure only warns, other errors abort.
func findModel(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, model string, warn io.Writer) (catalog.Model, error) {
	svc := &catalog.Service{API: c, CacheDir: paths.Cache, Now: deps.Now}
	m, found, err := svc.Find(cmd.Context(), model)
	var ne *api.NetError
	if errors.As(err, &ne) {
		_, _ = fmt.Fprintf(warn, "warning: could not check the catalog (%v); continuing\n", err)
		return catalog.Model{Model: model}, nil
	}
	if err != nil {
		return m, err
	}
	if !found {
		return m, &output.Error{Code: output.CodeValidation, Message: "unknown model; run 'kie-cli models search'"}
	}
	return m, nil
}

func buildInput(cmd *cobra.Command, deps Deps) (map[string]any, error) {
	flags := cmd.Flags()
	var base map[string]any
	if path, _ := flags.GetString("input"); path != "" {
		var data []byte
		var err error
		if path == "-" {
			data, err = io.ReadAll(deps.Stdin)
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, usageErr("cannot read --input: %v", err)
		}
		if err := json.Unmarshal(data, &base); err != nil || base == nil {
			return nil, usageErr("--input must be a JSON object")
		}
	}
	pairs, _ := flags.GetStringArray("param")
	over, err := ParseParams(pairs, os.ReadFile)
	if err != nil {
		return nil, err
	}
	return MergeInput(base, over), nil
}

func resolveOutDir(cmd *cobra.Command, cfg config.Config) string {
	if out, _ := cmd.Flags().GetString("out"); out != "" {
		return out
	}
	if cfg.OutDir != "" {
		return cfg.OutDir
	}
	return fallbackOutDir
}

func defaultTimeoutFor(taskTypes []string) time.Duration {
	for _, t := range taskTypes {
		if strings.Contains(t, "Video") {
			return videoTimeout
		}
	}
	return defaultTimeout
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type finishArgs struct {
	id, model, outDir string
	timeout           time.Duration
	createdAt         time.Time
	uploaded          []string
}

// finishTask waits for the task, records the final state, downloads and emits.
// Every error carries details.taskId so a paid task is never lost.
func finishTask(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, a finishArgs) error {
	warn := warnWriter(cmd, deps)
	w := task.Waiter{Sleep: deps.Sleep, Now: deps.Now, Progress: warn}
	rec, err := w.Wait(cmd.Context(), c, a.id, a.timeout)
	a.model = firstNonEmpty(rec.Model, a.model)
	if err != nil {
		if rec.State == "fail" {
			recordFinal(paths, a, "fail", rec.FailMsg, warn)
		}
		return withTaskID(err, a.id, nil)
	}
	recordFinal(paths, a, "success", "", warn)
	urls := result.URLs(rec.Response)
	files := []download.File{}
	if noDl, _ := cmd.Flags().GetBool("no-download"); !noDl && len(urls) > 0 {
		if files, err = download.All(cmd.Context(), c, urls, a.outDir, a.model, a.id); err != nil {
			return withTaskID(err, a.id, map[string]any{"resultUrls": urls, "code": output.CodeIO})
		}
	}
	return output.Success(deps.Stdout, map[string]any{
		"taskId": a.id, "model": a.model, "state": rec.State,
		"files": files, "result": rec.Response, "creditsConsumed": rec.CreditsConsumed,
		"costTime": rec.CostTime, "uploaded": nonNil(a.uploaded),
	})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// recordFinal appends the final state; failures only warn (the task result is already known).
func recordFinal(paths config.Paths, a finishArgs, state, note string, warn io.Writer) {
	if err := config.EnsureHome(paths); err != nil {
		fmt.Fprintf(warn, "warning: could not record task %s in the ledger: %v\n", a.id, err)
		return
	}
	e := ledger.Entry{TaskID: a.id, Model: a.model, State: state, OutDir: a.outDir, CreatedAt: a.createdAt, Note: note}
	if err := ledger.Append(paths.Ledger, e); err != nil {
		fmt.Fprintf(warn, "warning: could not record task %s in the ledger: %v\n", a.id, err)
	}
}

// withTaskID returns a new *output.Error from err with details.taskId (and extra)
// added. Non-output errors become code "api" keeping the message. A "code" key in
// extra overrides the error code (used for download failures -> io).
func withTaskID(err error, id string, extra map[string]any) error {
	var oe *output.Error
	var ae *api.Error
	switch {
	case errors.As(err, &oe):
	case errors.As(err, &ae):
		oe = ae.AsOutput()
	default:
		oe = &output.Error{Code: output.CodeAPI, Message: err.Error()}
	}
	details := make(map[string]any, len(oe.Details)+len(extra)+1)
	for k, v := range oe.Details {
		details[k] = v
	}
	code := oe.Code
	for k, v := range extra {
		if k == "code" {
			code = v.(string)
			continue
		}
		details[k] = v
	}
	details["taskId"] = id
	return &output.Error{Code: code, Message: oe.Message, Details: details}
}
