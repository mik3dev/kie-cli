# kie-cli Etapa 6 (Distribución) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dejar kie-cli instalable y mantenible:
- Binarios multiplataforma con goreleaser.
- CI en GitHub Actions (test + lint + chequeo de compilación para Windows).
- Workflow de release al pushear un tag `v*`.
- README con instalación y uso.
- Arreglos mínimos para que el binario de Windows funcione: home vía `USERPROFILE`, sin aviso falso de permisos, tests que compilan en Windows.

**Architecture:**
- Sin código de producto nuevo, salvo `config.UserHome` y el ajuste de permisos para Windows.
- `.goreleaser.yaml` y `.github/workflows/{ci,release}.yml` en la raíz.
- Dos tests guardianes:
  - En el paquete raíz `kiecli`: los workflows nunca usan la key ni el tag `live`.
  - En `internal/cli`: el README solo menciona comandos y flags que existen, reusando `checkSkillText` de la etapa 5.

**Tech Stack:** Go 1.27, goreleaser v2, GitHub Actions, golangci-lint v2.13.2 (la versión local).

**Spec:** `docs/superpowers/specs/2026-10-17-kie-cli-design.md`:
- §1: "Distribuir como binario fácil de instalar (macOS, Linux, Windows)".
- §7, línea de instalación:
  - Repo **privado**: la instalación es `make install` / `go install` (con `GOPRIVATE`) o descargar el binario de GitHub Releases privado con `gh release download`.
  - `goreleaser` arma los binarios multiplataforma.
  - El Homebrew tap queda para cuando el repo sea público.
- §8, fila 6.
- §9 Testing: los smoke tests reales llevan el tag `live` más `KIE_E2E=1`, solo a mano; el workflow de CI no usa el tag ni tiene la key como secret.

## Global Constraints

- Siguen vigentes las de las etapas 1–5 (un JSON por stdout, códigos de salida, inmutabilidad, cobertura ≥80% por paquete, lint limpio, ningún test contra la API real).
- **CI nunca** referencia `KIE_API_KEY`, `KIE_E2E`, `-tags=live` ni `secrets.*` distinto de `secrets.GITHUB_TOKEN`.
- Versión inyectada con `-ldflags "-X github.com/mik3dev/kie-cli/internal/cli.Version=<v>"`. El Makefile ya lo hace; goreleaser usa `{{.Version}}`.
- Matriz de goreleaser: `linux`, `darwin` y `windows` × `amd64` y `arm64`, con `CGO_ENABLED=0`.
  - Archivos: `tar.gz`, y `zip` en Windows.
  - Nombre del binario: `kie-cli`.
  - Incluir `checksums.txt`.
- El repo es privado y el release va a GitHub Releases del mismo repo. Sin Homebrew tap (spec §7).
- Las acciones de GitHub se fijan por versión mayor: `actions/checkout@v5`, `actions/setup-go@v6` (con `go-version-file: go.mod`), `golangci/golangci-lint-action@v8` (con `version: v2.13.2`), `goreleaser/goreleaser-action@v6` (con `version: "~> v2"`). Si alguna mayor no existe, usar la última existente y anotarlo en el report.
- No se crea ni se pushea ningún tag en esta etapa. Publicar el primer release (`v0.1.0`) lo decide el dueño después del merge.
- El README va en inglés (igual que la skill) y apunta a la skill para el uso por agentes.

## Review Focus

1. **Windows sin `HOME`, pero con `USERPROFILE`:** `config.ResolvePaths` y `skills install` resuelven el home igual, sin error. → test en Task 1.
2. **Windows: aviso falso de permisos.** `config.json` en Windows siempre reporta 0666, así que el aviso "permissions are 0666" no debe salir. → test en Task 1 (inyectando el GOOS).
3. **`GOOS=windows go vet ./...` falla hoy** (`syscall.Umask` en `internal/skills/skills_test.go`). Debe pasar, y CI lo corre. → Task 1, verificado en Task 2.
4. **El binario del release reporta la versión real**, no `dev`. CI compila con un `Version` de prueba y comprueba la salida de `version`. → step de CI en Task 2.
5. **El README menciona un flag que no existe** (o queda obsoleto tras un rename): el test falla. Además, un workflow que filtre la key o el tag `live` también hace fallar un test. → Task 2 (workflows) y Task 3 (README).

