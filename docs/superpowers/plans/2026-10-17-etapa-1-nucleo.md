# kie-cli Etapa 1 (Núcleo) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Binario `kie-cli` que autentica, explora el catálogo vivo, resume schemas, sube archivos locales, crea tareas, espera y descarga resultados, con salida JSON estable y códigos de salida.

**Architecture:** Paquetes pequeños en `internal/` (output, config, api, cache, catalog, schema, ledger, task, upload, download, result) y comandos cobra en `internal/cli/`. Cada paquete recibe sus dependencias (cliente HTTP, reloj, sleep) por parámetro para probarlo con `httptest`. Ningún test usa la API real.

**Tech Stack:** Go 1.27, `github.com/spf13/cobra`, `golang.org/x/term` (lectura sin eco). El resto con stdlib.

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md` (secciones 3, 4 y 7, y la fila "1" de la tabla de §8). La investigación de la API está en `docs/research/`, sobre todo en `kie-official-skill-reference.md`.

## Global Constraints

- Módulo `github.com/mik3dev/kie-cli`, binario `kie-cli`, `go 1.27` en go.mod.
- Base URL `https://api.kie.ai`, upload host `https://kieai.redpandaai.co`. Ambas se pueden sobrescribir con `KIE_BASE_URL` y `KIE_UPLOAD_URL` (para tests y smoke).
- Header `Authorization: Bearer <key>` en todas las llamadas. La key nunca aparece en stdout, stderr, errores ni archivos fuera de `config.json`.
- Envelope `{code,msg,data}`: HTTP 200 no significa éxito, hay que revisar `code`.
- Mapeo de códigos de kie: 401→`auth`, 402→`credits`, 422/400/404→`api` (salida 2 si vino de validación local), 429/455/433→`rate_limit`, 5xx→`api`.
- Códigos de salida: 0 ok · 1 otro · 2 uso/validación · 3 auth · 4 créditos · 5 tarea falló · 6 timeout · 7 rate limit/servicio no disponible · 8 `submit_uncertain`.
- stdout lleva **exactamente un** objeto JSON por comando. Progreso y avisos van por stderr (`--quiet` los silencia).
- Estado en `~/.kie-cli/` (0700), o en `KIE_CLI_HOME` si está definido. `config.json` va con 0600.
- Rate limits: el grupo discovery (`/api/v1/models*`) permite 1 req/s (espaciar ≥1.1s), `createTask` 20/10s y `recordInfo` 10/s.
- Caché: catálogo 1h, schemas 24h.
- Polling: backoff de 3s a 15s (×1.5). Timeout por defecto 30m si algún `taskType` del modelo contiene "Video" y 10m en otro caso.
- Retención: las subidas duran 24h y los resultados 14 días. Por eso se descarga por defecto a `./kie-output/`.
- Inmutabilidad: las funciones que transforman `input` devuelven un mapa nuevo y no mutan el recibido.
- Cobertura ≥80% por paquete (`go test -cover ./...`).
- **La key real solo se usa en la Task 4 (captura, solo lectura) y nunca en CI ni en `go test` sin tag.**

## Review Focus

1. **Un prompt que coincide con un archivo existente** (p. ej. `-p prompt=README.md`) no debe subirse. La subida automática solo aplica a claves cuyo nombre contiene `url`, `image`, `video`, `audio`, `file` o `mask`. → test en Task 9.
2. **Respuesta de kie con HTTP 200 + `code` 402 en `createTask`**: debe salir con código 4 y no dejar entrada `unknown` en el ledger. → test en Task 8.
3. **Interrupción (Ctrl-C) durante `wait`**: la tarea queda en el ledger y stdout emite un JSON con `taskId` y `error.code="timeout"` para que el agente pueda retomarla con `task wait`. → test en Task 8 (cancelación de contexto).
4. **Dos resultados con el mismo nombre o un archivo ya existente en `--out`**: nunca se sobrescribe (sufijo `-1`, `-2`). → test en Task 10.
5. **`config.json` con permisos 0644**: aviso por stderr, pero funciona. **`KIE_API_KEY` vacía (`""`)**: se trata como no definida. → test en Task 2.

---

### Task 1: Esqueleto, contrato de salida y `version`

**Files:**
- Create: `go.mod`, `cmd/kie-cli/main.go`, `internal/output/output.go`, `internal/cli/root.go`, `internal/cli/version.go`, `Makefile`, `.gitignore`
- Test: `internal/output/output_test.go`, `internal/cli/version_test.go`

