# kie-cli

A CLI built for AI agents to generate images, video and audio through the [kie.ai](https://kie.ai) API. 180+ models come from the live catalog. Every command prints exactly one JSON object on stdout; progress and warnings go to stderr. It ships an agent skill (`kie-generate`) that teaches agents how to drive it.

## Install

**(a) Release archive:** download it from the [releases page](https://github.com/mik3dev/kie-cli/releases), or with the [GitHub CLI](https://cli.github.com):

```sh
gh release download --pattern '*darwin_arm64*' --repo mik3dev/kie-cli
tar -xzf kie-cli_*_darwin_arm64.tar.gz   # Windows: unzip the .zip
# move the binary to a directory on your PATH
```

If macOS Gatekeeper blocks a browser-downloaded binary: `xattr -d com.apple.quarantine ./kie-cli`.

Archives are named `kie-cli_<version>_<os>_<arch>` (`tar.gz`, `zip` on Windows) for `linux`, `darwin` and `windows` on `amd64` and `arm64`. A `checksums.txt` is published with each release.

**(b) Go toolchain:**

```sh
go install github.com/mik3dev/kie-cli/cmd/kie-cli@latest
```

**(c) From a clone:**

```sh
make install
```

## Setup

```sh
kie-cli auth login    # prompts for the API key (or reads the first stdin line)
kie-cli auth status   # where the key comes from (redacted)
kie-cli credits       # remaining balance
```

Alternatively set `KIE_API_KEY`; it takes precedence over the stored key. Config lives in `~/.kie-cli/` (`config.json`, presets, caches). Set `KIE_CLI_HOME` to use another directory. On Windows the home falls back to `USERPROFILE` when `HOME` is unset.

## Quickstart

```sh
kie-cli models search imagen                 # find a model
kie-cli models schema google/imagen4-fast    # its input fields
kie-cli models price google/imagen4-fast     # cost
kie-cli run google/imagen4-fast -p prompt="a red fox" --dry-run
kie-cli run google/imagen4-fast -p prompt="a red fox" --out ./media
```

`--dry-run` validates the input, prints the request body and the price, and submits nothing. A real `run` submits, waits and downloads. On success the output has `taskId`, `model`, `state`, `files`, `ids`, `result`, `creditsConsumed`:

```json
{"taskId":"...","model":"google/imagen4-fast","state":"success","files":[{"path":"media/....png","url":"https://...","kind":"image"}]}
```

Array params take a repeated `-p` (`-p image_urls=a.png -p image_urls=b.png`); a comma list is never split and is treated as one value. Use the local files in `files[].path`. Other useful flags: `--no-wait` (then `kie-cli task wait <taskId>`), `--no-download`, `--timeout`, `--input file.json`. The run command's built-in help lists them all.

## Presets

A preset is a named model plus default params. Factory presets: `imagen-rapida`, `imagen-calidad`, `imagen-edicion`, `video-calidad`, `video-imagen`, `video-inicio-fin`, `video-barato`, `musica`, `instrumental`, `voz`.

```sh
kie-cli preset list
kie-cli preset show imagen-rapida
kie-cli run @imagen-rapida -p prompt="a red fox"
```

Precedence: preset params < `--input` < `-p`.

Add or override presets in `~/.kie-cli/presets.yaml`. A user preset with the same name replaces the factory one entirely:

```yaml
presets:
  - name: logo-video
    description: Short logo animation. Missing fields - prompt.
    model: kling/v2-5-turbo-text-to-video-pro
    params:
      duration: "5"
      aspect_ratio: "16:9"
```

Quote string values in YAML (for example `duration: "5"`): many schemas expect strings and an unquoted number would not match.

## Agent skill

```sh
kie-cli skills install                  # default: ~/.claude/skills
kie-cli skills install --target DIR
```

The `kie-generate` skill teaches an agent the flow (find a model, read its schema, check credits and price, dry-run, run), how to pass input, how to use presets, how to chain tasks with `ids`, how to read the output, and how to react to each error code.

## Output and exit codes

Success prints one JSON object. Failure prints `{"ok":false,"error":{"code","message","details"?}}` and sets the exit code. Errors after a task was created carry `details.taskId`.

| exit | error.code | meaning |
|---|---|---|
| 0 | - | success |
| 2 | `usage` | bad command, flag or preset |
| 2 | `validation` | input does not match the schema, or unknown model |
| 3 | `auth` | missing or invalid API key |
| 4 | `credits` | not enough credits; do not retry |
| 5 | `task_failed` | the model failed the task |
| 6 | `timeout` | still running when the wait ended; the task id is kept for `kie-cli task wait` |
| 7 | `rate_limit` | too many requests; wait, then retry |
| 8 | `submit_uncertain` | the submit may or may not have created a task. Do not resubmit (double charge); check https://kie.ai/logs |
| 1 | `api` | other kie.ai or network error |
| 1 | `io` | local file problem (download, write, read) |

## Development

```sh
make test       # go test -race -cover ./...
make lint       # golangci-lint run ./...
make build      # bin/kie-cli
make snapshot   # local goreleaser build, no publish
make tidy-check # go mod tidy -diff (must print nothing)
```

Tests never call the real API. Live smoke tests run by hand only (build tag `live`, `KIE_E2E=1`) and never in CI. Load the key from a gitignored `.env` without printing it:

```sh
# free checks only (GET endpoints and --dry-run, 0 credits)
set -a; . ./.env; set +a; KIE_E2E=1 go test -tags live -run Live ./internal/cli/ -v
# also the paid tests; KIE_E2E_BUDGET caps credits spent in one run (default 30)
set -a; . ./.env; set +a; KIE_E2E=1 KIE_E2E_PAID=1 KIE_E2E_BUDGET=30 go test -tags live -run Live ./internal/cli/ -v
# refresh testdata/live/records from paid runs (sanitized; refuses to write if the API key appears)
set -a; . ./.env; set +a; KIE_E2E=1 KIE_E2E_PAID=1 KIE_E2E_CAPTURE=1 go test -tags live -run Live ./internal/cli/ -v
```

Releasing: push a tag `vX.Y.Z`. The release workflow runs goreleaser and publishes the archives and `checksums.txt` to GitHub Releases.

## License

[MIT](LICENSE). Files under `testdata/live/` are responses captured from the kie.ai API and belong to kie.ai.