---

### Task 1: Compatibilidad con Windows

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `internal/cli/skills.go` (usar `config.UserHome`)
- Modify: `internal/skills/skills_test.go`. Mover el test que usa `syscall.Umask` a `internal/skills/skills_unix_test.go` con `//go:build unix`; el resto queda portable.

**Interfaces:**
- Produces:
  - `func config.UserHome(env func(string) string) string`: devuelve `HOME`, o `USERPROFILE` si `HOME` está vacío, o `""`. Lo usan `ResolvePaths` y `skills install`, y se mantienen los mensajes de error actuales, agregando "or USERPROFILE" donde nombran HOME.
  - `var goos = runtime.GOOS` (sin exportar, en config.go): `Load` omite el aviso de permisos si `goos == "windows"`.

- [ ] **Step 1: Write the failing tests**

```go
func TestUserHome(t *testing.T) // table: HOME=/h -> "/h"; HOME="" USERPROFILE=C:\u -> `C:\u`; both set -> HOME; none -> ""
func TestResolvePathsUserProfile(t *testing.T) // KIE_CLI_HOME="" HOME="" USERPROFILE=dir -> Home == filepath.Join(dir, ".kie-cli")
func TestLoadNoPermWarningOnWindows(t *testing.T) // config.json 0644; goos="windows" (restore after) -> warn buffer empty; goos="linux" -> warning present
// internal/cli/skills_test.go:
func TestSkillsInstallUserProfile(t *testing.T) // HOME="", USERPROFILE=tmp -> tmp/.claude/skills/kie-generate/SKILL.md
```

- [ ] **Step 2: Run** `go test ./internal/config/ ./internal/cli/ -run 'UserHome|UserProfile|PermWarning'`. Expected: FAIL.

- [ ] **Step 3: Implement.** Use `config.UserHome` in both places. Move the umask test to `skills_unix_test.go`.

- [ ] **Step 4: Run** `make test && golangci-lint run ./... && GOOS=windows go vet ./... && GOOS=windows go build ./...`. Expected: all green, ≥80% per package.

- [ ] **Step 5: Commit** `fix: resolve home and config permissions on Windows`

---

### Task 2: goreleaser, CI y release

