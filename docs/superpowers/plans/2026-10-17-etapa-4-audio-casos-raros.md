# kie-cli Etapa 4 (Audio y casos raros) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `run` cubre los tres casos que hoy rechaza o atiende a medias:
- los modelos síncronos, cuyo path no es `createTask` (Gemini Omni audio y character);
- el recovery de Suno, con su propio polling;
- los resultados con media dentro de `resultObject` (máscaras de segment-map, por ejemplo).

Los modelos de chat se rechazan con un error claro.

**Architecture:**
- `internal/result` gana un recolector genérico de URLs de media (`MediaURLs`), que sirve tanto para `resultObject` como para las respuestas síncronas.
- `internal/task` gana `Call`, un POST síncrono con la misma política de errores que `Submit` pero sin ledger, y `WaitRecovery`, el polling del endpoint de recovery.
- La CLI elige el camino según tres datos: el path del schema, el `taskType` del catálogo y el id de modelo.

**Tech Stack:** Go 1.27, sin dependencias nuevas.

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`:
- §2: chat fuera de alcance.
- §3: filas "Hay modelos síncronos" y "La forma del resultado varía".
- §4: "Modelos síncronos": POST al path del schema, el body va en `result` y las URLs de media se descargan.
- Fila "4" de §8.

Los datos reales vienen de la captura: `testdata/live/schemas/gemini-omni-audio.json`, `gemini-omni-character.json` y `ai-music-api__suno-recovery-audio.json`.

## Global Constraints

- Siguen vigentes las de las etapas 1 a 3:
  - stdout lleva **exactamente un** objeto JSON y los avisos van por stderr.
  - Códigos de salida: 2 uso/validación · 3 auth · 4 créditos · 5 tarea falló · 6 timeout · 7 rate limit · 8 `submit_uncertain`.
  - Sin mutación.
  - Cobertura ≥80% por paquete y `golangci-lint run ./...` limpio.
  - Ningún test usa la API real ni lee `.env`.
- **Chat:** un modelo es de chat si algún `taskType` del catálogo contiene `"Chat"`. `run` lo rechaza con `usage` (salida 2) y el mensaje `chat models are out of scope for kie-cli; the agent is the LLM`, antes de cualquier POST.
- **Síncronos** (path del schema distinto de `/api/v1/jobs/createTask` y no chat):
  - Se hace POST al path del schema. El body es el input coercionado y validado tal cual, sin envoltorio `{model,input}` (Ruling 6 de la etapa 1: `InputSchema` ya devuelve el body completo).
  - Salida: `{"ok":true,"model":m,"state":"success","sync":true,"result":<data>,"files":[...],"ids":{...},"uploaded":[...]}`, sin `taskId`.
  - No se escribe ledger, porque no hay tarea.
  - `--no-wait` se ignora con un aviso por stderr.
  - `--dry-run` muestra `"path":"<path>"` y el body con `body` = input.
- **`ids` de síncronos:** `data.audioId` va a `audio_ids:[...]` y `data.characterId` a `character_ids:[...]`. Son los nombres de input de `gemini-omni-video` (`audio_ids`, `character_ids`). `task_id` se **omite** en salidas síncronas; en las de tareas sigue estando siempre.
- **Errores síncronos:** se aplica la misma política que `createTask` (spec §4).
  - Se reintenta solo con 429 o si el request no llegó a enviarse.
  - Si la falla es ambigua después de enviar (timeout, 5xx o 455), sale `submit_uncertain` (salida 8) **sin** ledger.
  - Los `code` 401, 402 y 422 se mapean igual que en la etapa 1.
- **Recovery de Suno** (`ai-music-api/suno-recovery-audio`): se envía por `createTask` como dice el schema real y se hace polling en `GET /api/v1/suno/recovery/record-info?task_id=<taskId>`, no en `recordInfo`.
  - `code` 201 significa que sigue corriendo (`data` es null).
  - `code` 200 significa que terminó; `data` trae una entrada por track y se trata como `{"data": data}` para reusar `result.Extract`/`result.URLs`.
  - `code` 500 significa que falló todo: `task_failed` (salida 5) con `details.taskId`.
  - El backoff y el timeout son los mismos que en `Waiter`.
  - **Provisional:** la documentación de kie se contradice sobre este endpoint, así que esto se verifica en la etapa 7.
- **`MediaURLs`** junta los strings `http://` o `https://` que estén:
  - bajo una clave que, en minúsculas, termina en `url`;
  - o dentro de un array bajo una clave que termina en `urls`.

  Se excluyen las claves que contienen `callback` o `stream`. El recorrido sigue las claves en orden alfabético y quita duplicados. `asset://` y otros esquemas se ignoran.
