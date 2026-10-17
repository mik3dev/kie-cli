# Audio with kie-cli

## Music (Suno via `ai-music-api/*`)

Generate a song:

`kie-cli run ai-music-api/generate -p custom_mode=false -p instrumental=false -p model=V6 -p prompt="warm lofi about a rainy night" --out ./music`

- Required: `custom_mode`, `instrumental`, `model`. Read the allowed `model` values in `kie-cli models schema ai-music-api/generate` and prefer the newest non-discontinued one it lists.
- `custom_mode=false`: `prompt` is a free idea and lyrics are written for you.
- `custom_mode=true`: `title` is required, plus at least one of `style`, `lyrics`, `negative_tags`. In this mode `prompt` is taken as the exact lyrics unless `lyrics` is given.
- `instrumental=true` means no vocals (do not pass lyrics or `vocal_gender`).
- Presets: `kie-cli run @musica -p prompt="..."` and `@instrumental`. Both are non-custom mode.
- A task usually produces 2 tracks: `files` has 2 audio files and `ids.audio_ids` has 2 ids. Report both.

### Extend, cover and other follow-ups

Follow-ups take the previous output: `ids.task_id` and one entry of `ids.audio_ids` (the track you pick).

`kie-cli run ai-music-api/extend -p audio_id=<ids.audio_ids[i]> -p model=V6 -p prompt="add a bridge" -p continue_at=60`

- The `model` of an extend must match the model of the source audio.
- Other ops use `task_id` + `audio_id`: `convert-to-wav-format`, `create-music-video`, `generate-persona`, `timeStamped-lyrics`. `cover-generate` (cover art) needs only `task_id`.
- Cover/restyle an uploaded song with `upload-and-cover-audio` using `upload_url` (a local path is uploaded for you) or `task_id` + `audio_id`.
- Always confirm names with `kie-cli models schema ai-music-api/<op>`; list the family with `kie-cli models search ai-music-api`.

## Speech (ElevenLabs)

`kie-cli run elevenlabs/text-to-speech-multilingual-v2 -p text="Hello from kie" -p voice=EkK5I93UQWFDigLMpZcX --out ./voice`

- `voice` is an enum of ElevenLabs voice IDs (not names); the allowed values are in `models schema`. The preset `@voz` fixes a default voice; pass `text`.
- Optional tuning: `stability`, `similarity_boost`, `style`, `speed` (numbers).
- Cost is per characters; check `kie-cli models price <model>`.
- Dialogue with several voices: `elevenlabs/text-to-dialogue-v3` takes a `dialogue` array of `{text, voice}`; pass it as JSON: `-p 'dialogue=[{"text":"Hi","voice":"..."}]'` or via `--input file.json`.

## Synchronous models

Some endpoints answer immediately instead of creating a task (for example Gemini Omni audio/character helpers, lyric timing). The output has `"sync": true`, `result` with the raw response, and `ids` filled when the response has ids (`audio_ids`, `character_ids`). `--no-wait` is ignored for them. There is no `taskId` to wait for.

## Recovery

- Link expired or download failed (`io`): the task is still known. `kie-cli task get <taskId>` shows the result; `kie-cli task wait <taskId> --out DIR` downloads again.
- Timeout or a lost session: `kie-cli task list` to find the `taskId`, then `kie-cli task wait <taskId>`.
- `submit_uncertain`: do not resubmit. Tell the user the task may exist and ask them to check https://kie.ai/logs; resubmit only with their confirmation. `kie-cli task list` shows the attempt as `state: unknown` without a taskId.
- Expired Suno audio links can be refreshed with the recovery model (`ai-music-api/suno-recovery-audio`, input `task_id`) when the schema lists it.
