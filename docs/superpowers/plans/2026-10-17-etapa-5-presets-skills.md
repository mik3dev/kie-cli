# kie-cli Etapa 5 (Presets + skills) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tres piezas:
- Presets YAML (de fábrica embebidos + del usuario) usables como `kie-cli run @nombre`, más `kie-cli preset list|show`.
- La skill `kie-generate` con sus references, embebida en el binario e instalable con `kie-cli skills install [--target DIR]`.
- Un test que falla si la skill menciona un comando o flag que no existe.

**Architecture:**
- `go:embed` no acepta `..`. Por eso un archivo en la raíz del módulo (`assets.go`, `package kiecli`) embebe `presets/default.yaml` y `skills/`, y exporta un `embed.FS`.
- `internal/preset` carga y mezcla los presets.
- `internal/skills` copia el árbol embebido a disco.
- La CLI agrega `preset` y `skills`, y en `run` resuelve `@nombre` antes de todo lo demás.

**Tech Stack:** Go 1.27. Dependencia nueva: `gopkg.in/yaml.v3` (la sugiere el spec §7).

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`:
- §4 (interfaz `run <model|@preset>`, `preset list | show <name>`, `skills install [--target DIR]`)
- §5 Presets
- §6 Skills
- §7 (`internal/preset`, `internal/skills`, `presets/default.yaml`, `skills/kie-generate/...`)
- fila "5" de §8

Fuentes de contenido para la skill: `docs/research/{image,video,audio,endpoints-and-prior-art,kie-official-skill-reference}.md`.

## Global Constraints

- Siguen vigentes las de las etapas 1–4:
  - stdout lleva **exactamente un** objeto JSON por comando y los avisos van por stderr.
  - Los códigos de salida no cambian.
  - Las funciones de datos no mutan sus argumentos.
  - Cobertura ≥80% por paquete y `golangci-lint run ./...` limpio.
  - Ningún test usa la API real ni lee `.env`.
- Formato de presets (el mismo en `presets/default.yaml` y en `~/.kie-cli/presets.yaml`, ruta `config.Paths.Presets`): un documento YAML con la clave `presets:` y una lista de `{name, description, model, params}`.
  - Claves desconocidas son error (`yaml.Decoder.KnownFields(true)`).
  - `name` debe cumplir `^[a-z0-9][a-z0-9-]*$`.
  - `model` es obligatorio.
  - `params` es opcional.
  - Un nombre repetido dentro del mismo archivo es error.
- Los `params` se normalizan con un round-trip JSON y `Decoder.UseNumber()`, así los números son `json.Number`, igual que los valores tipados de `-p`.
- Precedencia de input: `preset.params` < `--input` < `-p`. Es una mezcla superficial con `MergeInput`, y después pasa por el `Coerce` y `Validate` de siempre.
- El preset del usuario reemplaza **entero** al de fábrica con el mismo nombre; no se mezclan sus params.
- Que falte el archivo del usuario no es error. Si existe pero es inválido, es `usage` (exit 2) y el mensaje nombra la ruta del archivo y la causa. Esto aplica a `run @x`, `preset list` y `preset show`.
- Un preset desconocido da `usage`: `unknown preset @<name>; see kie-cli preset list`.
- `skills install`:
  - Destino por defecto: `$HOME/.claude/skills` (vía `deps.Env("HOME")`). Sin HOME y sin `--target` da `usage`.
  - Copia `skills/kie-generate/**` a `<target>/kie-generate/**`, con directorios `0755` y archivos `0644`.
  - Sobrescribe los archivos existentes y **no borra** nada más.
  - Un error de escritura da `io`.
- El contenido de la skill va en inglés (lo leen agentes), en formato Agent Skills: frontmatter YAML con `name: kie-generate` y `description` (qué hace y cuándo usarla).

## Presets de fábrica (decisión del builder; el dueño los revisa después, spec §5)

Precios tomados de `pricingDesc` en `testdata/live/models.json`:

| name | model | params | costo aprox. |
|---|---|---|---|
| `imagen-rapida` | `google/imagen4-fast` | `aspect_ratio: "1:1"` | 4 cr/imagen |
| `imagen-calidad` | `google/imagen4-ultra` | `aspect_ratio: "1:1"` | 12 cr/imagen |
| `imagen-edicion` | `google/nano-banana-edit` | `output_format: png` | 4 cr (requiere `image_urls`) |
| `video-calidad` | `kling/v2-5-turbo-text-to-video-pro` | `duration: "5"`, `aspect_ratio: "16:9"` | 42 cr/5s |
| `video-imagen` | `kling/v2-5-turbo-image-to-video-pro` | `duration: "5"` | 42 cr/5s (requiere `image_url`) |
| `video-inicio-fin` | `veo-3-1` | `generation_type: FIRST_AND_LAST_FRAMES_2_VIDEO`, `model: veo3_fast` | ver `models price` (requiere `image_urls` [inicio, fin]) |
| `video-barato` | `bytedance/v1-lite-text-to-video` | `resolution: 480p`, `duration: "5"` | ~10 cr/5s |
| `musica` | `ai-music-api/generate` | `custom_mode: false`, `instrumental: false`, `model: V5` | 12 cr (2 tracks) |
| `instrumental` | `ai-music-api/generate` | `custom_mode: false`, `instrumental: true`, `model: V5` | 12 cr (2 tracks) |
| `voz` | `elevenlabs/text-to-speech-multilingual-v2` | `voice: EkK5I93UQWFDigLMpZcX` | 12 cr/1000 caracteres |

- Cada `description` dice cuándo usar el preset y qué campos faltan (por ejemplo `prompt`, `image_urls`). Van en inglés.
- `duration: "5"` va entre comillas porque el schema la declara como string.

## Review Focus

1. **Override del usuario:** `~/.kie-cli/presets.yaml` redefine `imagen-rapida` y agrega `mio`. `preset list` muestra ambos con `source: "user"`; el resto sale con `source: "factory"`, en orden por nombre. `run @imagen-rapida --dry-run` usa el modelo del usuario. → tests en Task 1 y Task 2.
2. **Archivo del usuario roto** (YAML inválido, clave desconocida `modle:`, falta `model`, nombre `Foo Bar`): exit 2, el mensaje incluye la ruta y nunca hay panic. Que el archivo falte no es error. → test en Task 1 (paquete) y Task 2 (CLI, exit 2).
3. **Precedencia:** el preset pone `aspect_ratio: "1:1"`, `--input` pone `"16:9"` y `-p aspect_ratio=9:16`. En `--dry-run`, `body.input.aspect_ratio == "9:16"`. Sin `-p`, gana `--input`. → test en Task 2.
4. **`run @nope` y `run @`:** exit 2 con `unknown preset`, sin llamar a la API (`createHits == 0`). → test en Task 2.
5. **`skills install` sobre un destino con contenido:** el `SKILL.md` viejo se reemplaza y la carpeta `otra-skill/` hermana queda intacta. Un destino no escribible (un archivo regular en vez de directorio) da exit 1 con code `io`. → test en Task 3.

---

### Task 1: Paquete `preset` y presets de fábrica embebidos

**Files:**
- Create: `assets.go` (raíz del módulo, `package kiecli`)
- Create: `presets/default.yaml`
- Create: `internal/preset/preset.go`, `internal/preset/preset_test.go`
- Modify: `go.mod`, `go.sum` (`gopkg.in/yaml.v3`)

**Interfaces:**
- Produces:
  - `kiecli.Assets embed.FS`, con `//go:embed presets/default.yaml skills`. **Nota:** `skills/` no existe hasta Task 3, así que en esta tarea crear `skills/kie-generate/SKILL.md` con solo el frontmatter (`name: kie-generate`, `description: TODO`); Task 3 lo completa.
  - `type preset.Preset struct { Name string \`json:"name"\`; Description string \`json:"description"\`; Model string \`json:"model"\`; Params map[string]any \`json:"params,omitempty"\`; Source string \`json:"source"\` }`. `Source` es `"factory"` o `"user"`.
  - `func preset.Parse(data []byte, source string) ([]Preset, error)`: valida según Global Constraints.
  - `func preset.Load(factory []byte, userPath string) ([]Preset, error)`: hace la mezcla por nombre y devuelve la lista ordenada por `Name`. Si el archivo del usuario no existe, no hay error. Si es inválido, el error incluye `userPath`.
  - `func preset.Find(list []Preset, name string) (Preset, bool)`

- [ ] **Step 1: Write the failing tests**

```go
func TestParseValid(t *testing.T)
// "presets:\n- name: a\n  description: d\n  model: m/x\n  params: {n: 5, s: \"5\", b: true}\n"
// -> 1 preset; Params["n"] == json.Number("5"), Params["s"] == "5", Params["b"] == true; Source == "factory"
func TestParseErrors(t *testing.T) // Review Focus 2, table:
// invalid YAML; unknown key `modle:`; missing model; name "Foo Bar"; duplicate name -> error each
func TestLoadUserOverridesAndAdds(t *testing.T) // Review Focus 1
// factory {a(model m1), b}; user file {a(model m2), c} -> [a(m2,user), b(factory), c(user)]
func TestLoadMissingUserFile(t *testing.T)   // nonexistent path -> factory only, nil error
func TestLoadBrokenUserFileNamesPath(t *testing.T) // error message contains userPath
func TestFactoryPresetsMatchLiveSchemas(t *testing.T)
// Parse(kiecli.Assets presets/default.yaml): the 10 names from the plan table exist.
// For each: model is in testdata/live/models.json (.models[].model).
// Load testdata/live/schemas/<model with "/"→"__">.json into schema.Doc, then doc.InputSchema().
// schema.Validate(sch, schema.Coerce(sch, p.Params)) has no problems apart from
// those whose Message ends in "missing required field".
```

- [ ] **Step 2: Run** `go test ./internal/preset/`. Expected: FAIL (package without implementation).

- [ ] **Step 3: Implement.** Use `yaml.NewDecoder` + `KnownFields(true)`. Normalize params with `json.Marshal` → `json.Decoder.UseNumber()`. Load `default.yaml` with the plan table's values and descriptions.

- [ ] **Step 4: Run** `go test -race -cover ./internal/preset/ && golangci-lint run ./...`. Expected: PASS, ≥80%, 0 issues.

- [ ] **Step 5: Commit** `feat: add preset loading with embedded factory presets`

---

### Task 2: `run @preset` y `preset list|show`

**Files:**
- Create: `internal/cli/preset.go`, `internal/cli/preset_test.go`
- Modify: `internal/cli/run.go`:
  - `runModel`: replace the `"presets arrive in stage 5"` branch.
  - `buildInput`: new `presetParams map[string]any` parameter; pass it through from `prepareInput`.
- Modify: `internal/cli/root.go` (register `newPresetCmd`)
- Modify: `internal/cli/run_e2e_test.go`, only `TestRunPresetAndSyncModelsAreUsageErrors`: its `@preset` case now expects `unknown preset`.

**Interfaces:**
- Consumes: `kiecli.Assets`, `preset.Load`, `preset.Find` (Task 1); `config.ResolvePaths(deps.Env).Presets`.
- Produces:
  - `kie-cli preset list` → `{"presets":[{name,description,model,source}]}`, without `params`.
  - `kie-cli preset show <name>` → `{"preset":{name,description,model,params,source}}`. Accepts `name` with or without `@`.
  - `run @name`: resolve the model from the preset, then follow the normal flow with that model. Success outputs (and dry-run) add `"preset": "<name>"`.

- [ ] **Step 1: Write the failing tests** (`preset_test.go`, using `newFake` and `KIE_CLI_HOME` pointed at a tmp dir):

```go
func TestPresetList(t *testing.T)          // no user file -> 10 presets, all source "factory", sorted by name, no "params" key
func TestPresetListUserOverride(t *testing.T) // Review Focus 1: user file with imagen-rapida(model google/imagen4) + mio -> source "user" on both
func TestPresetListBrokenUserFile(t *testing.T) // Review Focus 2: `presets: [` -> exit 2, message contains presets.yaml path
func TestPresetShow(t *testing.T)          // show imagen-rapida and show @imagen-rapida -> same output; params.aspect_ratio == "1:1"
func TestPresetShowUnknown(t *testing.T)   // show nope -> exit 2, "unknown preset @nope"
func TestRunPresetDryRun(t *testing.T)
// run @imagen-rapida -p prompt=gato --dry-run -> exit 0, body.model == "google/imagen4-fast",
// body.input == {"prompt":"gato","aspect_ratio":"1:1"}, out["preset"] == "imagen-rapida"
func TestRunPresetPrecedence(t *testing.T) // Review Focus 3
// --input file {"aspect_ratio":"16:9"} -> "16:9"; adding -p aspect_ratio=9:16 -> "9:16"
func TestRunPresetUnknown(t *testing.T)    // Review Focus 4: run @nope and run @ -> exit 2, createHits == 0
func TestRunPresetUserOverride(t *testing.T) // Review Focus 1: user imagen-rapida with model google/imagen4 -> dry-run body.model == "google/imagen4"
```

- [ ] **Step 2: Run** `go test ./internal/cli/ -run 'Preset'`. Expected: FAIL.

- [ ] **Step 3: Implement.** `runModel` loads presets only when the argument starts with `@`, so runs without a preset never read the YAML.

- [ ] **Step 4: Run** `make test && golangci-lint run ./...`. Expected: green, ≥80% per package, 0 issues.

- [ ] **Step 5: Commit** `feat: run presets and add preset list/show commands`

---

### Task 3: Skill `kie-generate`, `skills install` y test de consistencia

**Files:**
- Modify/Create: `skills/kie-generate/SKILL.md`, `skills/kie-generate/references/{image,video,audio}.md`
- Create: `internal/skills/skills.go`, `internal/skills/skills_test.go`
- Create: `internal/cli/skills.go`, `internal/cli/skills_test.go`, `internal/cli/skills_consistency_test.go`
- Modify: `internal/cli/root.go` (register `newSkillsCmd`)

**Interfaces:**
- Consumes: `kiecli.Assets` (Task 1); `NewRoot` (to walk the cobra tree).
- Produces:
  - `func skills.Install(src fs.FS, root, target string) ([]string, error)`. It copies `src/root/**` to `target/<base(root)>/**` and returns the written paths, sorted.
  - `kie-cli skills install [--target DIR]` → `{"target": DIR, "files": [...]}`.
  - `func checkSkillText(root *cobra.Command, text string) []string`, in the test file. For each line containing `kie-cli `, tokenize what follows up to the end of the line or the closing backtick:
    - Walk subcommands while each token names a child command.
    - The first token after `kie-cli` must be a command or a flag.
    - Every `--x` / `--x=v` / `-x` token must resolve with `cmd.Flag(name)`, or `cmd.Flags().ShorthandLookup` for `-x`.
    - Tokens like `<model>`, `@preset`, `key=value` and the rest are ignored.
    - Returns one problem per command or flag that does not exist.

**Skill content** (in English; guidance only, it does not replace the schema):
- `SKILL.md` (≤ ~200 lines):
  - Frontmatter `name: kie-generate`, plus a `description` that triggers on "generate/edit image, video, music, speech, sound with kie.ai".
  - Flow (spec §6): `models search`/`models list` → `models schema <model>` → `run --dry-run` if expensive → `run` → use `files[].path`.
  - Presets: `preset list`, `run @name`.
  - Critical rules: never invent models or fields (only what `models schema` shows); check `credits` and `models price` before video; `--no-wait` + `task wait` to parallelize; chain using `ids` from the output; `--out` for the destination.
  - Contract: stdout is one JSON object; `error.code` values and their exit codes (table: 0 ok, 2 `usage`/`validation`, 3 `auth`, 4 `credits`, 5 `task_failed`, 6 `timeout`, 7 `rate_limit`, 8 `submit_uncertain`, 1 `api`/`io`), and what the agent does with each one (for example, with 8 it runs `task list`/`task get` and does not resubmit).
- `references/image.md`: t2i vs edit (`image_urls`, `image_url`); `aspect_ratio` required in imagen4; upscale/remove-background. Source: `docs/research/image.md`.
- `references/video.md`: t2v / i2v / first-last (`image_urls` with `generation_type` in veo-3-1, `tail_image_url` in kling); extend with `task_id`; `last_frame_url`; durations as strings. Source: `docs/research/video.md`, `endpoints-and-prior-art.md`.
- `references/audio.md`: Suno (`custom_mode`, `instrumental`, `model`; 2 tracks; `ids.audio_ids` → `audio_id` for extend/cover); TTS (`voice` enum); sync models (`sync: true`); recovery. Source: `docs/research/audio.md`.
- Every command example uses only existing flags; the consistency test enforces this.

- [ ] **Step 1: Write the failing tests**

```go
// internal/skills/skills_test.go
func TestInstallCopiesTree(t *testing.T)   // fstest.MapFS{"s/k/SKILL.md","s/k/references/a.md"}, root "s/k" -> target/k/SKILL.md, target/k/references/a.md; returned list sorted
func TestInstallOverwritesKeepsSiblings(t *testing.T) // Review Focus 5: old target/k/SKILL.md replaced; target/otra-skill/x.md untouched
func TestInstallTargetIsFile(t *testing.T) // target is a regular file -> error

// internal/cli/skills_test.go
func TestSkillsInstallTarget(t *testing.T)  // --target tmp -> exit 0, files include tmp/kie-generate/SKILL.md and references/{image,video,audio}.md
func TestSkillsInstallDefaultHome(t *testing.T) // HOME=tmp, no --target -> tmp/.claude/skills/kie-generate/SKILL.md
func TestSkillsInstallNoHome(t *testing.T)  // HOME="" and no --target -> exit 2
func TestSkillsInstallIOError(t *testing.T) // Review Focus 5: --target <regular file> -> exit 1, error.code "io"

// internal/cli/skills_consistency_test.go
func TestCheckSkillTextCatchesBadRefs(t *testing.T)
// "`kie-cli run --bogus`" -> 1 problem; "`kie-cli nope`" -> 1 problem;
// "`kie-cli models schema <model> --refresh`" and "kie-cli run @musica -p prompt=x --no-wait" -> 0 (adjust flags to ones that exist)
func TestSkillsMentionOnlyRealCommands(t *testing.T)
// fs.WalkDir(kiecli.Assets, "skills") every .md -> checkSkillText(NewRoot(...), text) == nil
func TestSkillDocumentsExitCodes(t *testing.T)
// SKILL.md contains each of: usage validation auth credits task_failed timeout rate_limit submit_uncertain api io
func TestSkillFrontmatter(t *testing.T)
// SKILL.md starts with "---\n", has "name: kie-generate" and a non-empty "description:" before the closing "---"
```

- [ ] **Step 2: Run** `go test ./internal/skills/ ./internal/cli/ -run 'Install|Skill'`. Expected: FAIL.

- [ ] **Step 3: Implement** `skills.Install` (`fs.WalkDir` + `os.MkdirAll` + `os.WriteFile`), the `skills install` command, and write the skill content. Before writing the examples, run `go run ./cmd/kie-cli <cmd> --help` to confirm the flags.

- [ ] **Step 4: Run** `make test && golangci-lint run ./...`. Expected: green, ≥80% per package, 0 issues.

- [ ] **Step 5: Commit** `feat: add kie-generate skill with skills install and consistency test`