**Interfaces:**
- Produces:
  - `output.Error{Code string; Message string; Details map[string]any}` implementa `error`. `func (e *Error) ExitCode() int`.
  - Constantes `output.CodeValidation="validation"`, `CodeAuth="auth"`, `CodeCredits="credits"`, `CodeRateLimit="rate_limit"`, `CodeSubmitUncertain="submit_uncertain"`, `CodeTaskFailed="task_failed"`, `CodeTimeout="timeout"`, `CodeAPI="api"`, `CodeIO="io"`, `CodeUsage="usage"`.
  - `func output.Success(w io.Writer, v map[string]any) error` escribe `{"ok":true,...v}`.
  - `func output.Fail(w io.Writer, err error) int` escribe `{"ok":false,"error":{code,message,details}}` y devuelve el exit code. Un error que no es `*output.Error` se trata como `CodeAPI` con salida 1.
  - `func cli.NewRoot(deps Deps) *cobra.Command`, con `type Deps struct{Stdout, Stderr io.Writer; Stdin io.Reader; Env func(string) string; Now func() time.Time}`. Flags persistentes: `--quiet`, `--debug`.
  - `func cli.Execute(deps Deps, args []string) int`: el error de un comando pasa por `output.Fail`, y los errores de uso de cobra (flag desconocido, args) por `CodeUsage` con salida 2. `SilenceUsage` y `SilenceErrors` activos para que stdout solo tenga JSON.
  - Variable `cli.Version = "dev"`, sobrescrita con `-ldflags`.

- [ ] **Step 1: Write the failing tests**

```go
func TestExitCodes(t *testing.T) {
    cases := map[string]int{"usage":2,"validation":2,"auth":3,"credits":4,"task_failed":5,"timeout":6,"rate_limit":7,"submit_uncertain":8,"api":1,"io":1}
    for code, want := range cases {
        if got := (&output.Error{Code: code}).ExitCode(); got != want { t.Errorf("%s: got %d want %d", code, got, want) }
    }
}
func TestFailWritesSingleJSON(t *testing.T) {
    var b bytes.Buffer
    exit := output.Fail(&b, &output.Error{Code: "auth", Message: "no key"})
    // exit == 3; b decodes to exactly one object {"ok":false,"error":{"code":"auth","message":"no key"}}; no trailing data
}
func TestVersionCommand(t *testing.T) {
    // Execute(deps, []string{"version"}) == 0 ; stdout == {"ok":true,"version":"dev"} + "\n"
}
func TestUnknownFlagIsUsageError(t *testing.T) {
    // Execute(deps, []string{"version","--nope"}) == 2 ; stdout JSON error.code == "usage"; stderr may have text
}
```

