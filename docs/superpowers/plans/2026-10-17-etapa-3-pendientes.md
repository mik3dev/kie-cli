# Etapa 3 — pendientes diferidos

Extraído del ledger SDD al cerrar etapa 3. Sin rulings en esta etapa.

- | T1 self | tests (data[] entries without id/audio_url) vs impl note | consistent |
- | T2 self | Suno generate params must satisfy stage-2 validation on live schema; plan lets implementer adjust + note | ok |
- Task 1: minor (deferred): one malformed data[] element drops all Suno tracks/ids (whole-array decode)
- Task 1: minor (deferred): empty-response loop lacks {"data":[1]} / {"data":null} cases
- Task 2: minor (deferred): unchecked type assertions in chain_test.go; no ids test on failed/timeout paths
- Task 2: minor (deferred): download kind follows Content-Type over URL ext (fake image/png → .mp3 kind "image"); verify real Suno CT in stage 7
- Final review: minor (deferred): no per-track audio_id↔url mapping when data[] partial/deduped (agent maps via files[].url ↔ result.data[].audio_url)
- Final review: minor (deferred): download.go comment "URL ext wins" wrong for kind; task get ids uses API taskId (fallback to arg if empty)
- Final review: note (stage 7): verify real recordInfo uses response.data[] (not sunoData[]) and Suno CDN Content-Type
