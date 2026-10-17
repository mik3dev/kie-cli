# kie-cli Etapa 2 (Validación) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `run` convierte los `-p` al tipo que pide el schema, valida el input completo antes de gastar créditos (`--no-validate` lo baja a avisos) y ofrece `--dry-run` con el body final y el precio.

**Architecture:** Hay dos funciones puras nuevas en `internal/schema`: `Coerce` (tipos según el schema) y `Validate` (JSON Schema 2020-12 vía `santhosh-tekuri/jsonschema/v6`, más un chequeo de claves desconocidas). La detección de campos de archivo suma `format: uri` del schema. `run` encadena: input crudo → `Coerce` → copia con placeholders para archivos locales → `Validate` → `--dry-run` sale ahí; si no, sube los archivos y envía.

**Tech Stack:** Go 1.27, cobra (ya está), `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2` (dependencia nueva, la sugiere el spec en §7).

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`: §4 "Reglas de `run`" (conversión de tipos, subida automática, validación previa, `--no-validate`, `--dry-run`) y la fila "2" de §8. Los diferidos de la etapa 1 están en `docs/superpowers/plans/2026-10-17-etapa-1-pendientes.md`.

## Global Constraints

- Las de la etapa 1 siguen vigentes, en particular:
  - stdout lleva **exactamente un** objeto JSON por comando y los avisos van por stderr (`--quiet` los silencia).
  - Códigos de salida: 2 uso/validación · 3 auth · 4 créditos · 5 tarea falló · 6 timeout · 7 rate limit · 8 `submit_uncertain` · 1 otro.
  - Las funciones que transforman `input` devuelven un mapa nuevo y no mutan el recibido.
  - Cobertura ≥80% por paquete (`go test -race -cover ./...`).
  - `golangci-lint run ./...` limpio.
  - Ningún test usa la API real ni lee `.env`.
- Error de validación: `output.Error{Code: "validation"}` (salida 2) con `details.problems = [{field, message, got?}]`.
- `field` usa notación legible relativa a `input`, por ejemplo `image_urls[0]` o `elements[1].name`. `got` se omite si falta el campo, y los strings largos se recortan a 100 runas.
- Validación **antes** de subir archivos o de llamar a `createTask`: un input inválido no gasta subidas ni créditos.
- Si el schema es `null` o no compila, se envía sin validar con un aviso por stderr (spec §4). Nunca bloquea al usuario.
- `--no-validate` convierte cada problema en un aviso por stderr (`warning: validation: <field>: <message>`). La coerción se aplica igual.
- Fixtures: los schemas reales de `testdata/live/schemas/` (la barra del id de modelo se escribe `__` en el nombre de archivo).

## Review Focus

1. **Un string que parece JSON en un campo string** (`-p prompt="[intro] hola"` o `-p prompt='[1,2]'`) se envía como string literal, sin parsear. → test en Task 1.
2. **Ruta local en un campo con `pattern: ^(https?|oss)://`** (`image_urls` de `kling-3.0-omni/image-to-video`): la validación pasa porque valida contra placeholders, y `--dry-run` lista la subida pendiente sin subir nada. → test en Task 4.
3. **Tipos cruzados**: `-p duration=5` en un campo `integer` envía `5`, y `--input '{"duration":5}'` en un campo `string` envía `"5"`. → test en Task 1.
4. **Schema con `required` erróneo (caso Suno)**: con `--no-validate` se envía igual, los avisos van a stderr y stdout sigue teniendo un solo JSON. → test en Task 4.
5. **Los 226 schemas reales compilan** con el validador. Un schema de kie con una rareza (`format: double`, `nullable`, claves con espacios) no deja ningún modelo inutilizable. → test en Task 2.

---

### Task 1: Coerción por schema

**Files:**
- Create: `internal/schema/coerce.go`, `internal/schema/coerce_test.go`
- Modify: `internal/cli/params.go` (sin tipado literal), `internal/cli/params_test.go`

