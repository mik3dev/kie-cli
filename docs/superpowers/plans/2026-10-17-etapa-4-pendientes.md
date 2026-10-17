# Etapa 4 — rulings y pendientes diferidos

Extraído del ledger SDD al cerrar etapa 4.

## Rulings
- Ruling 1: offline catalog fallback + non-job schema → usage error "cannot tell chat vs sync, retry online" (implementer choice) accepted — safer than POSTing a possibly-chat body — cost if wrong: sync model unusable while catalog unreachable (rare)

## Diferidos / notas
- Task 1: minor (deferred): bare string under *urls key accepted; byte-order key sort; SyncIDs drops all on one wrong-typed id; stdstrings alias (rename local strings helper)
- Task 1: ⚠️ carried: task paths must always fill IDs.TaskID (now omitempty) — check in T2/T3 reviews
- Task 2: ⚠️ resolved: download.All creates outDir (MkdirAll)
- Task 2: minor (deferred): offline-fallback branch untested; chat guard after schema.Fetch (fetch error masks chat message); syncDownloadErr forces CodeIO; sync submit_uncertain message mentions "task"
- Task 3: minor (deferred, stage 7): recovery 500 retried by GET policy before task_failed; HTTP 5xx conflated with envelope 500 (task_failed); 200 with null data → success with files []
- Final review: minor (deferred): uncertainMsg says "task" for sync calls; sync-specific wording clearer
