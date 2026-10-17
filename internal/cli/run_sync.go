package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mik3dev/kie-cli/internal/api"
	"github.com/mik3dev/kie-cli/internal/config"
	"github.com/mik3dev/kie-cli/internal/download"
	"github.com/mik3dev/kie-cli/internal/output"
	"github.com/mik3dev/kie-cli/internal/result"
	"github.com/mik3dev/kie-cli/internal/schema"
	"github.com/mik3dev/kie-cli/internal/task"
	"github.com/mik3dev/kie-cli/internal/upload"
	"github.com/spf13/cobra"
)

const (
	chatMsg    = "chat models are out of scope for kie-cli; the agent is the LLM"
	noSyncID   = "sync"
	noWaitSync = "warning: --no-wait ignored: synchronous models return immediately"
)

// isChatModel reports whether any catalog taskType marks a chat model.
func isChatModel(taskTypes []string) bool {
	for _, t := range taskTypes {
		if strings.Contains(strings.ToLower(t), "chat") {
			return true
		}
	}
	return false
}

// runSync runs a model whose endpoint is not createTask: the validated input
// is POSTed as the body, the answer is immediate and no ledger entry exists.
func runSync(cmd *cobra.Command, deps Deps, c *api.Client, paths config.Paths, cfg config.Config,
	doc schema.Doc, model string, prep preparedInput, warn io.Writer) error {
	method, path, err := doc.Endpoint()
	if err != nil {
		return usageErr("cannot read the endpoint of %s: %v", model, err)
	}
	flags := cmd.Flags()
	if dry, _ := flags.GetBool("dry-run"); dry {
		return emitDryRun(cmd, deps, c, paths, model, prep, dryTarget{path: path, body: prep.checked}, warn)
	}
	if noWait, _ := flags.GetBool("no-wait"); noWait {
		_, _ = fmt.Fprintln(warn, noWaitSync)
	}
	input, uploaded, err := upload.ResolveLocal(cmd.Context(), prep.input, prep.isFile, func(ctx context.Context, p string) (string, error) {
		return upload.File(ctx, c, p)
	})
	if err != nil {
		return err
	}
	sort.Strings(uploaded)
	data, err := task.Call(cmd.Context(), c, method, path, input)
	if err != nil {
		return err
	}
	ids := result.SyncIDs(data)
	urls := result.MediaURLs(data)
	files := []download.File{}
	if noDl, _ := flags.GetBool("no-download"); !noDl && len(urls) > 0 {
		outDir := resolveOutDir(cmd, cfg)
		if files, err = download.All(cmd.Context(), c, urls, outDir, model, syncFileID(ids)); err != nil {
			return syncDownloadErr(err, urls, data)
		}
	}
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	return output.Success(deps.Stdout, withPreset(map[string]any{
		"model": model, "state": "success", "sync": true, "result": data,
		"files": files, "ids": ids, "uploaded": nonNil(uploaded),
	}, prep.preset))
}

// syncFileID names downloaded files after the id the response created.
func syncFileID(ids result.IDs) string {
	switch {
	case len(ids.AudioIDs) > 0:
		return ids.AudioIDs[0]
	case len(ids.CharacterIDs) > 0:
		return ids.CharacterIDs[0]
	}
	return noSyncID
}

// syncDownloadErr reports a failed download as io, keeping the response and
// its URLs so the created resource is not lost.
func syncDownloadErr(err error, urls []string, data json.RawMessage) error {
	var oe *output.Error
	if !errors.As(err, &oe) {
		oe = &output.Error{Code: output.CodeIO, Message: err.Error()}
	}
	details := make(map[string]any, len(oe.Details)+2)
	for k, v := range oe.Details {
		details[k] = v
	}
	details["resultUrls"] = urls
	details["result"] = data
	return &output.Error{Code: output.CodeIO, Message: oe.Message, Details: details}
}