**Interfaces:**
- Consumes: `schema.Doc.InputSchema() (map[string]any, error)` (ya resuelto, sin `$ref`).
- Produces:
  - `func schema.Coerce(sch map[string]any, input map[string]any) map[string]any`. Nunca falla: si un valor no se puede convertir, queda tal cual y lo reporta `Validate`.
  - `func schema.PropSchema(sch map[string]any, key string) (map[string]any, bool)` busca en `properties` y, en orden, en las ramas de `oneOf`/`anyOf`/`allOf`. Devuelve la primera coincidencia. La usan las Tasks 2 y 3.
  - `cli.ParseParams` cambia de contrato: los valores son siempre `string` (o `[]any` de strings si la clave se repite). `@file` y `@@` no cambian. **Ya no parsea JSON ni literales.**

- [ ] **Step 1: Write the failing tests** (`coerce_test.go`, tabla con un schema sintético más `bytedance__seedance-2-5`):

```go
func TestCoerce(t *testing.T) {
	sch := map[string]any{"type": "object", "properties": map[string]any{
		"n":     map[string]any{"type": "integer"},
		"f":     map[string]any{"type": "number"},
		"b":     map[string]any{"type": "boolean"},
		"s":     map[string]any{"type": "string"},
		"arr":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"urls":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"obj":   map[string]any{"type": "object", "properties": map[string]any{"k": map[string]any{"type": "integer"}}},
		"multi": map[string]any{"type": []any{"string", "integer"}},
	}}
	cases := []struct{ key string; in, want any }{
		{"n", "5", json.Number("5")},
		{"n", " 5 ", json.Number("5")},
		{"n", "abc", "abc"},                      // invalid: left for Validate
		{"f", "1.5", json.Number("1.5")},
		{"b", "TRUE", true},
		{"s", 5.0, "5"},                          // from --input JSON
		{"s", true, "true"},
		{"s", "[intro] hola", "[intro] hola"},   // Review Focus 1
		{"s", "[1,2]", "[1,2]"},                 // Review Focus 1
		{"arr", "3", []any{json.Number("3")}},    // scalar wraps
		{"arr", "[1,2]", []any{1.0, 2.0}},        // JSON array literal
		{"arr", []any{"1", "2"}, []any{json.Number("1"), json.Number("2")}}, // repeated -p
		{"urls", "a.png", []any{"a.png"}},
		{"obj", `{"k":"7"}`, map[string]any{"k": json.Number("7")}},
		{"multi", "5", "5"},                      // already matches string
		{"unknown", "true", true},                // not in schema: literal guess
		{"unknown", "[intro]", "[intro]"},
	}
	// for each: got := Coerce(sch, map[string]any{c.key: c.in})[c.key]; reflect.DeepEqual(got, c.want)
}
func TestCoerceNilSchemaGuessesLiterals(t *testing.T)   // nil sch: "true"->true, "5"->5.0, `{"a":1}`->map, "hola"->"hola"
func TestCoerceOneOfBranchProps(t *testing.T)           // kling-3.0-omni__image-to-video: "duration" se busca dentro de oneOf; "image_urls"="a.png" -> []any{"a.png"}
func TestCoerceDoesNotMutate(t *testing.T)              // input y sch iguales (reflect.DeepEqual con copia previa) tras Coerce
func TestCoerceSeedanceDuration(t *testing.T)           // live seedance-2-5: {"duration":"5"} -> json.Number("5")
```

En `params_test.go`, actualizar `TestParseParams`: `-p n=5` → `"5"`, `-p b=true` → `"true"`, `-p x={"a":1}` → `"{\"a\":1}"`, y la repetición `-p u=a -p u=b` → `[]any{"a","b"}`.

- [ ] **Step 2: Run** `go test ./internal/schema/ ./internal/cli/ -run 'Coerce|ParseParams'`. Expected: FAIL (`Coerce` undefined; ParseParams devuelve los tipos viejos).

- [ ] **Step 3: Implement.**
  - Los tipos de una propiedad salen de `type` (string o array). Si no tiene `type`, se usa la unión de los `type` de sus ramas `oneOf`/`anyOf`.
  - Si el valor ya cumple alguno de esos tipos, no se toca.
  - Si no, se prueba en el orden `integer`, `number`, `boolean`, `array`, `object`, `string`.
  - Los números convertidos son `json.Number`: no pierden precisión y v6 los acepta.
  - `array`: el string que empieza con `[` y es JSON válido se usa como array. Cualquier otro escalar se envuelve en un array de un elemento. Los ítems se convierten con `items`.
  - `object`: el string que empieza con `{` y es JSON válido se parsea, y sus propiedades se convierten de forma recursiva.
  - Un número (float64/json.Number) o un bool en un campo `string` se pasa a string. Los float64 usan `strconv.FormatFloat(f, 'f', -1, 64)`.
  - Las claves que no están en el schema y el schema `nil` usan el tipado literal que tenía `parseValue` en la etapa 1: JSON si empieza con `{`/`[` y es válido, bool, número, y si no, string. Esa lógica se mueve a `schema`.

