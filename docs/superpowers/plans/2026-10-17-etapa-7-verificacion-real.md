# kie-cli Etapa 7 (Verificación real) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verificar kie-cli contra la API real de kie.ai en tres partes:
- Un barrido offline de los 227 schemas reales ya capturados.
- Smoke tests manuales detrás del tag `live`, gratis primero y pagados después, con presupuesto de créditos.
- Fixtures reales de `recordInfo` que pasan a ser tests de regresión offline.

Los bugs que aparezcan se corrigen.

**Architecture:**
- **Barrido offline:** es un test normal (sin key) sobre `testdata/live/schemas/`.
- **Smoke tests:** viven en `internal/cli/live_test.go` (`//go:build live`). Corren el CLI en proceso con `cli.NewRoot` y el entorno real, y solo cuando `KIE_E2E=1`. Los que generan (gastan créditos) piden además `KIE_E2E_PAID=1`.
- **Fixtures:** las respuestas reales de `recordInfo` se guardan, saneadas, en `testdata/live/records/`. Los tests de `internal/result` y `internal/cli` las usan.
- **Fixes:** los bugs encontrados se arreglan en tareas de fix que se definen al ver los resultados (Task 4).

**Tech Stack:** Go 1.27, sin dependencias nuevas.

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`:
- §8, fila 7.
- §9, smoke tests reales.
- §10, riesgos.
- §8, regla de parada: si el catálogo o los schemas contradicen el diseño, el builder se detiene y consulta al dueño.

Pendientes de verificación acumulados: `docs/superpowers/plans/2026-10-17-etapa-{1,3,4,5}-pendientes.md`.

## Global Constraints

- **La key (decisión del dueño):**
  - Solo se carga con `set -a; . ./.env; set +a`.
  - Nunca se hace `cat`, `Read` ni print de `.env` o de la key.
  - La key no aparece en stdout, stderr, logs de test, fixtures ni commits.
  - Cada fixture escrito se verifica con `bytes.Contains(data, []byte(key)) == false` antes de escribirlo.
- **Presupuesto:** **80 créditos en total** (spec §9).
  - Esta etapa gasta como máximo **30**, y el resto queda de reserva.
  - Antes de cada generación se leen `credits` y el costo esperado. El test hace `t.Skip` si el costo supera lo que queda del presupuesto de la corrida (`KIE_E2E_BUDGET`, por defecto `30`, menos lo gastado desde el inicio de la corrida).
  - No se genera video (solo `--dry-run`).
- **Los smoke tests corren solo a mano:** tag `live` + `KIE_E2E=1` (y `KIE_E2E_PAID=1` para los pagados). CI nunca los corre; `workflows_test.go` ya lo vigila.
- **Los subagentes implementadores solo pueden correr los smoke tests gratis** (`KIE_E2E=1`, sin `KIE_E2E_PAID`). La corrida pagada la ejecuta el controlador una sola vez, después de revisar Task 3.
- **Regla de parada (spec §8):** si un resultado real contradice el diseño (por ejemplo, el endpoint de recovery no existe, Suno no devuelve `data[]` ni `sunoData[]`, o el upload rechaza el body chunked), se para y se consulta al dueño antes de cambiar el diseño. Los bugs que no tocan el diseño se arreglan en Task 4.
- **Fixtures reales:** guardan `data` de `recordInfo` sin headers y sin la key. Las URLs firmadas del CDN se reemplazan por `https://cdn.example/<basename>` y se conserva la extensión.

## Costos esperados de la corrida pagada (de `pricingDesc` en `testdata/live/models.json`)

| test | invocación | costo |
|---|---|---|
| imagen | `run z-image -p prompt=… -p aspect_ratio=1:1` | 0.8 |
| upload + encadenar | `run recraft/remove-background -p image=<png local del test anterior>` | 1 |
| no-wait + task wait | `run z-image … --no-wait`, después `task wait <id>` | 0.8 |
| preset imagen | `run @imagen-rapida -p prompt=…` | 4 |
| preset TTS | `run @voz -p text="<~60 caracteres>"` | ~1 |
| preset Suno | `run @instrumental -p prompt=…` | 12 |
| recovery (opcional) | `run ai-music-api/suno-recovery-audio -p task_id=<Suno>`, solo si `models price` da ≤3 | ≤3 |

Total ≈ 20–23 créditos. El modelo sync `gemini-omni-audio` queda fuera: pide `audio_id` + `name` (crea una voz a partir de un audio) y no tiene un uso barato claro; su ruta sigue cubierta por los mocks.

