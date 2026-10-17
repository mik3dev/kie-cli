package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
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
	f.Bool("dry-run", false, "validate and print the request body without uploading or submitting")
	f.Bool("no-validate", false, "send even if the input does not match the schema (problems become warnings)")
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
	var ref presetRef
	if strings.HasPrefix(model, "@") {
		p, err := lookupPreset(deps, model)
		if err != nil {
			return err
		}
		ref = presetRef{name: p.Name, params: p.Params}
		model = p.Model
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
	if isChatModel(m.TaskType) {
		return usageErr(chatMsg)
	}
	if !doc.IsJob() && len(m.TaskType) == 0 {
		// Offline catalog fallback: a non-job schema may be chat or synchronous.
		return usageErr("cannot tell whether %s is a chat or a synchronous model while the catalog is unreachable; retry online", model)
	}
	if doc.Null() {
		_, _ = fmt.Fprintln(warn, "warning: "+nullSchemaWarning)
	}
	prep, err := prepareInput(cmd, deps, doc, model, ref, warn)
	if err != nil {
		return err
	}
	if !doc.IsJob() {
		return runSync(cmd, deps, c, paths, cfg, doc, model, prep, warn)
	}
	if dry, _ := flags.GetBool("dry-run"); dry {
		return emitDryRun(cmd, deps, c, paths, model, prep, dryTarget{body: map[string]any{"model": model, "input": prep.checked}}, warn)
	}
	input, uploaded, err := upload.ResolveLocal(cmd.Context(), prep.input, prep.isFile, func(ctx context.Context, p string) (string, error) {
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
		return output.Success(deps.Stdout, withPreset(map[string]any{
			"taskId": id, "model": model, "state": "submitted", "uploaded": nonNil(uploaded),
			"ids": result.IDs{TaskID: id},
		}, ref.name))
	}
	timeout := timeoutFlag
	if timeout == 0 {
		timeout = defaultTimeoutFor(m.TaskType)
	}
	return finishTask(cmd, deps, c, paths, finishArgs{
		id: id, model: model, outDir: outDir, timeout: timeout, createdAt: now, uploaded: uploaded, preset: ref.name,
	})
}

const (
	pendingUploadHost = "https://pending-upload.invalid/"
	maxGotRunes       = 100
)

type pendingUpload struct {
	Path        string `json:"path"`
	Placeholder string `json:"placeholder"`
}

// preparedInput is the coerced input plus what validation and --dry-run need.
type preparedInput struct {
	input    map[string]any // coerced, local paths untouched
	checked  map[string]any // coerced, local paths replaced by placeholders
	isFile   func(string) bool
	pending  []pendingUpload
	problems []schema.Problem // only set under --no-validate
	preset   string           // preset name when invoked via @name
}

// withPreset returns out with "preset" set when the run came from a preset.
func withPreset(out map[string]any, name string) map[string]any {
	if name != "" {
		out["preset"] = name
	}
	return out
}

// prepareInput builds, coerces and validates the input without any network access.
// Invalid input is a validation error unless --no-validate is set.
func prepareInput(cmd *cobra.Command, deps Deps, doc schema.Doc, model string, ref presetRef, warn io.Writer) (preparedInput, error) {
	p := preparedInput{preset: ref.name}
	sch, err := doc.InputSchema()
	if err != nil {
		_, _ = fmt.Fprintf(warn, "warning: cannot read the input schema (%v); -p values are typed by guess\n", err)
		sch = nil
	}
	raw, err := buildInput(cmd, deps, sch, ref.params)
	if err != nil {
		return p, err
	}
	p.input = schema.Coerce(sch, raw)
	p.isFile = fileKeyFunc(sch)
	var given []string
	p.checked, given, err = upload.ResolveLocal(cmd.Context(), p.input, p.isFile, func(_ context.Context, path string) (string, error) {
		ph := pendingUploadHost + url.PathEscape(filepath.Base(path))
		p.pending = append(p.pending, pendingUpload{Path: path, Placeholder: ph})
		return ph, nil
	})
	if err != nil {
		return p, err
	}
	for i := range p.pending { // report paths as the user wrote them
		if i < len(given) {
			p.pending[i].Path = given[i]
		}
	}
	sort.SliceStable(p.pending, func(i, j int) bool { return p.pending[i].Path < p.pending[j].Path })
	if sch == nil {
		return p, nil
	}
	problems, err := schema.Validate(sch, p.checked)
	if err != nil {
		_, _ = fmt.Fprintf(warn, "warning: cannot validate the input (%v); sending without validation\n", err)
		return p, nil
	}
	problems = capGot(problems)
	if len(problems) == 0 {
		return p, nil
	}
	if noValidate, _ := cmd.Flags().GetBool("no-validate"); !noValidate {
		return p, &output.Error{
			Code: output.CodeValidation,
			Message: fmt.Sprintf("input does not match the schema of %s (%s); fix the fields, or use --no-validate if the schema is wrong",
				model, problemCount(len(problems))),
			Details: map[string]any{"problems": problems},
		}
	}
	for _, pr := range problems {
		_, _ = fmt.Fprintf(warn, "warning: validation: %s: %s\n", pr.Field, pr.Message)
	}
	p.problems = problems
	return p, nil
}

func problemCount(n int) string {
	if n == 1 {
		return "1 problem"
	}
	return fmt.Sprintf("%d problems", n)
}

// capGot returns a copy of problems with string "got" values cut to maxGotRunes runes.
func capGot(problems []schema.Problem) []schema.Problem {
	out := make([]schema.Problem, len(problems))
	for i, pr := range problems {
		if s, ok := pr.Got.(string); ok {
			if r := []rune(s); len(r) > maxGotRunes {
				pr.Got = string(r[:maxGotRunes])
			}
		}
		out[i] = pr
	}
	return out
}

// dryTarget is the request --dry-run reports; path is set for synchronous endpoints only.
type dryTarget struct {
	path string
	body map[string]any
}

// emitDryRun prints the request that would be sent; nothing is uploaded or submitted.
func emitDryRun(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, model string, p preparedInput, t dryTarget, warn io.Writer) error {
	out := withPreset(map[string]any{
		"dryRun": true, "model": model,
		"body":           t.body,
		"pendingUploads": p.pending,
	}, p.preset)
	if t.path != "" {
		out["path"] = t.path
	}
	if p.pending == nil {
		out["pendingUploads"] = []pendingUpload{}
	}
	svc := &catalog.Service{API: c, CacheDir: paths.Cache, Now: deps.Now}
	if desc, err := svc.Price(cmd.Context(), model); err != nil {
		_, _ = fmt.Fprintf(warn, "warning: could not get the price (%v)\n", err)
	} else {
		out["pricing"] = desc
	}
	if len(p.problems) > 0 {
		out["problems"] = p.problems
	}
	return output.Success(deps.Stdout, out)
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

// buildInput merges the preset params, --input and the -p values (later wins). Plain -p values for keys the
// schema does not declare are typed by guess; @file contents and --input
// values stay as they are.
func buildInput(cmd *cobra.Command, deps Deps, sch, presetParams map[string]any) (map[string]any, error) {
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
	tagged, err := parseTagged(pairs, os.ReadFile)
	if err != nil {
		return nil, err
	}
	undeclared := map[string]any{}
	for k, v := range tagged {
		if _, declared := schema.PropSchema(sch, k); !declared {
			undeclared[k] = v
		}
	}
	over := unwrapFileValues(MergeInput(tagged, schema.GuessLiterals(undeclared)))
	return MergeInput(MergeInput(presetParams, base), over), nil
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
	preset            string
}

// finishTask waits for the task, records the final state, downloads and emits.
// Every error carries details.taskId so a paid task is never lost.
func finishTask(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, a finishArgs) error {
	warn := warnWriter(cmd, deps)
	w := task.Waiter{Sleep: deps.Sleep, Now: deps.Now, Progress: warn}
	wait := w.Wait
	if a.model == task.RecoveryModel {
		wait = w.WaitRecovery
	}
	rec, err := wait(cmd.Context(), c, a.id, a.timeout)
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
	return output.Success(deps.Stdout, withPreset(map[string]any{
		"taskId": a.id, "model": a.model, "state": rec.State,
		"files": files, "result": rec.Response, "creditsConsumed": rec.CreditsConsumed,
		"costTime": rec.CostTime, "uploaded": nonNil(a.uploaded),
		"ids": result.Extract(a.id, rec.Response),
	}, a.preset))
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
		_, _ = fmt.Fprintf(warn, "warning: could not record task %s in the ledger: %v\n", a.id, err)
		return
	}
	e := ledger.Entry{TaskID: a.id, Model: a.model, State: state, OutDir: a.outDir, CreatedAt: a.createdAt, Note: note}
	if err := ledger.Append(paths.Ledger, e); err != nil {
		_, _ = fmt.Fprintf(warn, "warning: could not record task %s in the ledger: %v\n", a.id, err)
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
