# Etapa 1 — rulings y pendientes diferidos

Extraído del ledger SDD al cerrar etapa 1. Insumo para planes de etapas 2–7.

## Rulings
- Ruling 1: If Task 4 cannot run (key/API failure), stop and ask owner (plan says so) — fixtures must not be invented — cost if wrong: delay only.
- Ruling 2: download GETs of result URLs carry NO Authorization header (key must never go to non-kie hosts); only the download-url fallback POST to api.kie.ai is authenticated — security, per spec anti-goal "key never leaks" — cost if wrong: a CDN that needs auth fails, but fallback covers it.
- Ruling 3: subagent commits carry their own model's Co-Authored-By trailer (accurate attribution) — cost if wrong: cosmetic
- Ruling 4: createTask 2xx with invalid/missing envelope (api.Error KieCode=200 "invalid response envelope"/"unexpected data shape") is treated as submit_uncertain in Task 8, like 5xx — request reached server, outcome unknown — cost if wrong: agent told to check logs unnecessarily (cheap)
- Ruling 5: models list/search hide models whose taskType contains "Chat" (spec §2: chat out of scope; 40 chat models in live catalog) — carry into Task 5 — cost if wrong: chat models invisible, intended
- Ruling 6: InputSchema uses whole body schema on non-createTask paths (chat/omni bodies where 'input' isn't the wrapper) — accepted deviation; job models unchanged — cost if wrong: non-job summaries differ in shape (stage 4 revisits)
- Ruling 7: plan-mandated Variants [][]Field loses branch titles/input description/nested object keys — spec intent is agent can build valid input from summary — change to Variants []Variant{Title,Description string; Fields []Field}, add Summary.Notes (input-level description, 600 runes), Field.ItemFields []Field (one level, arrays of objects) — cost if wrong: slightly bigger summary output
- Ruling 8: accept Task 11 signal.NotifyContext in Execute (SIGINT/SIGTERM → ctx cancel → JSON with taskId) — implements Review Focus 3 — cost if wrong: none expected

## Diferidos / notas
- Task 1: minor (deferred): Fail(w,nil) panics — add nil guard
- Task 1: minor (deferred): --help/no-args print help to stderr, no stdout JSON (exit 0) — decide in final review
- Task 1: minor (deferred): no Execute test for plain RunE error → exit 1 / typed output.Error from RunE
- Task 1: minor (deferred): Success key order alphabetical (map)
- Task 1: minor (deferred): run() sets package-global cobra.EnableTraverseRunHooks each call — move to init/NewRoot
- Task 2: minor (deferred): auth login fails on corrupt config.json (blocks recovery)
- Task 2: minor (deferred): EnsureHome doesn't tighten existing looser home dir
- Task 2: minor (deferred): go.mod x/term marked indirect — go mod tidy
- Task 2: minor (deferred): KIE_API_KEY env not TrimSpace'd (trailing newline → bad header)
- Task 2: minor (deferred): TTY branch of readKey untested
- Task 3: note: api.Request.Body JSON-only; Task 9 must add multipart support (carry into Task 9 dispatch)
- Task 3: minor (deferred): no TDD RED evidence (process gap)
- Task 3: minor (deferred): TestAsOutputMapping lacks ExitCode checks 401→3, 429→7; dead `var _ = output.CodeAPI`
- Task 3: minor (deferred): client_test.go:445 ignores net.Listen error
- Task 3: minor (deferred): limiter-wait cancel returns ctx err vs last API err inconsistency
- Task 3: minor (deferred): discovery 1.1s spacing measured at schedule time (latency jitter)
- Task 4: minor (deferred): unused model.Raw field; price/success-rate errors bucketed as schemaErrors
- Task 4: minor (deferred): REPORT inputKeys doesn't resolve $ref — not exhaustive
- Task 4: note: reviewer's literal key grep denied by permissions; implementer reported 0; owner asked to re-run before push
- Task 5: minor (deferred): empty catalog (nil models) cached as null hit for 1h
- Task 5: minor (deferred): model id unescaped in URL path (? or .. alters request) — escape per segment
- Task 5: minor (deferred): no CLI test that `models list` hides chat
- Task 5: note: run() now maps *api.Error via AsOutput (exit 3/4/7) — beyond brief, needed
- Task 6: minor (deferred): property-level oneOf/anyOf shown as "any"; items.enum/minItems/maxLength/pattern not surfaced
- Task 6: minor (deferred): required-only anyOf ("at least one of") ignored
- Task 6: minor (deferred): Endpoint picks sorted-first path for multi-path schemas
- Task 6: minor (deferred): summary.go:198 mergeOwn writes into shared resolved properties map (mutation) — copy first
- Task 6: minor (deferred): truncation caps (600 Notes / 300 variant desc) untested
- Task 7: note: ledger.List(path, limit, warn io.Writer) — 3rd param (nil ok); carry to Tasks 8/11
- Task 7: minor (deferred): ledger.Append doesn't create parent dir — callers must EnsureHome (carry to Task 8)
- Task 8: note: Submit(…, warn ...io.Writer) variadic, defaults os.Stderr; Wait doesn't update ledger — Task 11 run must pass quiet-aware writer and append final state
- Task 8: minor (deferred, carry to Task 11): Submit warn variadic defaults os.Stderr — run must pass warnWriter
- Task 8: minor (deferred, carry to Task 11): Wait non-context Get error lacks Details.taskId — run must add taskId
- Task 8: minor (deferred): malformed2xx matches api error message prefixes — use typed marker
- Task 8: minor (deferred): Record lacks json tags (resultJson/response)
- Task 8: minor (deferred): timeout doesn't bound in-flight Get
- Task 8: minor (deferred): dial-error test doesn't prove retries
- Task 9: minor (deferred): file-key value equal to an existing file (no path prefix/media ext) gets uploaded — harden in stage 2 (schema-based)
- Task 9: minor (deferred): uploaded list order non-deterministic (map iteration)
- Task 9: minor (deferred): existing dir/FIFO path says "file not found"; no symlink-to-dir test; no early-abort cleanup test
- Task 9: note (stage 7): upload body is chunked (unknown ContentLength) — verify kie upload host accepts it in smoke test
- Task 10: ⚠️ resolved: api.Client.HTTP has no custom RoundTripper (grep) — result GETs carry no auth
- Task 10: minor (deferred): transport error on first GET skips download-url fallback
- Task 10: minor (deferred): 0o755 test assertion umask-dependent; no concurrent-collision test
- Task 10: minor (deferred): copy fallback triggers on any non-ErrExist link error; dest visible while partially written on fallback; safeToken not unit-tested for edge inputs
- Task 11: note: golangci-lint 42 errcheck + 1 staticcheck from earlier tasks — stage close requires clean lint → final fix wave
- Task 11: minor (deferred): task wait outDir precedence should be --out > ledger outDir > cfg.OutDir > ./kie-output
- Task 11: minor (deferred): offline catalog fallback loses TaskType → video gets 10m timeout; reuse name heuristic
- Task 11: minor (deferred): chat guard relies only on schema IsJob — also check catalog TaskType "chat"
- Task 11: minor (deferred): no SIGINT-through-Execute test; error code passed via details "code" key convention; timeout test wall-clock
- Task 11: ⚠️ download uses api c.HTTP with 60s total Timeout — large video downloads would fail (load-bearing for stage 7) → final fix wave
- Final fix: re-review clean; minor (deferred): groupRunE message dangling ': ' with no args