## Review Focus

1. **Fuga de la key:** un fixture o un log de test con la key. → Task 3 rechaza escribir si la contiene, y el cierre de la etapa corre el escaneo `git grep -F "$KIE_API_KEY" $(git rev-list --all)` igual a 0.
2. **Gasto de más:** un test pagado cuyo costo no se conoce o supera el saldo restante tiene que hacer `t.Skip`, nunca correr. → unit test del helper de presupuesto en Task 2 (sin tag, siempre compila).
3. **El tag `live` se compila en `go test ./...` normal:** sin el tag, los tests live no compilan ni corren, y con el tag pero sin `KIE_E2E=1` se saltan. → verificación en Task 2 (`go test ./internal/cli/` sin el tag no los lista; con `-tags live` y sin env, todos en SKIP).
4. **La forma real de Suno** (`response.data[]` vs `sunoData[]`, el `Content-Type` del CDN): con el record real, `result.Extract` y `URLs` dan 2 `audio_ids` y 2 URLs de audio, y el `kind` descargado es `audio`. → test offline en Task 4 con el fixture capturado.
5. **Barrido offline del catálogo:** si algún schema real no compila, o rompe el resumen o la detección de campos de archivo, el test lo nombra (no es un panic). → Task 1.

---

### Task 1: Barrido offline de los schemas reales

**Files:**
- Create: `internal/schema/sweep_test.go`
- Create: `testdata/live/SWEEP.md` (generado por el test con `-update`)

**Interfaces:**
- Consumes: `schema.Doc`, `Summarize`, `InputSchema`, `IsJob`, `Validate`, `HasURIFormat`; `upload.FileKey` (si importarlo desde el test de `schema` crea un ciclo, replicar el sufijo `_url`/`_urls` con un comentario `ponytail:`).

- [ ] **Step 1: Write the test**

```go
func TestSweepLiveSchemas(t *testing.T)
// For every testdata/live/schemas/*.json:
//   - unmarshal into schema.Doc. Null schemas (google/gemini-3-8-flash-tts) and non-job schemas (chat, omni) are counted, not failed.
//   - for job schemas: InputSchema() OK; Summarize(doc) OK; Validate(sch, map[string]any{}) returns (problems, nil):
//     the schema compiles and the error is never non-nil.
//   - collect file fields per model: top-level keys where upload.FileKey(k) || schema.HasURIFormat(sch, k).
// Fail listing every model whose step errored (all at once, not on the first).
// With -update (flag in the test file), write testdata/live/SWEEP.md: totals (job / non-job / null / errors),
// models with 0 file fields whose taskType has "Image to", "Video to" or "Audio to" (likely detection gaps),
// and one line per model: `model: field, field`.
```

- [ ] **Step 2: Run** `go test ./internal/schema/ -run Sweep -v`. If there are failures, the test names them. Each one is a real bug: fix it in this same task (`schema`), with a minimal unit test that reproduces it.

- [ ] **Step 3: Run** `go test ./internal/schema/ -run Sweep -update`. Review `SWEEP.md`. In the report, list the models suspected of detection gaps and whether they are real gaps (the field accepts files and is not detected). Gaps that the heuristic can fix without false positives go in this task, with a test.

- [ ] **Step 4: Run** `make test && golangci-lint run ./...`. Expected: green.

- [ ] **Step 5: Commit** `test: sweep all live schemas offline`

---

### Task 2: Arnés live y smoke tests gratis

**Files:**
- Create: `internal/cli/live_budget_test.go` (no build tag): pure budget helper plus its unit test.
- Create: `internal/cli/live_test.go` (`//go:build live`)
- Modify: `README.md`, Development section: how to run (`set -a; . ./.env; set +a; KIE_E2E=1 go test -tags live -run Live ./internal/cli/ -v`, plus `KIE_E2E_PAID=1` for the paid tests, and `KIE_E2E_BUDGET`).

**Interfaces:**
- Produces (in `live_budget_test.go`, package `cli_test`):
  - `type liveBudget struct{ limit, start float64 }`
  - `func (b liveBudget) allow(current, cost float64) (ok bool, reason string)`: `ok` iff `cost > 0 && (b.start-current)+cost <= b.limit`. `cost <= 0` (unknown) → false, "unknown cost".
