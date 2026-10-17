# kie-cli Etapa 3 (Encadenar) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** La salida de `run`, `task get` y `task wait` incluye un objeto `ids` con lo necesario para encadenar la siguiente tarea (`task_id`, `audio_ids` de Suno, `last_frame_url`). Además, los tracks de Suno (`response.data[].audio_url`) se descargan.

**Architecture:** `internal/result` gana `IDs` y lee `data[]` de Suno, y la CLI emite `ids` en los tres comandos. Las claves de `ids` usan los mismos nombres que los campos de input de los modelos encadenados (`task_id`, `audio_id`), para que el agente las copie directo a `-p`.

**Tech Stack:** Go 1.27, sin dependencias nuevas.

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`: §3 (filas "La forma del resultado varía" y "Encadenamiento"), §4 "Contrato de salida" (`"ids":{"audio_ids":[…]}`) y la fila "3" de §8. Los patrones de encadenamiento están en `docs/research/audio.md` (líneas 73-81) y en `docs/research/endpoints-and-prior-art.md` (Veo/Runway extend).

## Global Constraints

- Siguen vigentes las de las etapas 1 y 2:
  - stdout lleva **exactamente un** objeto JSON por comando y los avisos van por stderr.
  - Códigos de salida sin cambios.
  - Las funciones que transforman datos devuelven valores nuevos, sin mutar.
  - Cobertura ≥80% por paquete (`go test -race -cover ./...`) y `golangci-lint run ./...` limpio.
  - Ningún test usa la API real ni lee `.env`.
- Forma de `ids`: `{"task_id": "<taskId>", "audio_ids": [...], "last_frame_url": "..."}`.
  - `task_id` siempre está.
  - `audio_ids` y `last_frame_url` se omiten si no hay valor, nunca se emiten como `null` ni como `[]`.
- `audio_ids` mantiene el orden de `response.data[]`. Solo cuentan las entradas con `id` no vacío.
- Orden de descarga: `resultUrls`, luego `data[].audio_url` y por último `lastFrameUrl`, sin duplicados (spec §3).
- Forma de los tracks de Suno (docs/research/audio.md:73): `response.data[]` de `{id, audio_url, stream_audio_url, image_url, prompt, model_name, title, tags, createTime, duration}`. Normalmente son 2 tracks por tarea. `stream_audio_url` e `image_url` **no** se descargan.
- El resto de los resultados raros de Suno (`resultObject`, recovery, modelos síncronos) son de la etapa 4.

## Review Focus

1. **Tarea Suno con 2 tracks:** se descargan 2 archivos con nombres distintos (`_0`, `_1`) y `ids.audio_ids` sale en el mismo orden que `files`. → test en Task 2.
2. **Resultado de imagen sin `data[]` ni `lastFrameUrl`:** `ids` es exactamente `{"task_id": "..."}`, sin claves `null` ni vacías. → test en Task 1 y en Task 2.
3. **`task get` de una tarea que aún no terminó** (`response` nulo): `ids` trae solo `task_id` y la salida es JSON válido, sin fallar. → test en Task 2.
4. **Entrada de `data[]` sin `audio_url` o sin `id`** (p. ej. tarea a medias): se salta sin fallar, y el resto de las entradas se procesa. → test en Task 1.
5. **Encadenar con los ids de la salida:** un `-p audio_id=<ids.audio_ids[0]>` en `ai-music-api/extend` y un `-p task_id=<ids.task_id>` en `veo/extend` pasan la validación de la etapa 2 con `--dry-run`. → test en Task 2.

---

### Task 1: Extracción de ids y audio de Suno

**Files:**
- Modify: `internal/result/result.go`, `internal/result/result_test.go`

**Interfaces:**
- Produces:
  - `type result.IDs struct { TaskID string `json:"task_id"`; AudioIDs []string `json:"audio_ids,omitempty"`; LastFrameURL string `json:"last_frame_url,omitempty"` }`
  - `func result.Extract(taskID string, response json.RawMessage) IDs`. Si `lastFrameUrl` es un array, se toma el primer string no vacío.
  - `result.URLs(response)` suma `data[].audio_url` según el orden de Global Constraints.

- [ ] **Step 1: Write the failing tests**

```go
func TestURLsIncludesSunoAudio(t *testing.T)
// `{"data":[{"id":"a1","audio_url":"https://c/1.mp3","stream_audio_url":"https://s/1","image_url":"https://c/1.jpg"},
//           {"id":"a2","audio_url":"https://c/2.mp3"}]}` -> ["https://c/1.mp3","https://c/2.mp3"]
func TestURLsOrderAndDedupAcrossShapes(t *testing.T)
// resultUrls ["https://a/x.mp4"], data[{audio_url "https://a/x.mp4"},{audio_url "https://a/y.mp3"}], lastFrameUrl "https://a/f.png"
// -> ["https://a/x.mp4","https://a/y.mp3","https://a/f.png"]
func TestExtractSuno(t *testing.T)
// same data as above + entry {"audio_url":"https://c/3.mp3"} without id and {"id":"a3"} without audio_url
// -> IDs{TaskID:"t1", AudioIDs:["a1","a2","a3"]}; URLs skips the entry without audio_url
func TestExtractLastFrame(t *testing.T)
// `{"lastFrameUrl":["","https://a/f.png"]}` -> LastFrameURL "https://a/f.png"; `{"lastFrameUrl":"https://a/g.png"}` -> "https://a/g.png"
func TestExtractEmptyResponses(t *testing.T) // Review Focus 2/3
// for `` , `null`, `{}`, `not json`, `{"data":"x"}`: Extract("t1", raw) == IDs{TaskID:"t1"}
// and json.Marshal(IDs{TaskID:"t1"}) == `{"task_id":"t1"}`
```

- [ ] **Step 2: Run** `go test ./internal/result/`. Expected: FAIL (`Extract` undefined; `URLs` ignores `data`).

- [ ] **Step 3: Implement.** Dentro de `URLs`, el bloque `data[]` decodifica en `[]struct{ID string `json:"id"`; AudioURL string `json:"audio_url"`}`. Si `data` no es un array, se ignora. Se quita el comentario `ponytail: Suno ... stage 4`.

- [ ] **Step 4: Run** `go test -race -cover ./internal/result/`. Expected: PASS, cobertura ≥80%.

- [ ] **Step 5: Commit** `feat: extract chaining ids and Suno audio urls from results`

---

### Task 2: `ids` en `run`, `task get` y `task wait`, y flujos encadenados

**Files:**
- Modify: `internal/cli/run.go` (`finishTask` y la rama `--no-wait`), `internal/cli/task.go` (`task get`)
- Create: `internal/cli/chain_test.go`
- Modify: `internal/cli/run_e2e_test.go` (solo si hace falta, para que `fakeKie` sirva una respuesta de Suno usando el `recordReply` que ya existe)

**Interfaces:**
- Consumes: `result.Extract(taskID, response) result.IDs` y `result.URLs` (Task 1).
- Produces: la clave `ids` en la salida de éxito de `run` (incluido `--no-wait`, donde es `{"task_id": id}`), `task wait` (vía `finishTask`) y `task get`. Las salidas de error no cambian (ya llevan `details.taskId`).

- [ ] **Step 1: Write the failing tests** (`chain_test.go`, con `newFake`):

```go
func TestRunImageIdsOnlyTaskID(t *testing.T) // Review Focus 2
// existing imagen4 success flow -> m["ids"] == map[string]any{"task_id":"task-1"}