- [ ] **Step 4: Run** `go test -race -cover ./internal/schema/ ./internal/cli/`. Expected: PASS y cobertura ≥80%.

- [ ] **Step 5: Commit** `feat: coerce run params by schema type`

---

### Task 2: Validación JSON Schema

**Files:**
- Create: `internal/schema/validate.go`, `internal/schema/validate_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `schema.PropSchema` (Task 1) y `schema.Doc.InputSchema()`.
- Produces:
  - `type schema.Problem struct { Field string `json:"field"`; Message string `json:"message"`; Got any `json:"got,omitempty"` }`
  - `func schema.Validate(sch map[string]any, input map[string]any) ([]Problem, error)`. El `error` aparece solo si el schema no compila. `[]Problem` va ordenado por `Field` y después por `Message`, y es `nil` si el input es válido.

- [ ] **Step 1: Write the failing tests** (sobre schemas reales):

```go
func TestValidateAllLiveSchemasCompile(t *testing.T)
// Review Focus 5: para cada testdata/live/schemas/*.json con Doc no nulo y IsJob(),
// InputSchema() y luego Validate(sch, map[string]any{}) no deben devolver error (los problems sí se permiten).

func TestValidateSeedance(t *testing.T) // bytedance__seedance-2-5
// {"prompt":"x","resolution":"4k"}       -> Problem{Field:"resolution", Got:"4k"}, Message contiene "720p"
// {"prompt":"x","duration":"cinco"}      -> Field "duration", Got "cinco", Message contiene "integer"
// {"prompt":"x","reference_image_urls": 31 URLs} -> Field "reference_image_urls", Message contiene "30"
// {"prompt":"x","resolution":"720p","duration":json.Number("5")} -> nil problems

func TestValidateRequiredAndUnknown(t *testing.T) // kling-3.0-omni__image-to-video
// {} -> problems incluyen Field "prompt" y Field "image_urls" (missing; Got ausente)
// {"prompt":"x","image_urls":["https://a/1.png"],"promt":"y"} -> Problem{Field:"promt"}
//   con Message que contiene "unknown field" y "kie-cli models schema"

func TestValidateOneOfNamesVariants(t *testing.T) // kling-3.0-omni__image-to-video
// image_urls con 3 URLs https -> algún Message contiene `"Single First Frame"` y otro `"First and Last Frames"`

func TestValidateNestedFieldPath(t *testing.T)
// schema sintético: items de objeto con "name" integer; {"elements":[{"name":1},{"name":"x"}]} -> Field "elements[1].name"

func TestValidateAdditionalPropertiesTrueSkipsUnknown(t *testing.T)
// schema con additionalProperties:true -> la clave extra no genera Problem