- Produces (in `live_test.go`):
  - `func liveEnv(t *testing.T) (run func(args ...string) (map[string]any, int))`: `t.Skip` unless `KIE_E2E=1` and `KIE_API_KEY` is non-empty. It runs `cli.NewRoot` with `Deps{Stdout, Stderr, Stdin, Env: os.Getenv, Now: time.Now}`, parses the single JSON object from stdout and returns the exit code. It never `t.Log`s the env or the full stderr (only the lines with `warning:`).
  - `func paid(t *testing.T, cost float64)`: `t.Skip` unless `KIE_E2E_PAID=1`. It reads `credits` and applies `liveBudget.allow`. The `start` of the run is measured once (`sync.Once`), and the budget comes from `KIE_E2E_BUDGET` (default 30).

- [ ] **Step 1: Write the failing unit test** (no tag):

```go
func TestLiveBudgetAllow(t *testing.T) // Review Focus 2, table:
// limit 30, start 80: current 80 cost 4 -> ok; current 60 cost 12 -> false (20 spent + 12 > 30);
// current 52 cost 2 -> ok (28+2=30); cost 0 -> false "unknown cost"; cost -1 -> false
```

- [ ] **Step 2: Run** `go test ./internal/cli/ -run LiveBudget`. Expected: FAIL. Then implement and run again: PASS.

- [ ] **Step 3: Write the free live tests** in `live_test.go`. Each one starts with `run := liveEnv(t)`:

```go
func TestLiveAuthAndCredits(t *testing.T)   // auth status -> ok, source env; credits -> numeric balance > 0
func TestLiveCatalog(t *testing.T)          // models list -> >= 200 models; models search suno -> includes ai-music-api/generate
func TestLiveSchemas(t *testing.T)          // models schema for: z-image, recraft/remove-background, veo-3-1, ai-music-api/generate,
                                            // elevenlabs/text-to-speech-multilingual-v2, kling/v2-5-turbo-image-to-video-pro -> exit 0, fields non-empty
func TestLivePrices(t *testing.T)           // models price for each model of the paid table -> exit 0; t.Logf the pricing text (no key in it)
func TestLiveDryRunPresets(t *testing.T)    // run @<name> --dry-run for each of the 10 factory presets with the missing fields filled with
                                            // placeholders (prompt=test, image_url(s)=a local temp PNG, text=hola) -> exit 0
                                            // (Spec §5 check: the factory presets validate against the live schema)
func TestLiveDryRunVideoChain(t *testing.T) // run veo/extend -p task_id=fake-task -p prompt=x --dry-run -> exit 0
```

- [ ] **Step 4: Verify the gating** (Review Focus 3):
  - `go test ./internal/cli/ -v -run Live 2>&1 | grep -c '=== RUN   TestLive[A-Z]'` gives `0`; only `TestLiveBudgetAllow` runs.
  - `go test -tags live ./internal/cli/ -run Live -v` (without `KIE_E2E`): every live test is SKIP.
  - `go vet -tags live ./...` is clean.

- [ ] **Step 5: Run the free tests** (allowed: they cost 0 credits): `set -a; . ./.env; set +a; KIE_E2E=1 go test -tags live -run Live ./internal/cli/ -v 2>&1 | tail -60`. Put the real results in the report. **Never** print `.env` or the key. A failure here is a real finding: write it in the report with the exact error (without the key) and do not fix it in this task, unless the failure is in the test itself.

- [ ] **Step 6: Run** `make test && golangci-lint run ./... && golangci-lint run --build-tags live ./...`. Expected: green.

- [ ] **Step 7: Commit** `test: add live smoke harness and free live checks`

---

### Task 3: Smoke tests pagados y captura de records reales

**Files:**
- Modify: `internal/cli/live_test.go`
- Create (generado en la corrida pagada): `testdata/live/records/<model __>.json`

**Interfaces:**
- Consumes: `liveEnv`, `paid` (Task 2).
- Produces: `func captureRecord(t *testing.T, taskID string)`. It calls `GET /api/v1/jobs/recordInfo?taskId=` through `api.Client` (same config as the CLI) and writes `data` to `testdata/live/records/<model with "/"→"__">.json`, but only if `KIE_E2E_CAPTURE=1`.
  - Saneado: every `https://…` string inside `resultJson` is rewritten to `https://cdn.example/<basename>`; `taskId` is kept.
  - If the resulting bytes contain the key: `t.Fatal("refusing to write: fixture contains the API key")`.

