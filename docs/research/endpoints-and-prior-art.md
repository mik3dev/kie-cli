# kie.ai: legacy families + prior art (researched 2026-10-17)

Sources: docs.kie.ai/*.md (fetched; site is flaky, sometimes returns a ~5KB HTML shell: retry with `-A Mozilla/5.0` + sleep), official agent skill `https://kie.ai/.well-known/agent-skills/kie-models.tar.gz` (best source, SKILL.md + references/en.md), live unauthenticated probes of api.kie.ai.

## TL;DR

1. **Veo and Runway are no longer "non-unified" in current docs.** They are Market models on `POST /api/v1/jobs/createTask`, polled with `GET /api/v1/jobs/recordInfo`. Old per-family endpoints are documented under `docs.kie.ai/old-model/...` and still respond (probed: 401, not 404). Same for 4o Image, Flux Kontext, Suno (old-model section exists).
2. Truly non-task APIs: chat/sync (`/claude/v1/messages`, `/codex/v1/responses`, `/grok/v1/responses`, `/openai/v1/...`, Gemini paths), common API, file upload.
3. `GET /api/v1/models`, `/models/{model}/schema`, `/models/{model}/price`, `/models/{model}/success-rate` exist (auth-gated; bogus path 404s, these 401). Not in docs.kie.ai pages; specified only in the official `kie-models` skill. Schema `paths` key tells the route.
4. Undocumented, unauthenticated `GET /api/v1/playground/model-paths` returns 269 model ids incl. `veo-3-1`, `veo/extend`, `veo/get-1080p-video`, `veo/get-4k-video`, `runway`, `runway/extend-ai-video`, `runway/gen4-aleph`, `flux1-kontext`, `4o-image-api`. Confirms they are market models.
5. Doc inconsistency: Veo "new" callback pages still show legacy payload shape; Runway callback page shows Market shape. Rely on recordInfo, not callback docs.

## Part A: families

Base `https://api.kie.ai`, `Authorization: Bearer <key>`. **All errors come as HTTP 200 with `{code,msg,data}`; always check `code`** (401 auth, 402 credits, 404, 422 validation, 429 rate, 433 sub-key limit, 455 maintenance, 500/501/505).

### A1. Current (Market) form

All `POST /api/v1/jobs/createTask`, body `{model, callBackUrl?, input:{...}}`.

| Op | model | input |
|---|---|---|
| Veo generate | `veo-3-1` | prompt*, model: veo3_fast(default)\|veo3_lite\|veo3, image_urls[], generation_type: TEXT_2_VIDEO\|FIRST_AND_LAST_FRAMES_2_VIDEO\|REFERENCE_2_VIDEO, aspect_ratio: 16:9(default)\|9:16\|Auto, resolution: 720p(default)\|1080p\|4k, duration: 4\|6\|8 (default 8; REFERENCE only 8), enable_translation, watermark, enable_fallback(deprecated). REFERENCE: 1-3 imgs, fast/lite only. |
| Veo extend | `veo/extend` | task_id*, prompt*, seeds? (10000-99999), model: fast(default)\|quality\|lite, watermark?. Chains from prior Veo taskId. |
| Veo 1080p | `veo/get-1080p-video` | **camelCase** taskId*, index? . Async: new taskId; result in recordInfo `resultJson.resultUrls[0]`; if exists already, returned in `data.resultUrl`. ~1-3 min. Source task must be successful. |
| Veo 4K | `veo/get-4k-video` | **snake_case** task_id*, index? (int, 0). ~5-10 min; repeat call for same task returns code 422 ("4k is processing" or already generated + urls in data.resultUrls). Poll >=30s. Extra credits. |
| Runway gen | `runway` | prompt*, duration*: 5\|10, quality*: 720p\|1080p, image_url?, aspect_ratio: 16:9\|4:3\|1:1\|3:4\|9:16 (required for t2v, ignored with image_url), watermark?. 10s incompatible with 1080p. Docs text says callBackUrl "required"; schema/market treat optional. |
| Runway extend | `runway/extend-ai-video` | task_id*, prompt*, quality*, watermark?. Output = original + ~5s. Chains from Runway taskId. |
| Runway Aleph | `runway/gen4-aleph` | prompt*, video_url*, aspect_ratio?, seed?, reference_image?, watermark?, upload_cn?. **Docs: DISCONTINUED (deprecated: true)** but id still listed. Hide/warn. |

createTask response: `{code:200,msg:"success",data:{taskId, recordId}}`; poll by taskId (not recordId).

**Poll: `GET /api/v1/jobs/recordInfo?taskId=...`** same for ALL (incl. 1080p/4k/extend tasks).
- `data.state`: `waiting|queuing|generating` (non-terminal), `success`, `fail` (terminal). Official skill: `successFlag` 0 non-terminal, 1 success, **3 fail**. Poll on `state`; `progress` field unpopulated.
- Fields: taskId, model, state, successFlag, param (JSON string whose `input` is itself a JSON string: parse twice), resultJson (string), `response` (parsed resultJson, success only), failCode (string), failMsg, costTime (s), creditsConsumed, createTime/completeTime/updateTime (ms).
- Result shapes: `{resultUrls:[...]}` (images, video, most); `{resultObject:{...}}` (text, OmniHuman); Suno audio at `response.data[].audio_url`; lyrics `resultObject.lyricsData[].text`; Seedance 2 `lastFrameUrl[]`.
- Limits: recordInfo 10/s per taskId; createTask 20 per 10s per account (100+ concurrent tasks); discovery endpoints 1/s shared across models/schema/price/success-rate. Docs advise 2-3s start + backoff; video can exceed 10 min.
- Retention: result files 14 days; task logs 2 months.

Callbacks (Market format): POST to callBackUrl `{code,msg,data:{taskId,model,state,param,resultJson,failCode,failMsg,costTime,creditsConsumed,createTime,completeTime,updateTime}}`; code 200 success / 501 fail; 15s timeout; reply 200; may duplicate. Veo gen callback page still documents `data.info.{resultUrls,originUrls,resolution,seeds,media_ids,has_audio_list}` + `promptJson`; Veo 4K callback `data.info.resultUrls`.

### A2. Legacy form (docs `old-model/`, endpoints still live)

| Op | Method path | Body (camelCase) |
|---|---|---|
| Veo generate | POST /api/v1/veo/generate | prompt*, model (veo3\|veo3_fast\|veo3_lite), imageUrls[], generationType, aspect_ratio, callBackUrl, enableFallback, enableTranslation, watermark, resolution, duration |
| Veo extend | POST /api/v1/veo/extend | taskId*, prompt*, seeds, model(fast\|quality\|lite), watermark, callBackUrl |
| Veo 1080p | **GET** /api/v1/veo/get-1080p-video?taskId=&index= | `data.resultUrl`; non-200 until ready, retry 20-30s |
| Veo 4K | **POST** /api/v1/veo/get-4k-video | taskId*, index, callBackUrl |
| Veo poll | GET /api/v1/veo/record-info?taskId= | `data.successFlag` 0 generating, 1 success, 2 failed, 3 generation failed; `data.response.{resultUrls,originUrls,fullResultUrls,resolution}`, errorCode (400/500/501), errorMessage, fallbackFlag. "Authoritative" for gen/extend/1080p/4k. |
| Runway gen | POST /api/v1/runway/generate | prompt*, duration*, quality*, imageUrl, aspectRatio, waterMark, callBackUrl (required) |
| Runway extend | POST /api/v1/runway/extend | taskId*, prompt*, quality*, waterMark, callBackUrl |
| Runway poll | GET /api/v1/runway/record-detail?taskId= | `data.state` wait\|queueing\|generating\|success\|fail; videoInfo.{videoId,taskId,videoUrl,imageUrl}; parentTaskId (extend); failCode/failMsg; expireFlag (1 after 14d) |
| Aleph gen/poll | POST /api/v1/aleph/generate; GET /api/v1/aleph/record-info | prompt*, videoUrl*, aspectRatio, seed, referenceImage, waterMark, uploadCn, callBackUrl |
| Flux Kontext | POST /api/v1/flux/kontext/generate; GET .../record-info | not read in depth |

Recommendation: **target Market only**; legacy optional. Chaining ops (veo/extend, veo/get-*, runway/extend-ai-video) are ordinary models whose input takes a prior taskId under inconsistent names (`task_id` vs `taskId`): a schema-driven CLI must not hardcode one. Consider a `--from-task <id>` convenience that maps to whichever of those fields the schema declares.

### A3. Common API
- `GET /api/v1/chat/credit` -> `{code,msg,data:<number>}` (balance, may be fractional). Name is misleading but is the credits endpoint.
- `POST /api/v1/common/download-url` `{url}` -> `data` is a string URL valid **20 min**. Only kie-generated result URLs (resultUrls, audio_url); docs say upload-API URLs NOT supported, official skill says they work (conflict: test). External URLs -> 422. Does not extend 14-day retention. Result URLs are usually plain-GET downloadable (douhashi does unauthenticated GET, falls back to download-url once).
- Webhook verification (optional; generate `webhookHmacKey` at kie.ai/settings). Headers `X-Webhook-Timestamp` (unix seconds), `X-Webhook-Signature`. `sig = base64(HMAC-SHA256(key=webhookHmacKey, msg=taskId + "." + timestampSeconds))`; taskId from `data.taskId` (Suno: `data.task_id`). Constant-time compare. **Signature does not cover the body**; docs give no replay window (enforce own, e.g. +-5 min); confirm via recordInfo.
- No list-tasks endpoint exists (douhashi README); keep a local ledger.

### A4. File upload API (host `https://kieai.redpandaai.co`, same Bearer key)
- `POST /api/file-url-upload` JSON `{fileUrl*, uploadPath*, fileName?}` (http/https; 30s download timeout; rec <=100MB)
- `POST /api/file-stream-upload` multipart `file`, `uploadPath*`, `fileName?` (rec <=100MB; use for >10MB)
- `POST /api/file-base64-upload` JSON `{base64Data*, uploadPath*, fileName?}` (plain or data URL; +33% size; rec <=10MB, best <=1MB)
- `uploadPath` without leading/trailing slash. Same name overwrites (cache delay) -> unique dir per upload.
- Response `{success,code,msg,data:{fileName,filePath,downloadUrl,fileSize,mimeType,uploadedAt}}`; use `downloadUrl` (tempfile.redpandaai.co/...). **Deleted after 24h**, no delete endpoint. Codes 200/400/401/405/500.

### A5. Catalog coverage (cannot call; inferred from docs/skill)
- `GET /api/v1/models[?taskType=&provider=&q=]`, no pagination, ~202 models. `data:{total,models:[{model,slug,title,provider,taskType[],description|null,pricingDesc}]}`. taskType e.g. "Text to Video"; percent-encode spaces; multi-value comma-separated. Fetch once, filter locally (1 req/s shared budget).
- `GET /api/v1/models/{model}/schema` (slash not encoded: `/models/wan/v2-2-t2v/schema`) -> `data:{model, openapi}`; `openapi` is inline OpenAPI 3.1 or **null** (docs not synced). `$ref`s are not inlined; keys may contain %20 and trailing spaces. `input` may be `oneOf`. Contains `paths` (route + method), `callbacks`, `servers`, error codes; not result retrieval. Non-createTask `paths` key means a sync chat/Gemini model.
- `/models/{m}/price` -> `{model,pricingDesc}`; `/models/{m}/success-rate` -> 24h, 10-min points, nulls when no traffic.
- Veo/Runway in catalog: unproven without a key, but strongly expected (model-paths list has them; docs pages are generated from the same OpenAPI; douhashi builds its catalog from those pages). Handle `openapi:null` gracefully.
- ~78 models need an input file URL; field names vary (image_url, input_image, image, video_url, first_frame_image, ...), always read schema.

## Part B: prior art

No official CLI/SDK. Official: `npx skills add https://kie.ai` -> `kie-models` + `kie-chat-agents` skills (curl+jq recipes, `KIE_API_KEY` env).

| Repo | Lang | Stars | Last push | Approach | Notes |
|---|---|---|---|---|---|
| github.com/douhashi/kie-ai-cli | **Go** | 0 | 2026-10-16 | Catalog **generated from docs.kie.ai OpenAPI** (llms.txt -> pages -> yaml), embedded, `catalog update` fetches newer; does not use /api/v1/models | MIT. Closest to our goal. SQLite ledger, submit-and-walk-away, `task run <model> --<field>` flags from schema, `--input file\|-`, noun-verb cmds, `--json`, exit 0/1/2, `file upload`, `credits show`, shell completion with model ids, `skill install` (SKILL.md telling agent to use `model show`). Refuses unknown fields pre-submit. Chat models excluded. Good tests, 120 files. Study first. |
| github.com/hanskaii/kie-ai-cli | Go | 0 | 2026-08-21 | catalog.json generated by `tools/gen_catalog.py` from docs, embedded; hand pricing.json | MIT, stdlib only, install.sh/ps1, 130+ models, batches. Python regen step. |
| github.com/artschekoff/kie-ai-cli (+kie-ai-mcp) | Go | 0 | 2026-08-30 | Hardcoded image models, per-model param validation | JSON on every command, `describe` emits CLI schema JSON, slash-command docs, encrypted creds. Image only. |
| github.com/kelvincushman/kie-cli | Go | 2 | 2026-09-01 | CLI Printing Press generated from OpenAPI (`kie-pp-cli`), 129 embedded models, MCP | Apache-2.0. `--agent`, `--no-input`, `--dry-run`, `doctor --json`, `which --json`, `--confirm-paid`. Over-scoped (director/academy, affiliate link). Same generator output in mvanhorn/printing-press-library. |
| github.com/nick-vels/VelsVisual | JS (npm, 0 deps) | 76 | 2026-08-06 | Registry + per-model schema fetched from docs, cached 24h | MIT. `schema`, `run --dry-run --wait --download --json --refresh-schema`; non-zero exit on API error; setup wizard + skill. Best pragmatic UX, most stars among CLIs. |
| github.com/mrdainami/kie-mcp | JS/TS | 38 | 2026-08-20 | "Dumb" shim: 5 tools (kie_post, kie_get, kie_upload_file, kie_download, kie_fetch_model_docs), host allow-list | MIT. Minimal, injection-aware; agent reads docs. |
| github.com/elibarnett/kie-mcp | JS | 7 | 2026-10-14 | Hardcoded 165+ models, pricing, research profiles; CI drift-watch | Rich but stale-prone. |
| github.com/andrewlwn77/kie-ai-mcp-server | JS | 24 | 2025-09-02 | One tool per model | Stale; the anti-pattern. |
| github.com/gateway/ComfyUI-Kie-API | Python | 41 | 2026-10-06 | Hand-written ComfyUI nodes | Also gateway/media-studio. |
| github.com/hassanvfx/kie-api-python | Python | 0 | 2026-09-12 | CLI+MCP, dry-run, upload-first, durable job records | Hardcoded workflows. |
| github.com/matt-riley/mediacreator | Go | 0 | 2026-10-03 | `gen-kie-catalog` pulls `/api/v1/playground/model-paths` into a Go fallback list | Only user of that endpoint. |
| github.com/justintanner/apicity | TS | 2 | 2026-10-17 | Multi-provider SDK/MCP with local schemas, retries, cost estimates, approval gates | kie provider. |

Zero of 13 cloned repos call `GET /api/v1/models`; the dynamic catalog + schema flow is unexploited. No polished Go SDK exists; douhashi is the reference.

### Patterns worth copying
- Noun-verb commands; `--json` for success output; errors as plain text on stderr; exit 0 ok / 1 runtime / 2 usage. Add distinct codes for auth(401), insufficient credits(402), task failed, timeout, rate-limited.
- Non-interactive by default; `--agent`/`--no-input` style flag; `--dry-run` prints final createTask body; paid-action confirmation only when a TTY.
- Introspection for agents: `describe`/`model show --json`/`doctor --json`; ship an installable SKILL.md that tells agents to query the CLI, not memorize models.
- Submit returns taskId immediately; `--wait`/`task wait`; `task get`, `task download`; local ledger since no list API.
- Schema-driven flags from model `input` (enums, repeated array flags, bool `--x`/`--x=false`, `--input file|-` JSON merged under flags); reject unknown fields before submitting.
- Pre-flight credit check vs `pricingDesc`; show `creditsConsumed`.
- Catalog cache on disk with TTL + embedded fallback + explicit `catalog update` + staleness notice.
- Key from `KIE_API_KEY` env, then 0600 config; mask on display; never send key to result hosts or the upload host redirect targets.
- Downloads: unauthenticated GET first, fallback to common/download-url; atomic write; don't overwrite; unique upload dir.
- Gotchas: errors are HTTP 200; `param.input` double-encoded; `openapi` null; `$ref` keys with %20/trailing space; model ids contain `/` unescaped in URL path; encode spaces in query; discovery 1 req/s shared; poll 2-3s with backoff, 10/s per task; video >10 min; 14-day result and 24h upload retention; `taskId` vs `task_id` field name drift.