- **Orden de descarga** de tareas: `resultUrls`, `data[].audio_url`, `lastFrameUrl` y al final `MediaURLs(resultObject)`, sin duplicados.

## Review Focus

1. **`run` con un modelo de chat** (p. ej. `claude-sonnet-4-6`): sale con código 2 y el mensaje fijo, sin ningún POST. → test en Task 2.
2. **Modelo síncrono que responde `code` 402**: sale con código 4, sin ledger y sin `taskId`. → test en Task 2.
3. **Recovery que responde 201 varias veces y después 200** con 2 tracks: descarga 2 archivos y `ids.audio_ids` sale en orden. **Recovery que responde 500**: sale con código 5 y `details.taskId`. → tests en Task 3.
4. **`resultObject` solo de texto** (letras `lyricsData[].text`): `files` queda `[]`, el texto está en `result` y la salida es 0. **`resultObject` con `segments[].maskUrl`**: se descargan las máscaras. → test unitario en Task 1 y e2e en Task 2.
5. **URLs que no son media en respuestas síncronas** (`asset://…`, `callbackUrl`, `stream_audio_url`): no se descargan. → test en Task 1.

---

### Task 1: URLs de media genéricas e ids de síncronos

**Files:**
- Modify: `internal/result/result.go`, `internal/result/result_test.go`

**Interfaces:**
- Produces:
  - `func result.MediaURLs(v json.RawMessage) []string`, con las reglas de Global Constraints.
  - `result.URLs(response)` suma `MediaURLs(response.resultObject)` al final.
  - `result.IDs` gana `CharacterIDs []string \`json:"character_ids,omitempty"\`` y `TaskID` pasa a `json:"task_id,omitempty"`. Nota: las tareas siempre lo llenan.
  - `func result.SyncIDs(data json.RawMessage) IDs` mapea `audioId` a `AudioIDs` y `characterId` a `CharacterIDs`. Lo que no esté queda omitido.

- [ ] **Step 1: Write the failing tests**

```go
func TestMediaURLs(t *testing.T)
// {"characterId":"c1","imageUrl":"https://a/i.png","bodyImageUrl":"https://a/b.png","callbackUrl":"https://cb","x":{"stream_audio_url":"https://s"},
//  "assetUrl":"asset://x","list":{"maskUrls":["https://a/m1.png","https://a/m1.png"]}}
// -> ["https://a/b.png","https://a/i.png","https://a/m1.png"]   (alphabetical key walk, dedup; callback/stream/asset excluded)
func TestURLsIncludesResultObjectMedia(t *testing.T) // Review Focus 4
// {"resultObject":{"segments_count":2,"segments":[{"maskUrl":"https://a/0.png","name":"sky","index":0},{"maskUrl":"https://a/1.png","index":1}]}}
// -> ["https://a/0.png","https://a/1.png"]
func TestURLsTextOnlyResultObject(t *testing.T) // Review Focus 4
// {"resultObject":{"lyricsData":[{"text":"la la","title":"t"}]}} -> [] (empty, non-nil)
func TestSyncIDs(t *testing.T)
// {"audioId":"au1","name":"v"} -> IDs{AudioIDs:["au1"]}; json.Marshal == `{"audio_ids":["au1"]}`
// {"characterId":"ch1"} -> IDs{CharacterIDs:["ch1"]}; `null`/`{}` -> IDs{} and json.Marshal == `{}`
```