- [ ] **Step 1: Write the paid tests.** Use the costs from the plan table. Each test starts with `run := liveEnv(t); paid(t, <cost>)`, and they run in this order:

```go
func TestLivePaid1Image(t *testing.T)       // run z-image -p prompt="a red apple on white" -p aspect_ratio=1:1 --out <dir>
                                            // -> exit 0, files[0].kind image, file exists > 0 bytes; captureRecord; keep path for Paid2
func TestLivePaid2UploadChain(t *testing.T) // run recraft/remove-background -p image=<Paid1 local png> -> exit 0, uploaded has the path,
                                            // files[0] exists (stage-1 note: chunked upload accepted). captureRecord
func TestLivePaid3NoWait(t *testing.T)      // run z-image ... --no-wait -> taskId; task wait <id> --out <dir> -> files[0] exists
func TestLivePaid4PresetImage(t *testing.T) // run @imagen-rapida -p prompt="a lighthouse at dusk" -> exit 0, preset == "imagen-rapida"
func TestLivePaid5PresetVoice(t *testing.T) // run @voz -p text="Hola, esta es una prueba de kie-cli." -> files[0].kind audio. captureRecord
func TestLivePaid6Suno(t *testing.T)        // run @instrumental -p prompt="calm lo-fi piano" --timeout 15m -> len(ids.audio_ids)==2,
                                            // 2 files with kind audio (stage-3 note: real CDN Content-Type). captureRecord;
                                            // then run ai-music-api/extend -p audio_id=<ids.audio_ids[0]> -p model=V6 ... --dry-run -> exit 0
func TestLivePaid7Recovery(t *testing.T)    // run ai-music-api/suno-recovery-audio -p task_id=<Paid6 taskId>, only if models price
                                            // parses to <= 3; otherwise t.Skip with the price text. Verifies the recovery endpoint (stage-4 note)
```

Paid tests share state (Paid1's PNG, Paid6's task) through package vars. If a dependency is missing (its test was skipped or failed), skip.

- [ ] **Step 2: Verify without spending:**
  - `go vet -tags live ./...` and `golangci-lint run --build-tags live ./...` are clean.
  - `set -a; . ./.env; set +a; KIE_E2E=1 go test -tags live -run LivePaid ./internal/cli/ -v` → all SKIP (no `KIE_E2E_PAID`).
  - The implementer **does not** set `KIE_E2E_PAID`.

- [ ] **Step 3: Commit** `test: add paid live smoke tests with record capture`

- [ ] **Step 4 (controller, once, after the Task 3 review):**
  - Run `set -a; . ./.env; set +a; KIE_E2E=1 KIE_E2E_PAID=1 KIE_E2E_CAPTURE=1 KIE_E2E_BUDGET=30 go test -tags live -run 'Live' ./internal/cli/ -v -timeout 40m`.
  - Write `credits` before and after into the ledger, together with each test's result.
  - Check that the fixtures in `testdata/live/records/` do not contain the key (`grep -F` with the variable loaded, printing only the count).
  - Commit `test: capture real recordInfo fixtures` (fixtures only).

---

### Task 4: Regresión con records reales y fixes de los hallazgos

**Files:**
- Modify: `internal/result/result_test.go` (and `internal/cli/*_test.go` if a finding requires it)
- Modify: production code, only what each finding requires.

**Interfaces:**
- Consumes: the `testdata/live/records/*.json` fixtures (Task 3 Step 4), and the findings list the controller writes to the ledger after the paid run (and after the free run in Task 2).

- [ ] **Step 1: Write the regression tests with the real records:**

```go
func TestRealRecords(t *testing.T) // Review Focus 4
// For each testdata/live/records/*.json: parse resultJson -> result.URLs non-empty and http(s);
// result.Extract: for the Suno record len(AudioIDs)==2 and URLs has 2 audio URLs; for z-image/recraft/TTS 1 URL
```

- [ ] **Step 2:** For each finding in the list, write a failing test that reproduces it, apply the minimal fix, and see it pass. Findings that contradict the design are not fixed here: the controller already escalated them to the owner (Global Constraints).

- [ ] **Step 3: Update** the docs that change because of real findings (skill, references, README, presets). The consistency tests must still pass.

- [ ] **Step 4: Run** `make test && golangci-lint run ./... && golangci-lint run --build-tags live ./...`. Expected: green.

- [ ] **Step 5: Commit** `fix: <finding>` (one commit per finding) and `test: regression tests from real kie.ai records`