- [ ] **Step 2: Run** `go test ./...`. Expected: FAIL (paquetes inexistentes).
- [ ] **Step 3: Implement** lo indicado en Interfaces. `go mod init github.com/mik3dev/kie-cli`, `go get github.com/spf13/cobra`. El Makefile tiene `build` (`go build -ldflags "-X github.com/mik3dev/kie-cli/internal/cli.Version=$(git describe --tags --always)" -o bin/kie-cli ./cmd/kie-cli`), `test` (`go test -race -cover ./...`) e `install`. `.gitignore` ya existe (contiene `.env`, `.env.*` y `!.env.example`). Hay que **conservar esas líneas** y añadir `bin/`, `kie-output/` y `testdata/live/.raw/`.
- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: scaffold kie-cli with JSON output contract and version`

---

### Task 2: Config, home y `auth login|status`

**Files:**
- Create: `internal/config/config.go`, `internal/cli/auth.go`
- Test: `internal/config/config_test.go`, `internal/cli/auth_test.go`

**Interfaces:**
- Consumes: `cli.Deps`, `output.Error`.
- Produces:
  - `type config.Paths struct{Home, Config, Presets, Cache, Ledger string}`.
  - `func config.ResolvePaths(env func(string) string) (Paths, error)`: usa `KIE_CLI_HOME` o, si no está, `$HOME/.kie-cli`. Los archivos son `config.json`, `presets.yaml`, `cache/` y `tasks.jsonl`.
  - `func config.EnsureHome(p Paths) error`: crea el directorio con 0700.
  - `type config.Config struct{APIKey string \`json:"api_key,omitempty"\`; OutDir string \`json:"out_dir,omitempty"\`}`.
  - `func config.Load(p Paths, warn io.Writer) (Config, error)`: si el archivo no existe, devuelve un Config vacío sin error. Si `perm&0o077 != 0`, escribe en warn `warning: <path> permissions are <perm>, should be 0600`.
  - `func config.Save(p Paths, c Config) error`: escribe en un temp con 0600 y luego hace rename.
  - `func config.ResolveKey(env func(string) string, c Config) (key, source string, err error)`: el source es `"env"` o `"config"`. Sin key, devuelve `*output.Error{Code:"auth", Message:"no API key: set KIE_API_KEY or run 'kie-cli auth login'"}`.
  - `kie-cli auth login`: lee la key con `term.ReadPassword` si stdin es TTY y, si no, la primera línea de stdin. La guarda y emite `{"ok":true,"saved":"<path>"}`.
  - `kie-cli auth status`: emite `{"ok":true,"source":"env|config","key":"sk-…abcd"}`. Se muestran solo los últimos 4 caracteres, y nada si la key mide menos de 12.
  - `func config.Redact(key string) string`, usada también por `api`.

- [ ] **Step 1: Write the failing tests**: `TestResolvePathsHonorsKIE_CLI_HOME`, `TestSaveWrites0600AndHome0700`, `TestLoadWarnsOnLoosePerms` (chmod 0644, warn contiene "should be 0600" y Load devuelve la key), `TestResolveKeyEnvBeatsConfig`, `TestResolveKeyEmptyEnvFallsBackToConfig` (`KIE_API_KEY=""` usa config), `TestResolveKeyMissing` (`Code=="auth"`), `TestAuthLoginFromStdinPipe` (stdin `"abc123456789xyz\n"`, luego `auth status` devuelve `key` terminado en `"…9xyz"` y stdout nunca contiene `abc123456789xyz`), `TestRedactShortKey` (`Redact("short")=="****"`).
- [ ] **Step 2: Run** `go test ./internal/config/ ./internal/cli/`. Expected: FAIL.
- [ ] **Step 3: Implement** según Interfaces. `go get golang.org/x/term`.
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: config home ~/.kie-cli and auth login/status`

---

### Task 3: Cliente API

**Files:**
- Create: `internal/api/client.go`, `internal/api/errors.go`, `internal/api/limiter.go`
- Test: `internal/api/client_test.go`, `internal/api/limiter_test.go`

**Interfaces:**
- Consumes: `output.Error`, `config.Redact`.
- Produces:
  - `type api.Client struct{BaseURL, UploadURL, Key string; HTTP, UploadHTTP *http.Client; Debug io.Writer; Sleep func(context.Context, time.Duration) error}`.
  - `func api.New(key string, env func(string) string) *Client`: los defaults salen de Global Constraints. `HTTP.Timeout` es 60s y el de upload 10m (cliente aparte, `UploadHTTP`).
  - `func (c *Client) Do(ctx context.Context, req Request, out any) error`, con `type Request struct{Method, Path string; Query url.Values; Body any; Host Host; Class Class}`, `Host` = `HostAPI|HostUpload` y `Class` = `ClassDiscovery|ClassCreate|ClassRecord|ClassOther`.
    - Decodifica `{code,msg,data}`. Si `code!=200`, devuelve `*api.Error{KieCode int; Msg string}`. Si out≠nil, hace unmarshal de `data` en out.
    - Si el HTTP no es 2xx, devuelve `*api.Error{KieCode: status}`.
    - Si falla la red, devuelve `*api.NetError{Sent bool; Err error}`. `Sent=false` cuando el error ocurre al conectar o al escribir (`*net.OpError` con Op "dial", o error antes de recibir headers sin haber escrito el body). Si hay duda, `Sent=true`.
  - `func (e *api.Error) AsOutput() *output.Error` mapea según Global Constraints. `Details` = `{"kie_code":N,"kie_msg":...}`.
  - Reintentos dentro de `Do` **solo** cuando `Method==GET`, `KieCode==429` o `NetError.Sent==false`: hasta 3, con backoff 1s/2s/4s mediante `c.Sleep`. POST `createTask` nunca se reintenta si `NetError.Sent==true` o hay 5xx/455.
  - Limitador en proceso por `Class`: Discovery con espacio mínimo de 1.1s, Create a 20/10s (ventana deslizante) y Record a 10/s. `// ponytail: in-process only; parallel kie-cli processes don't coordinate, add file lock if 429s show up.`
  - `--debug`: escribe en `Debug` el método, la URL y el status, con `Authorization: Bearer ****`.

- [ ] **Step 1: Write the failing tests** (con `httptest.Server`):
  - `TestDoDecodesData` (code 200 → out lleno).
  - `TestHTTP200WithErrorCode` (body `{"code":402,"msg":"insufficient"}` → `AsOutput().Code=="credits"`, ExitCode 4).
  - `TestAuthHeaderAndNoKeyLeak` (el server ve `Bearer k-secret`; con `Debug` a un buffer y un error 401, ni el buffer ni `err.Error()` contienen `k-secret`).
  - `TestGetRetries429ThenOK` (2× code 429 y luego 200 → ok, 3 requests).
  - `TestPostCreateNoRetryOn500` (1 request, error con KieCode 500).
  - `TestPostCreateRetries429` (429 y luego 200 → 2 requests).
  - `TestDialErrorIsNotSent` (BaseURL a un puerto cerrado → `NetError.Sent==false`).
  - `TestServerHangupAfterBodyIsSent` (el handler hijackea y cierra después de leer el body → `Sent==true`).
  - `TestDiscoveryLimiterSpacing` (con reloj y sleep falsos, 3 llamadas discovery suman ≥2.2s de sleep).
- [ ] **Step 2: Run** `go test ./internal/api/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./internal/api/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: kie API client with envelope errors, retries and rate limiting`

---

### Task 4: Captura real (solo lectura) — PUNTO DE CONTROL

**Files:**
- Create: `scripts/capture/main.go` (package main, usa `internal/api`)
- Create (output): `testdata/live/models.json`, `testdata/live/schemas/<model con "/"→"__">.json`, `testdata/live/price/<…>.json`, `testdata/live/success-rate/<…>.json` (de 5 modelos), `testdata/live/credit.json`, `testdata/live/REPORT.md`

**Interfaces:**
- Consumes: `api.New`, `api.Client.Do` con `ClassDiscovery`.
- Produces: fixtures en `testdata/live/` usadas por las Tasks 5, 6 y 9. Se guarda solo `data` de cada respuesta, nunca headers ni la key.

- [ ] **Step 1: Comprobar la key sin mostrarla.** La key está en `.env` (raíz del repo, gitignored). Verificar con `git check-ignore -q .env && grep -q '^KIE_API_KEY=.\+' .env && echo ok`. **Nunca** hacer `cat`, `Read` ni imprimir `.env`. Si no hay key, **detenerse y avisar**: no hay que inventar fixtures.
- [ ] **Step 2: Implement** `scripts/capture/main.go`. Solo llama `GET /api/v1/models`, `GET /api/v1/models/{m}/schema` de cada modelo (slash sin codificar), `price` y `success-rate` de 5 modelos (1 imagen, 1 video, 1 suno, 1 tts y `veo-3-1` si existe) y `GET /api/v1/chat/credit`. **Prohibido** llamar `createTask`, upload o `download-url`. Escribe `REPORT.md` con el total de modelos, el conteo por `taskType`, los schemas `null`, los paths distintos de `/api/v1/jobs/createTask`, si aparecen `ai-music-api/`, `veo` y `runway`, y la lista de nombres de propiedad de `input` que contengan `url|image|video|audio|file|mask`.
- [ ] **Step 3: Run** `set -a; . ./.env; set +a; go run ./scripts/capture -out testdata/live`. Expected: tarda unos 4 min y escribe los archivos. Comparar `credit.json` antes y después: debe quedar igual (cero créditos gastados).
- [ ] **Step 4: Verify** con `set -a; . ./.env; set +a; grep -rF "$KIE_API_KEY" testdata/live | wc -l`. Expected: `0`.
- [ ] **Step 5: PUNTO DE CONTROL.** Mostrar `REPORT.md` al dueño. **Detenerse y consultar** si falta Suno, Veo o Runway en el catálogo, si más del 20% de los schemas son `null`, o si la forma de `data` difiere de `docs/research/kie-official-skill-reference.md`.
- [ ] **Step 6: Commit** `test: capture live catalog and schemas as fixtures (read-only, 0 credits)`

---

### Task 5: Caché, catálogo y `credits`

**Files:**
- Create: `internal/cache/cache.go`, `internal/catalog/catalog.go`, `internal/cli/models.go` (list, search, price, health), `internal/cli/credits.go`
- Test: `internal/cache/cache_test.go`, `internal/catalog/catalog_test.go`, `internal/cli/models_test.go`

**Interfaces:**
- Consumes: `api.Client`, `config.Paths.Cache`.
- Produces:
  - `func cache.Get(dir, name string, ttl time.Duration, now time.Time, into any) (hit bool, err error)` y `func cache.Put(dir, name string, v any) error`. El nombre del archivo se saca de `name` reemplazando "/" por "__".
  - `type catalog.Model struct{Model, Slug, Title, Provider string; TaskType []string \`json:"taskType"\`; Description *string; PricingDesc string \`json:"pricingDesc"\`}`.
  - `type catalog.Service struct{API *api.Client; CacheDir string; Now func() time.Time}`.
  - `func (s *Service) List(ctx, refresh bool) ([]Model, error)`: siempre pide el catálogo **sin filtros** y lo guarda en caché 1h.
  - `func catalog.Filter(ms []Model, q Query) []Model`, con `type Query struct{Type, Provider, Text string}`. Las comparaciones no distinguen mayúsculas. `Type` busca por subcadena en cualquier `taskType`, de modo que `--type video` matchea "Text to Video". `Text` busca en model, title y description.
  - `func (s *Service) Find(ctx, model string) (Model, bool, error)`.
  - `func (s *Service) Price(ctx, model) (string, error)` y `func (s *Service) Health(ctx, model) (json.RawMessage, error)`.
  - Comandos: `models list [--type --provider --refresh]` emite `{"ok":true,"total":N,"models":[...]}`. `models search <text> [--type --provider]` produce lo mismo, filtrado. `models price <m>` emite `{"ok":true,"model":m,"pricingDesc":"…"}` y `models health <m>` `{"ok":true,"model":m,"points":[…]}`. `credits` emite `{"ok":true,"credits":<number>}`.
- [ ] **Step 1: Write the failing tests** usando `testdata/live/models.json` y `credit.json` servidos por httptest:
  - `TestListUsesCacheWithinTTL` (2 llamadas en menos de 1h → 1 request).
  - `TestListRefreshBypassesCache`.
  - `TestFilterTypeVideoSubstring` (todos los resultados tienen un taskType con "video", y `len>0`).
  - `TestSearchText`.
  - `TestCreditsCommand` (stdout `{"ok":true,"credits":<valor de credit.json>}`).
  - `TestModelsListEmptyFilterIsOK` (filtro sin resultados → ok, total 0, salida 0).
- [ ] **Step 2: Run** `go test ./internal/cache/ ./internal/catalog/ ./internal/cli/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: models list/search/price/health with cache, and credits`

---

### Task 6: Schema, resolución `$ref` y `models schema`

**Files:**
- Create: `internal/schema/schema.go`, `internal/schema/ref.go`, `internal/schema/summary.go`, `internal/cli/schema.go`
- Test: `internal/schema/*_test.go`, `internal/cli/schema_test.go`

**Interfaces:**
- Consumes: `api.Client`, `cache`.
- Produces:
  - `type schema.Doc struct{Model string \`json:"model"\`; OpenAPI map[string]any \`json:"openapi"\`}`. `OpenAPI` vale nil si el schema llegó `null`.
  - `func schema.Fetch(ctx, c *api.Client, cacheDir, model string, refresh bool, now time.Time) (Doc, error)`. Usa caché 24h.
  - `func (d Doc) Endpoint() (method, path string, err error)`: toma el primer path y su método. Si `OpenAPI==nil`, devuelve `("POST","/api/v1/jobs/createTask",nil)` y `d.Null()==true`.
  - `func (d Doc) IsJob() bool`: path == `/api/v1/jobs/createTask`.
  - `func schema.Resolve(doc map[string]any, node any) (any, error)`: devuelve una copia con todos los `$ref` locales resueltos de forma recursiva. Hace percent-decode de cada segmento y **no** hace trim de espacios. Detecta ciclos y, en ese caso, deja `{"$ref":…}` sin expandir.
  - `func (d Doc) InputSchema() (map[string]any, error)`: toma el requestBody `application/json` de la operación, resuelto, y devuelve su `properties.input`.
  - `type schema.Field struct{Name string; Type string; Required bool; Enum []any; Default any; Description string; Minimum, Maximum *float64; MaxItems *int; Items string}` (en `Items` va el tipo del elemento si es un array).
  - `type schema.Summary struct{Model, Method, Path string; Job bool; Null bool; Fields []Field; Variants [][]Field}`. Si `input` tiene `oneOf`, cada rama es una entrada en `Variants` y `Fields` queda vacío.
  - `func schema.Summarize(d Doc) (Summary, error)`. Los campos van ordenados con los requeridos primero y luego por nombre. `Description` se recorta a 300 runas.
  - Comando `models schema <m> [--raw] [--refresh]`: sin `--raw` emite `{"ok":true,"schema":Summary}` y con `--raw` `{"ok":true,"model":m,"openapi":{…}}`. Si es null, añade `"warning":"schema not synced by kie; run without validation"`.
- [ ] **Step 1: Write the failing tests**:
  - `TestResolvePercentEncodedRef` (ref a `#/components/schemas/response%20not%20with%20recordId` resuelve).
  - `TestResolveTrailingSpaceKey` (`#/components/responses/Error ` resuelve sin trim).
  - `TestResolveCycleTerminates`.
  - `TestResolveDoesNotMutate` (el doc original queda deep-equal a una copia previa).
  - `TestSummarizeGptImage2` (con el schema de `gpt-image-2-text-to-image` de testdata/live, `prompt` es Required y `aspect_ratio` tiene Enum no vacío; si ese modelo no está en la captura, usar el primer schema de imagen no-null y afirmar `len(Fields)>0`).
  - `TestSummarizeOneOfVariants` (cualquier schema capturado con `oneOf` en input → `len(Variants)>=2`; si no hay ninguno, usar una fixture mínima en `internal/schema/testdata/oneof.json`).
  - `TestNullSchema`.
  - `TestSummarizeAllCaptured` (recorre **todos** los `testdata/live/schemas/*.json`: Summarize no hace panic ni devuelve error en ningún schema no-null; registra con `t.Log` cuántos son null o no-job).
- [ ] **Step 2: Run** `go test ./internal/schema/ ./internal/cli/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: schema fetch, $ref resolution and input summary`

---

### Task 7: Ledger local y `task list`

**Files:**
- Create: `internal/ledger/ledger.go`, `internal/cli/task.go` (solo `list` en esta task)
- Test: `internal/ledger/ledger_test.go`

**Interfaces:**
- Produces:
  - `type ledger.Entry struct{TaskID string \`json:"taskId,omitempty"\`; Model string \`json:"model"\`; State string \`json:"state"\`; OutDir string \`json:"outDir,omitempty"\`; CreatedAt time.Time \`json:"createdAt"\`; Note string \`json:"note,omitempty"\`}`. `State` toma los valores `submitted|unknown|success|fail`.
  - `func ledger.Append(path string, e Entry) error`: append de una línea abriendo con `O_APPEND|O_CREATE`, permisos 0600. `// ponytail: append-only; state updates are new lines, List keeps the last per taskId.`
  - `func ledger.List(path string, limit int) ([]Entry, error)`: lo más nuevo primero, con la última línea por `TaskID`. Las entradas sin TaskID (las `unknown`) se listan todas. Ignora líneas corruptas y avisa por un `io.Writer` opcional.
  - Comando `task list [--limit 20]` emite `{"ok":true,"tasks":[…]}`.
- [ ] **Step 1: Write the failing tests**: `TestAppendAndListNewestFirst`, `TestListCollapsesByTaskID` (submitted y luego success del mismo id → 1 entrada success), `TestListSkipsCorruptLine`, `TestLimit`, `TestFilePerm0600`.
- [ ] **Step 2: Run** `go test ./internal/ledger/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: local task ledger and task list`

---

### Task 8: Tareas — submit, get, wait

**Files:**
- Create: `internal/task/task.go`, `internal/task/wait.go`; modificar `internal/cli/task.go` (agregar `get`)
- Test: `internal/task/task_test.go`, `internal/task/wait_test.go`

**Interfaces:**
- Consumes: `api.Client` (`ClassCreate`, `ClassRecord`), `ledger`.
- Produces:
  - `func task.Submit(ctx, c *api.Client, ledgerPath, model string, input map[string]any, outDir string, now time.Time) (taskID string, err error)`. Hace POST a `/api/v1/jobs/createTask` con `{"model":…,"input":…}` (sin `callBackUrl`). Si sale bien, añade una entrada `submitted` al ledger.
    - Si hay `NetError{Sent:true}`, `KieCode>=500` o `KieCode==455`, añade una entrada `unknown` (sin TaskID, con `Note` = mensaje) y devuelve `*output.Error{Code:"submit_uncertain", Message:"task may have been created; check https://kie.ai/logs before resubmitting"}`.
    - Si hay `KieCode` 401, 402 o 422, devuelve `AsOutput()` y **no** escribe en el ledger.
  - `type task.Record struct{TaskID, Model, State string; ResultJSON string \`json:"resultJson"\`; Response json.RawMessage \`json:"response"\`; FailCode, FailMsg string; CostTime int64; CreditsConsumed float64; CreateTime, CompleteTime int64}`. `FailCode` acepta number, string o null (unmarshal tolerante).
  - `func task.Get(ctx, c, id string) (Record, error)`. Si `Response` viene vacío y `ResultJSON` no, lo parsea y lo usa como Response.
  - `func task.Terminal(state string) bool` (`success|fail`).
  - `type task.Waiter struct{Sleep func(context.Context, time.Duration) error; Now func() time.Time; Progress io.Writer}`.
  - `func (w Waiter) Wait(ctx, c, id string, timeout time.Duration) (Record, error)`: aplica backoff de 3s a 15s (×1.5) y en cada cambio de estado escribe `state: <s>` en Progress. Si la tarea falla, devuelve el Record y `*output.Error{Code:"task_failed", Details:{failCode,failMsg}}`. Si se pasa del timeout o el contexto se cancela, devuelve el último Record y `*output.Error{Code:"timeout", Details:{"taskId":id}}`.
  - Comando `task get <id>` emite `{"ok":true,"taskId","model","state","result":<response>,"creditsConsumed","costTime","failCode","failMsg"}`.
- [ ] **Step 1: Write the failing tests** (httptest y sleep falso que avanza el reloj):
  - `TestSubmitOK` (body enviado `{"model":"m","input":{"prompt":"x"}}`, sin callBackUrl, ledger con 1 entrada submitted).
  - `TestSubmit402NoLedger` (exit 4, ledger vacío).
  - `TestSubmit500IsUncertain` (code `submit_uncertain`, exit 8, ledger con 1 entrada `unknown`).
  - `TestSubmitDialErrorRetriesThenFailsNotUncertain` (puerto cerrado → error `api`, no `submit_uncertain`).
  - `TestWaitWalksStates` (waiting, queuing, generating, success → Record success, Progress con 4 líneas `state:`).
  - `TestWaitFail` (exit 5, Details.failMsg).
  - `TestWaitTimeout` (siempre generating, timeout 1m → exit 6, Details.taskId).
  - `TestWaitContextCancel` (cancelar → código timeout con taskId).
  - `TestGetParsesResultJSONWhenResponseMissing`.
  - `TestFailCodeNumberOrString`.
- [ ] **Step 2: Run** `go test ./internal/task/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./internal/task/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: task submit with safe retry policy, get and wait`

---

### Task 9: Subida automática de archivos locales

**Files:**
- Create: `internal/upload/upload.go`
- Test: `internal/upload/upload_test.go`

**Interfaces:**
- Consumes: `api.Client` (`HostUpload`).
- Produces:
  - `func upload.File(ctx, c *api.Client, path string) (downloadURL string, err error)`: hace POST multipart a `/api/file-stream-upload` con `file`, `uploadPath` y `fileName` (basename). `uploadPath` = `kie-cli/<YYYYMMDD>/<16 hex aleatorios>`, sin slash al inicio ni al final. Si el archivo pesa más de 100MB, devuelve `*output.Error{Code:"validation"}` antes de subir. La respuesta de upload trae `success` además de `code`, y se usa `data.downloadUrl`.
  - `func upload.FileKey(key string) bool`: true si el nombre de la clave, en minúsculas, contiene `url`, `image`, `video`, `audio`, `file` o `mask`. `// ponytail: name heuristic; stage 2 switches to schema-based detection.`
  - `func upload.ResolveLocal(ctx, input map[string]any, up func(context.Context, string) (string, error)) (map[string]any, []string, error)`: devuelve un **mapa nuevo** donde, en las claves que cumplen `FileKey` (y en cualquier profundidad de arrays u objetos bajo esas claves), cada string que no empiece por `http://`, `https://`, `oss://` ni `asset://` y que sea un archivo regular existente se reemplaza por la URL subida. También devuelve la lista de rutas subidas. Si un string parece ruta (empieza por `./`, `../`, `/` o `~/`, o tiene extensión de media conocida) pero no existe, devuelve `*output.Error{Code:"validation", Message:"file not found: <path>"}`. `~/` se expande.
- [ ] **Step 1: Write the failing tests**:
  - `TestResolveLocalUploadsFileKeys` (`image_urls:["./a.png","https://x/b.png"]` con a.png existente → `["<uploaded>","https://x/b.png"]`).
  - `TestPromptMatchingFileIsNotUploaded` (`prompt:"a.png"` con a.png existente → sin cambios, 0 subidas). [Review Focus 1]
  - `TestNestedObjectsUnderFileKey` (`video_list:[{"url":"./v.mp4"}]`).
  - `TestMissingPathIsValidationError`.
  - `TestResolveLocalDoesNotMutateInput`.
  - `TestUploadMultipartFields` (el server ve `uploadPath` con el patrón `^kie-cli/\d{8}/[0-9a-f]{16}$` y `fileName=a.png`).
  - `TestUploadTooLarge` (un archivo sparse de 101MB creado con `Truncate` → validation, 0 requests).
- [ ] **Step 2: Run** `go test ./internal/upload/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: auto-upload local files referenced in input`

---

### Task 10: Extracción de resultados y descarga

**Files:**
- Create: `internal/result/result.go`, `internal/download/download.go`
- Test: `internal/result/result_test.go`, `internal/download/download_test.go`

**Interfaces:**
- Consumes: `api.Client` (para el fallback `download-url`).
- Produces:
  - `func result.URLs(response json.RawMessage) []string`: concatena `resultUrls[]` y `lastFrameUrl[]` (si existe), sin duplicados y en orden. `// ponytail: Suno data[].audio_url lands in stage 4.`
  - `type download.File struct{Path string \`json:"path"\`; URL string \`json:"url"\`; Kind string \`json:"kind"\`}`, con `Kind` = `image|video|audio|other`.
  - `func download.All(ctx, c *api.Client, urls []string, outDir, model, taskID string) ([]File, error)`:
    - Crea `outDir` con 0755.
    - El nombre es `<model con "/"→"-">_<taskID>_<n><ext>`. La extensión sale del path de la URL y, si no tiene, de `mime.ExtensionsByType(Content-Type)`; si tampoco, `.bin`.
    - Si el archivo existe, prueba `-1`, `-2`… antes de la extensión.
    - Escribe en `.part` y luego hace rename.
    - Solo acepta URLs `https`.
    - Si el GET no es 2xx, una vez hace POST `/api/v1/common/download-url {"url":u}` (`data` es un string) y reintenta el GET con esa URL.
    - Si todo falla, devuelve `*output.Error{Code:"io", Details:{"url":u}}`. La tarea ya está en el ledger y el resultado sigue disponible 14 días con `task get`.
- [ ] **Step 1: Write the failing tests**:
  - `TestURLsResultUrlsAndLastFrame`.
  - `TestURLsEmptyOnNoResult`.
  - `TestDownloadNamesAndKind` (Content-Type `video/mp4`, URL sin extensión → `kling-3.0-video_t1_0.mp4`, kind video).
  - `TestDownloadNeverOverwrites` (el archivo ya existe → se crea `_0-1.png` y el original queda intacto). [Review Focus 4]
  - `TestDownloadFallbackDownloadURL` (el primer GET da 403, se llama download-url y el segundo GET da 200).
  - `TestDownloadRejectsHTTP`.
  - `TestNoPartFileLeftOnError`.
- [ ] **Step 2: Run** `go test ./internal/result/ ./internal/download/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Para usar `httptest.NewTLSServer`, el `api.Client` de los tests usa `srv.Client()`.
- [ ] **Step 4: Run** lo mismo. Expected: PASS.
- [ ] **Step 5: Commit** `feat: result URL extraction and safe downloads`

---

### Task 11: `run` y `task wait` de punta a punta

**Files:**
- Create: `internal/cli/run.go`, `internal/cli/params.go`; modificar `internal/cli/task.go` (agregar `wait`)
- Test: `internal/cli/params_test.go`, `internal/cli/run_e2e_test.go` (con un servidor falso completo que combina API, upload y CDN)

**Interfaces:**
- Consumes: todo lo anterior.
- Produces:
  - `func cli.ParseParams(pairs []string, readFile func(string) ([]byte, error)) (map[string]any, error)`:
    - Separa `clave=valor` en el primer `=`.
    - Un valor `@ruta` se reemplaza por el contenido del archivo como string. `@@` escapa un `@` literal.
    - Si el valor empieza con `{` o `[` y es JSON válido, se decodifica.
    - `true`, `false` y los números JSON se decodifican.
    - Lo demás queda como string.
    - Si una clave se repite, sus valores se acumulan en un array.
    - Sin `=`, devuelve `*output.Error{Code:"usage"}`.
    - `// ponytail: literal-based typing; stage 2 coerces by schema type.`
  - `func cli.MergeInput(base, over map[string]any) map[string]any`: mapa nuevo donde `over` gana clave por clave.
  - `kie-cli run <model> [-p k=v]... [--input file.json|-] [--out DIR] [--no-wait] [--no-download] [--timeout DUR]`:
    1. Si el modelo empieza con `@`, devuelve un error `usage` con "presets arrive in stage 5".
    2. Llama a `catalog.Find`. Si no existe, devuelve un error `validation` con `"unknown model; run 'kie-cli models search'"`. Si la caché falla por red, sigue con un aviso.
    3. Llama a `schema.Fetch`. Si `!IsJob()`, devuelve un error `usage` con "synchronous models arrive in stage 4". Si el schema es null, avisa por stderr.
    4. Arma el input con `MergeInput(--input, ParseParams)`.
    5. `upload.ResolveLocal`.
    6. `task.Submit`.
    7. Con `--no-wait`, emite `{"ok":true,"taskId","model","state":"submitted"}`.
    8. Si no, `Waiter.Wait` con el timeout por defecto de Global Constraints.
    9. Si no hay `--no-download`, `download.All` a `--out`, o `config.OutDir`, o `./kie-output`.
    10. Añade al ledger una entrada con el estado final.
    11. Emite `{"ok":true,"taskId","model","state","files":[…],"result":<response>,"creditsConsumed","costTime","uploaded":[rutas]}`.
  - Si `Wait` devuelve timeout o task_failed, emite error y el JSON incluye `details.taskId`.
  - `task wait <id> [--out] [--timeout] [--no-download]` corre desde el paso 8 y saca el modelo del Record.
  - Con `--quiet`, el `Progress` del Waiter y todos los avisos van a `io.Discard`. Sin `--quiet`, van a `Deps.Stderr`.
- [ ] **Step 1: Write the failing tests**:
  - `TestParseParams` (con tabla: `duration=5`→5.0, `sound=true`→true, `prompt=hola mundo`→"hola mundo", `x={"a":1}`→map, repetición de `image_urls`→[]any{"a","b"}, `prompt=@p.txt`→contenido, `k=@@x`→"@x", `bad`→usage).
  - `TestRunE2EImage` (servidor falso: catálogo y schema tomados de testdata/live para un modelo de imagen job, createTask → id, recordInfo waiting→success con `resultUrls` del CDN falso, CDN sirve PNG. Afirmar exit 0, un archivo en `--out` con contenido idéntico y stdout con un solo JSON con `files[0].kind=="image"` y `taskId`).
  - `TestRunE2EUploadsLocalImage` (`-p image_urls=./in.png` → el servidor falso recibe upload y createTask con la URL de upload).
  - `TestRunNoWait` (sin llamadas a recordInfo).
  - `TestRunUnknownModel` (exit 2).
  - `TestRunCredits402` (exit 4, sin entrada unknown). [Review Focus 2]
  - `TestTaskWaitResumes` (submit con `--no-wait` y después `task wait <id>` descarga).
  - `TestStdoutIsSingleJSONOnEveryPath` (para los casos anteriores, `json.Decoder` lee exactamente 1 valor y luego EOF).
- [ ] **Step 2: Run** `go test ./internal/cli/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `make test`. Expected: PASS, con cobertura de cada paquete en `internal/` ≥80% (revisar la salida de `-cover`).
- [ ] **Step 5: Verify manual sin key real**: `make build && KIE_CLI_HOME=$(mktemp -d) ./bin/kie-cli credits; echo $?`. Expected: JSON con `"code":"auth"` y salida `3`.
- [ ] **Step 6: Commit** `feat: run and task wait end-to-end`

---

## Cierre de etapa
- [ ] `make test` en verde; `golangci-lint run` sin errores.
- [ ] Revisión del dueño: demo con `--help`, `models list` con fixtures y `run` contra el servidor falso. Listar los flags añadidos que no estaban en el spec (ninguno previsto).
- [ ] **No** ejecutar `run` contra la API real en esta etapa: los smoke con créditos son de la etapa 7.