func TestRunNoWaitHasIds(t *testing.T)
// --no-wait -> ids == {"task_id":"task-1"}

func TestRunSunoTwoTracks(t *testing.T) // Review Focus 1
// recordReply returns success for model "ai-music-api/generate" whose resultJson is
// {"data":[{"id":"a1","audio_url":"<cdn>/1.mp3"},{"id":"a2","audio_url":"<cdn>/2.mp3"}]}
// run ai-music-api/generate -p prompt=x -p custom_mode=false -p instrumental=true -p model=V5 --out <tmp>
// (ajustar params al schema real si la validación lo exige y anotarlo en el report)
// -> len(files)==2, file paths distinct (end in _0.* and _1.*), ids.audio_ids == ["a1","a2"]

func TestTaskGetIds(t *testing.T) // Review Focus 3
// task get task-1 with state "generating" and resultJson "" -> ids == {"task_id":"task-1"}, exit 0
// task get task-1 with the Suno success reply -> ids.audio_ids == ["a1","a2"]

func TestTaskWaitIds(t *testing.T)
// task wait task-1 with the Suno success reply -> ids.audio_ids == ["a1","a2"]

func TestChainSunoExtendFromIds(t *testing.T) // Review Focus 5
// 1) run ai-music-api/generate (Suno reply) -> take ids.audio_ids[0]
// 2) run ai-music-api/extend -p audio_id=<that> -p model=V5 --dry-run -> exit 0, body.input.audio_id == "a1"

func TestChainVeoExtendFromIds(t *testing.T) // Review Focus 5
// 1) run google/imagen4 ... -> ids.task_id; 2) run veo/extend -p task_id=<that> -p prompt=more --dry-run -> exit 0,
//    body.input.task_id == "task-1"
```

- [ ] **Step 2: Run** `go test ./internal/cli/ -run 'Ids|Suno|Chain'`. Expected: FAIL (sin clave `ids`).

- [ ] **Step 3: Implement.** Agregar `"ids": result.Extract(id, rec.Response)` a los mapas de `output.Success` en `finishTask` y en `task get`, y `"ids": result.IDs{TaskID: id}` en la rama `--no-wait`. El `CDN` del fake sirve cualquier path. Si el `Content-Type` fijo (`image/png`) del fake impide comprobar `.mp3`, se acepta `.mp3` por la extensión de la URL (la extensión del path gana sobre el Content-Type).

- [ ] **Step 4: Run** `make test` y `golangci-lint run ./...`. Expected: todo verde, ≥80% por paquete y 0 issues.

- [ ] **Step 5: Commit** `feat: expose chaining ids in run and task output`