func TestValidateDoesNotMutate(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/schema/ -run Validate`. Expected: FAIL (undefined).

- [ ] **Step 3: Implement.**
  - Dependencia: `go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`.
  - Compilar con `NewCompiler()`, `DefaultDraft(jsonschema.Draft2020)`, `AddResource("input.json", sch)` y `Compile("input.json")`. No se activa la aserción de `format`.
  - Recorrer `*jsonschema.ValidationError` hasta sus hojas (`Causes`).
    - `Field` sale de `InstanceLocation`.
    - Un error `required` produce un Problem por cada clave faltante (`Field` = path padre + clave).
    - Si la hoja cuelga de `/oneOf/<i>/` o `/anyOf/<i>/` en la raíz del input, el mensaje lleva el prefijo `variant "<title>": `. Si la rama no tiene `title`, se usa `variant <i+1>`.
    - Un `oneOf` que coincide con más de una rama genera un Problem con `Field ""` y un mensaje que nombra las ramas.
    - `Message` es el texto de la librería (`err.ErrorKind.LocalizedString(printer)` con `golang.org/x/text/message` en inglés).
  - Claves desconocidas: se revisan aparte, solo en el nivel raíz del input.
    - Una clave es desconocida si `PropSchema` no la encuentra.
    - El chequeo se salta si el schema raíz declara `additionalProperties: true` o si no tiene propiedades en ningún lugar.
    - Si el schema ya declara `additionalProperties: false`, se deduplica con el error de la librería: queda un solo Problem por clave.
    - Mensaje: `unknown field; see 'kie-cli models schema <model>' for valid keys` (el texto `<model>` va literal, porque el comando no lo conoce aquí).

- [ ] **Step 4: Run** `go test -race -cover ./internal/schema/` y `golangci-lint run ./...`. Expected: PASS, cobertura ≥80% y 0 issues.

- [ ] **Step 5: Commit** `feat: validate run input against the model JSON schema`

---

### Task 3: Campos de archivo según el schema

**Files:**
- Create: `internal/schema/files.go`, `internal/schema/files_test.go`
- Modify: `internal/upload/upload.go` (`ResolveLocal` recibe el predicado), `internal/upload/upload_test.go`, `internal/cli/run.go` (solo la llamada, que se adapta)

**Interfaces:**
- Consumes: `schema.PropSchema` (Task 1).
- Produces:
  - `func schema.HasURIFormat(sch map[string]any, key string) bool` es true si el subárbol de la propiedad (incluidos `items`, `properties` anidadas y ramas `oneOf`/`anyOf`) contiene `"format": "uri"`.
  - `func upload.ResolveLocal(ctx context.Context, input map[string]any, isFileKey func(string) bool, up func(context.Context, string) (string, error)) (map[string]any, []string, error)`. Si `isFileKey` es `nil`, usa `upload.FileKey`.

- [ ] **Step 1: Write the failing tests.**
  - `TestHasURIFormat` (seedance-2-5): `reference_image_urls` → true (`items.format: uri`); `first_frame_url` → false (no declara format); `prompt` → false.
  - `TestHasURIFormatOneOf` (ai-music-api__separate-vocals): `audio_url` → true (solo existe en la rama 2).
  - En `upload_test.go`, `TestResolveLocalCustomPredicate`: el predicado `func(k string) bool { return k == "ref" }` sube el archivo local que está en `ref` y no toca el que está en `image_url`.

- [ ] **Step 2: Run** `go test ./internal/schema/ ./internal/upload/`. Expected: FAIL.

- [ ] **Step 3: Implement.** `run.go` pasa `nil` por ahora (el comportamiento no cambia). La Task 4 cablea el predicado `upload.FileKey(k) || schema.HasURIFormat(sch, k)`.

- [ ] **Step 4: Run** `go test -race -cover ./...`. Expected: PASS.

- [ ] **Step 5: Commit** `feat: detect file fields from schema uri format`

---

### Task 4: `run` con coerción, validación, `--no-validate` y `--dry-run`

**Files:**
- Modify: `internal/cli/run.go`
- Create: `internal/cli/run_validate_test.go`
- Modify: `internal/cli/run_e2e_test.go` (solo `fakeKie.serve`, que gana un case para `/price` y lee `testdata/live/price/<model con __>.json`. Si no existe, responde `{"code":404,...}`.)

**Interfaces:**
- Consumes:
  - `schema.Coerce`, `schema.Validate`, `schema.Problem`, `schema.HasURIFormat`
  - `upload.ResolveLocal(…, isFileKey, up)`
  - `catalog.Service.Price(ctx, model) (string, error)`
- Produces: los flags nuevos `--dry-run` y `--no-validate` en `run` (spec §4, no son flags extra).
  - Salida de `--dry-run`:

```json
{"ok":true,"dryRun":true,"model":"<m>",
 "body":{"model":"<m>","input":{…coerced, local paths → placeholder…}},
 "pendingUploads":[{"path":"<ruta local tal como vino>","placeholder":"https://pending-upload.invalid/<url.PathEscape(basename)>"}],
 "pricing":"<pricingDesc>",
 "problems":[…]}
```

  - `pendingUploads` siempre es un array (vacío si no hay).
  - `pricing` se omite si `Price` falla (con aviso por stderr).
  - `problems` solo aparece con `--no-validate` y si hay alguno.

- [ ] **Step 1: Write the failing tests** (`run_validate_test.go`, con `newFake`):

```go
func TestRunValidationErrorBlocksSubmit(t *testing.T)
// run bytedance/seedance-2-5 -p prompt=x -p resolution=4k
// -> code 2, error.code "validation", details.problems[0].field "resolution";
//    f.createHits == 0 y f.uploadHits == 0

func TestRunCoercesIntegerParam(t *testing.T)
// run bytedance/seedance-2-5 -p prompt=x -p duration=5 --no-wait
// -> createBody.input.duration == 5.0 (decodificado en el fake), no "5"

func TestRunNoValidateSendsWithWarnings(t *testing.T) // Review Focus 4
// run bytedance/seedance-2-5 -p prompt=x -p resolution=4k --no-validate --no-wait
// -> code 0, createHits == 1, stderr contiene "warning: validation: resolution:",
//    stdout es un único objeto JSON (json.Decoder: un valor y después EOF)

func TestRunDryRunWithLocalFileInPatternField(t *testing.T) // Review Focus 2
// archivo temporal a.png; run kling-3.0-omni/image-to-video -p prompt=x -p image_urls=<tmp>/a.png --dry-run
// -> code 0, dryRun true, body.input.image_urls[0] empieza con "https://pending-upload.invalid/",
//    pendingUploads[0].path == ruta tmp, createHits == 0, uploadHits == 0

func TestRunDryRunIncludesPricing(t *testing.T)
// fixture de price presente en testdata/live/price/ para el modelo usado -> "pricing" no vacío;
// modelo sin fixture -> sin "pricing" y stderr contiene "warning"

func TestRunNullSchemaSkipsValidation(t *testing.T)
// google/gemini-3-8-flash-tts (schema null) -p text=hola --dry-run -> code 0 y stderr con el aviso de schema nulo
```

  Antes de escribir los tests, el implementer elige con `ls testdata/live/price/` un modelo que tenga fixture de precio. Si `kling-3.0-omni/image-to-video` o `bytedance/seedance-2-5` faltan en `models.json`, usa otro modelo del mismo caso (schema con `pattern` en un campo URL / campo enum) y lo anota en su report.

- [ ] **Step 2: Run** `go test ./internal/cli/ -run 'RunValidation|RunCoerces|RunNoValidate|RunDryRun|RunNullSchema'`. Expected: FAIL (flags desconocidos).

- [ ] **Step 3: Implement** el flujo de `runModel` después de `schema.Fetch`:
  1. Calcular `sch := doc.InputSchema()`. Si da error, se avisa y se sigue con `sch = nil`.
  2. `input := schema.Coerce(sch, buildInput(...))`.
  3. Construir `isFile := func(k string) bool { return upload.FileKey(k) || schema.HasURIFormat(sch, k) }`.
  4. Primera pasada: `upload.ResolveLocal(ctx, input, isFile, placeholderFn)`. `placeholderFn` registra `{path, placeholder}` y devuelve el placeholder, sin red. El resultado es `checkInput`.
  5. Si `sch != nil`, correr `schema.Validate(sch, checkInput)`:
     - Si da error de compilación: aviso y se sigue.
     - Si hay problemas y no está `--no-validate`: devolver `output.Error{Code: validation, Message: "input does not match the schema of <model> (<n> problems); fix the fields, or use --no-validate if the schema is wrong", Details: {"problems": problems}}`.
     - Con `--no-validate`: un aviso por problema.
  6. Con `--dry-run`: pedir el precio y emitir el JSON de arriba. Sale sin `EnsureHome`, sin subir nada y sin crear la tarea.
  7. Si no es dry-run, sigue la subida real `upload.ResolveLocal(ctx, input, isFile, upload.File…)` y el resto del flujo de la etapa 1 sin cambios.

  Si `runModel` pasa de ~50 líneas, extraer `prepareInput(cmd, deps, c, model, doc) (input map[string]any, pending []pendingUpload, problems []schema.Problem, err error)`.

- [ ] **Step 4: Run** `make test` y `golangci-lint run ./...`. Expected: todo verde, cada paquete `internal` con ≥80% y 0 issues.

- [ ] **Step 5: Commit** `feat: validate input and add --dry-run and --no-validate to run`
