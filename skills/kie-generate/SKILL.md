---
name: kie-generate
description: Generate or edit images, video, music, speech and sound effects with kie.ai through the `kie-cli` tool. Use when the user asks to generate/edit an image, make a video (text-to-video, image-to-video, first/last frame), compose a song or instrumental, synthesize speech, or create sound, using kie.ai models.
---

# kie-generate

Drive kie.ai (image, video, audio models) with the `kie-cli` binary. Every command prints exactly one JSON object on stdout; progress and warnings go to stderr. Parse stdout, ignore stderr.

This skill is guidance only. The model's own schema is the source of truth: never invent models or fields.

## Flow

1. Find a model: `kie-cli models search <text>` or `kie-cli models list --type video` (`--type`/`--provider` filter). Ids look like `google/imagen4-fast`, `veo-3-1`, `ai-music-api/generate`.
2. Read its input: `kie-cli models schema <model>`. Use only the fields it lists, with their types and enums. `--refresh` bypasses the 24h cache; `--raw` gives the full OpenAPI document.
3. Check cost when it matters (always for video): `kie-cli credits` and `kie-cli models price <model>`.
4. Optional rehearsal: `kie-cli run <model> -p prompt=... --dry-run` validates the input, prints the request body and the price, and submits nothing.
5. Generate: `kie-cli run <model> -p prompt="a red fox" -p aspect_ratio=16:9 --out ./media`.
6. Use the files listed in `files[].path`.

## Passing input

- `-p key=value`, repeatable. Values are typed against the schema (numbers, booleans, JSON `{...}`/`[...]`).
- A repeated key builds an array: `-p image_urls=a.png -p image_urls=b.png`.
- A local file path in a media field (`image_url`, `image_urls`, `audio_url`, ...) is uploaded automatically; URLs pass through.
- `-p key=@file.txt` reads the text of a file into the value (for long prompts). `@@x` is a literal `@x`.
- `--input file.json` (or `--input -` for stdin) supplies a JSON object; `-p` overrides it.
- A rejected input fails with `validation` and `error.details.problems` (field, message). Fix the fields. `--no-validate` sends anyway; only use it when the schema is clearly wrong.

## Presets

Presets are named model + default params. `kie-cli preset list` shows them (`name`, `description`, `model`, `source`); `kie-cli preset show <name>` shows the params.

`kie-cli run @musica -p prompt="lofi, rainy night"` runs the preset. Precedence: preset params, then `--input`, then `-p`. The description says which fields you must still pass (for example `prompt`, `image_urls`). The output has a `"preset"` key. Users can add or override presets in `~/.kie-cli/presets.yaml`.

## Output

`kie-cli run` waits and downloads. Success looks like:

- `taskId`, `model`, `state` (`success`), `preset` (only with `@name`).
- `files`: list of `{path, url, kind}`. `path` is the local file.
- `ids`: `task_id`, plus `audio_ids`, `last_frame_url`, `character_ids` when the result has them. Use them to chain (see below).
- `result` (raw API response), `creditsConsumed`, `costTime`, `uploaded`.

Special cases:

- `--no-wait` returns at once with `taskId`, `state: "submitted"`, `ids.task_id`. Collect it later with `kie-cli task wait <taskId>` (same `--out`, `--no-download`, `--timeout`) or peek with `kie-cli task get <taskId>`.
- Synchronous models answer immediately: `sync: true`, no `taskId`; `files`/`ids` are filled when the response has media.
- `--out DIR` sets the download directory (default `./kie-output`). `--no-download` skips downloading; the URLs stay in `result`.
- `--timeout 20m` caps the wait (default 10m, 30m for video models).
- `kie-cli task list [--limit N]` shows recent local tasks (newest first), useful to recover a taskId.

## Rules

- Never invent a model id or a field. If it is not in `models search` / `models schema`, it does not exist.
- Check `credits` and `models price` before video and before batches. Credits are spent on submit.
- Parallelize: submit several with `--no-wait`, then `kie-cli task wait <id>` for each.
- Chain with `ids`, not with guessed values: a follow-up task takes `task_id` / `audio_id` / `last_frame_url` from the previous output (details in the references).
- Always set `--out` to a directory the user expects; report the paths you produced.
- Durations and some counts are strings in several schemas (`duration: "5"`). Trust the schema.
- Chat/LLM models are not supported by `run`.

## Errors

On failure stdout is `{"ok":false,"error":{"code","message","details"?}}` and the exit code is set:

| exit | error.code | meaning and what to do |
|---|---|---|
| 0 | - | success |
| 2 | `usage` | bad command, flag or preset (or no HOME for `skills install`). Re-read `--help`, fix the call |
| 2 | `validation` | input does not match the schema, or unknown model. Read `details.problems`, fix fields, retry |
| 3 | `auth` | missing/invalid API key. Ask the user to run `kie-cli auth login` or set `KIE_API_KEY`. Check with `kie-cli auth status` |
| 4 | `credits` | not enough credits. Stop and tell the user; do not retry |
| 5 | `task_failed` | the model failed the task (`details.taskId`). Change the prompt/inputs; do not blindly resubmit |
| 6 | `timeout` | still running when the wait ended. `details.taskId` is kept: `kie-cli task wait <taskId>` with a larger `--timeout` |
| 7 | `rate_limit` | too many requests. Wait a bit, then retry |
| 8 | `submit_uncertain` | the submit may or may not have been created. Do NOT resubmit (double charge). Run `kie-cli task list` / `kie-cli task get <taskId>` to check, and only resubmit if nothing was created |
| 1 | `api` | other kie.ai/network error. Read `message`, retry once if transient |
| 1 | `io` | local file problem (download, write, read). Check paths and disk; the result URLs may be in `details` |

Errors after a task was created carry `details.taskId`: always keep it, the task is paid and recoverable with `task wait`/`task get`.

## Other commands

- `kie-cli models health <model>`: recent success-rate points.
- `kie-cli auth status`: where the key comes from (redacted).
- `kie-cli skills install [--target DIR]`: install this skill (default `~/.claude/skills`).
- `--quiet` silences stderr; `--debug` adds diagnostics.

## References (read the one you need)

- [references/image.md](references/image.md): text-to-image, edit, upscale, background removal.
- [references/video.md](references/video.md): text/image/first-last-frame video, extend, chaining.
- [references/audio.md](references/audio.md): music (Suno), speech, sync models, recovery.