- [ ] **Step 2: Run** `go test ./internal/result/`. Expected: FAIL (undefined).

- [ ] **Step 3: Implement.** El recorrido de `MediaURLs` decodifica a `any` y baja de forma recursiva por mapas (con las claves ordenadas) y por arrays. La clave "padre" de cada elemento de un array es la del array.

- [ ] **Step 4: Run** `go test -race -cover ./internal/result/` y `make test`. Expected: PASS, todo lo existente sigue verde y la cobertura es ≥80%.

- [ ] **Step 5: Commit** `feat: collect media urls from resultObject and sync responses`

---

### Task 2: Modelos síncronos y guardia de chat en `run`

**Files:**
- Create: `internal/task/call.go`, `internal/task/call_test.go`, `internal/cli/sync_test.go`
- Modify: `internal/cli/run.go`

**Interfaces:**
- Consumes: `result.MediaURLs`, `result.SyncIDs` (Task 1), `catalog.Model.TaskType`, `schema.Doc.Endpoint()`, `api.Client.Do`.
- Produces:
  - `func task.Call(ctx context.Context, c *api.Client, method, path string, body map[string]any) (json.RawMessage, error)`. Devuelve el `data` del envelope.
  - Errores de `task.Call`: los mismos tipos y códigos que `task.Submit` para el caso ambiguo (`submit_uncertain`, salida 8), sin escribir ledger. Leer `Submit` y extraer lo compartido en vez de duplicarlo.
  - El chat se detecta en `run` con el `TaskType` del catálogo (ver Global Constraints). El fallback offline del catálogo sigue usando la guardia de `IsJob` y no rompe.

- [ ] **Step 1: Write the failing tests**

```go
// call_test.go (httptest)
func TestCallReturnsData(t *testing.T)            // {"code":200,"data":{"audioId":"au1"}} -> data == `{"audioId":"au1"}`
func TestCallMapsCredits(t *testing.T)            // {"code":402,...} -> *api.Error / output code "credits"
func TestCallAmbiguousIsSubmitUncertain(t *testing.T) // server closes conn after reading body -> output code "submit_uncertain"

// sync_test.go (newFake; add a fake route for /api/v1/omni/audio/create and /api/v1/omni/character/create)
func TestRunChatModelIsUsageError(t *testing.T)   // Review Focus 1: run claude-sonnet-4-6 -p x=y -> code 2, message contains "chat models are out of scope"; createHits == 0 and no omni hits
func TestRunSyncOmniAudio(t *testing.T)
// run gemini-omni-audio -p audio_id=<value valid per schema> -p name=voz (adjust to schema) ->
// code 0, sync true, no "taskId" key, ids == {"audio_ids":["au1"]}, files == [], request body has no "model"/"input" wrapper
func TestRunSyncCharacterDownloadsImages(t *testing.T)
// fake returns {"characterId":"ch1","imageUrl":"<cdn>/c.png"} -> files[0].url == "<cdn>/c.png", ids.character_ids == ["ch1"]
func TestRunSyncCreditsError(t *testing.T)        // Review Focus 2: omni route replies code 402 -> exit 4, ledger file absent/empty
func TestRunSyncDryRun(t *testing.T)              // --dry-run -> "path":"/api/v1/omni/audio/create", no omni hit
func TestRunSyncNoWaitWarns(t *testing.T)         // --no-wait -> exit 0, stderr contains "warning", omni hit == 1
func TestRunSegmentMapDownloadsMasks(t *testing.T) // Review Focus 4: grok-imagine-image-2-0/segment-map job whose resultJson is
// {"resultObject":{"segments":[{"maskUrl":"<cdn>/0.png","index":0}]}} -> files[0].url == "<cdn>/0.png"
```

- [ ] **Step 2: Run** `go test ./internal/task/ ./internal/cli/ -run 'Call|Sync|Chat|SegmentMap'`. Expected: FAIL.