**Files:**
- Create: `.goreleaser.yaml` (`version: 2`)
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`
- Modify: `Makefile` (targets `lint` → `golangci-lint run ./...`, and `snapshot` → `goreleaser release --snapshot --clean`)
- Modify: `.gitignore` (`dist/`)
- Create: `workflows_test.go` (root, `package kiecli`)

**Interfaces:**
- `.goreleaser.yaml`: one build `kie-cli` from `./cmd/kie-cli`, using the Global Constraints matrix and ldflags `-s -w -X github.com/mik3dev/kie-cli/internal/cli.Version={{.Version}}`. Archives as in Global Constraints, `checksum.name_template: checksums.txt`, `changelog` sorted ascending, excluding `^docs:` and `^test:`.
- `ci.yml` (`on: push` to `main`, `pull_request`), `permissions: contents: read`, one job on `ubuntu-latest` with these steps:
  1. checkout + setup-go.
  2. `go mod tidy && git diff --exit-code go.mod go.sum`.
  3. `go test -race -count=1 -cover ./...`.
  4. golangci-lint-action.
  5. `GOOS=windows go vet ./...` and `GOOS=darwin go build ./...`.
  6. Version check: `go build -ldflags "-X github.com/mik3dev/kie-cli/internal/cli.Version=ci-check" -o "$RUNNER_TEMP/kie-cli" ./cmd/kie-cli && "$RUNNER_TEMP/kie-cli" version | grep -q '"version":"ci-check"'` (Review Focus 4; first confirm the exact JSON format of `version` locally).
  7. `goreleaser check` (goreleaser-action with `args: check`).
- `release.yml` (`on: push: tags: ['v*']`), `permissions: contents: write`: checkout with `fetch-depth: 0`, setup-go, `go test ./...`, goreleaser-action `args: release --clean` with `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`.

- [ ] **Step 1: Write the failing test** (`workflows_test.go`):

```go
func TestWorkflowsNeverUseLiveKey(t *testing.T) // Review Focus 5
// for every .github/workflows/*.yml (require ≥2 files): content must not contain
// "KIE_API_KEY", "KIE_E2E", "tags=live", "tags live"; every "secrets." occurrence must be "secrets.GITHUB_TOKEN"
func TestGoreleaserInjectsVersion(t *testing.T)
// .goreleaser.yaml contains "github.com/mik3dev/kie-cli/internal/cli.Version={{.Version}}", "windows", "darwin", "linux", "arm64", "CGO_ENABLED=0"
```

- [ ] **Step 2: Run** `go test . -run 'Workflows|Goreleaser'`. Expected: FAIL (files missing).

- [ ] **Step 3: Create** the files according to Interfaces.

- [ ] **Step 4: Verify locally:**
  - `go test . && make test && golangci-lint run ./...`.
  - Run the version check from ci.yml step 6 (with `/tmp` → scratch dir).
  - `go run github.com/goreleaser/goreleaser/v2@latest check`, and `... release --snapshot --clean --skip=publish`. It must produce the 6 archives plus `checksums.txt` in `dist/`; `dist/` stays untracked.
  - If `go run` of goreleaser fails because of the toolchain, note it in the report; CI runs `check` anyway.

- [ ] **Step 5: Commit** `ci: add goreleaser config and CI/release workflows`

---

### Task 3: README

**Files:**
- Create: `README.md`
- Create: `internal/cli/readme_test.go`

**Interfaces:**
- Consumes: `checkSkillText(root *cobra.Command, text string) []string` (from `internal/cli/skills_consistency_test.go`, etapa 5) and `NewRoot`.

**README content** (in English; concise, ≤ ~200 lines; every command example uses real commands and flags; confirm each one with `go run ./cmd/kie-cli <cmd> --help`):
1. What it is (one paragraph): a CLI for agents over kie.ai, JSON on stdout, 200+ models via the live catalog.
2. Install (private repo):
   - (a) `gh release download --repo mik3dev/kie-cli --pattern '*<os>_<arch>*'`, then extract the archive and put it on PATH. Use the archive names that goreleaser actually produces (see `dist/` from Task 2, or the `name_template`).
   - (b) `GOPRIVATE=github.com/mik3dev/* go install github.com/mik3dev/kie-cli/cmd/kie-cli@latest`.
   - (c) `make install` from a clone.
3. Setup: `kie-cli auth login` (or `KIE_API_KEY`), `kie-cli auth status`, `kie-cli credits`. Config in `~/.kie-cli/` (`KIE_CLI_HOME` overrides it).
4. Quickstart: `models search`, `models schema`, `run ... --dry-run`, `run`, and the output with `files[].path`.
5. Presets: `preset list`, `run @imagen-rapida -p prompt=...`, the format of `~/.kie-cli/presets.yaml` with an example, the precedence rule (preset < `--input` < `-p`), and "quote string values in YAML (e.g. `duration: "5"`)".
6. Agent skill: `kie-cli skills install [--target DIR]`, and what the skill teaches.
7. Output contract and exit codes (the same table as SKILL.md).
8. Development:
   - `make test`, `make lint`, `make snapshot`.
   - Live smoke tests are run only by hand (`KIE_E2E=1`, tag `live`) and never in CI.
   - Releasing: push a tag `vX.Y.Z`.

- [ ] **Step 1: Write the failing test**

```go
func TestReadmeMentionsOnlyRealCommands(t *testing.T) // Review Focus 5
// os.ReadFile("../../README.md") -> checkSkillText(NewRoot(testDeps), text) == nil
func TestReadmeDocumentsExitCodes(t *testing.T)
// README contains each code: usage validation auth credits task_failed timeout rate_limit submit_uncertain api io, and "kie.ai/logs"
```

- [ ] **Step 2: Run** `go test ./internal/cli/ -run Readme`. Expected: FAIL (README missing).

- [ ] **Step 3: Write** `README.md`.

- [ ] **Step 4: Run** `make test && golangci-lint run ./...`. Expected: green.

- [ ] **Step 5: Commit** `docs: add README with install, usage and development guide`