- [ ] **Step 3: Implement.** En `runModel`, después de `findModel` y `schema.Fetch`:
  1. Si es chat, se corta con el error de usage.
  2. Si no es `IsJob()`, se toma el camino síncrono. Se reusa `prepareInput` (coerción, placeholders y validación), y luego vienen el dry-run, la subida real, `task.Call`, la descarga de `MediaURLs(data)` y la salida.
  3. Se quita el error viejo `synchronous models arrive in stage 4`.

  Si `run.go` pasa de unas 400 líneas, el camino síncrono va a `internal/cli/run_sync.go`.

- [ ] **Step 4: Run** `make test` y `golangci-lint run ./...`. Expected: verde, ≥80% y 0 issues.

- [ ] **Step 5: Commit** `feat: run synchronous models and reject chat models`

---

### Task 3: Polling del recovery de Suno

**Files:**
- Create: `internal/task/recovery.go`, `internal/task/recovery_test.go`, `internal/cli/recovery_test.go`
- Modify: `internal/cli/run.go` (`finishTask` elige el waiter), `internal/cli/task.go` (`task wait` y `task get` usan el endpoint de recovery cuando el modelo del ledger es el de recovery)

**Interfaces:**
- Consumes: `task.Waiter` (backoff 3s→15s, Sleep/Now inyectables), `result.Extract` y `result.URLs`.
- Produces:
  - `const task.RecoveryModel = "ai-music-api/suno-recovery-audio"`
  - `func (w Waiter) WaitRecovery(ctx context.Context, c *api.Client, taskID string, timeout time.Duration) (Record, error)`. El `Record` devuelto tiene `State` `"success"` y `Response = {"data": <data>}`, con el mismo comportamiento de timeout y cancelación que `Wait`.
  - Un `code` 500 devuelve un error `task_failed` (salida 5).

- [ ] **Step 1: Write the failing tests**

```go
// recovery_test.go (internal/task, httptest)
func TestWaitRecovery201Then200(t *testing.T)
// GET /api/v1/suno/recovery/record-info?task_id=r1 replies {"code":201,"data":null} x2 then
// {"code":200,"data":[{"id":"a1","audio_url":"https://c/1.mp3"},{"id":"a2","audio_url":"https://c/2.mp3"}]}
// -> State "success", result.Extract("r1", rec.Response).AudioIDs == ["a1","a2"], 3 requests, query param task_id=r1
func TestWaitRecovery500IsTaskFailed(t *testing.T)   // -> output code "task_failed"
func TestWaitRecoveryTimeout(t *testing.T)           // always 201 -> output code "timeout" (fake clock)

// recovery_test.go (internal/cli, newFake + recovery route)
func TestRunRecoveryDownloadsTracks(t *testing.T)    // Review Focus 3: run ai-music-api/suno-recovery-audio -p task_id=old1 ->
// createTask hit 1, recordInfo hits 0, files 2, ids == {"task_id":"task-1","audio_ids":["a1","a2"]}
func TestRunRecoveryFailed(t *testing.T)             // Review Focus 3: 500 -> exit 5, details.taskId == "task-1"
func TestTaskWaitRecoveryUsesLedgerModel(t *testing.T) // run --no-wait, then task wait task-1 -> uses recovery endpoint, exit 0
```

- [ ] **Step 2: Run** `go test ./internal/task/ ./internal/cli/ -run Recovery`. Expected: FAIL.

- [ ] **Step 3: Implement.** El `code` 201 viene en un envelope con HTTP 200: `api.Client.Do` lo devuelve como `*api.Error{KieCode:201}`. `WaitRecovery` lo trata como "sigue corriendo" y no como error. Los demás errores siguen la política de reintento de `recordInfo` (GET reintentable).
  - `finishTask` recibe el modelo y usa `WaitRecovery` cuando el modelo es `task.RecoveryModel`.
  - `task get` con ese modelo hace un solo GET al endpoint de recovery y emite lo mismo que hoy. El `state` es `generating` si vino 201, `success` si vino 200 y `fail` si vino 500.

- [ ] **Step 4: Run** `make test` y `golangci-lint run ./...`. Expected: verde, ≥80% y 0 issues.

- [ ] **Step 5: Commit** `feat: poll Suno recovery tasks on their own endpoint`
