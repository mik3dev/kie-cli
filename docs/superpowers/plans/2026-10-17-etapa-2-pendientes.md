# Etapa 2 — rulings y pendientes diferidos

Extraído del ledger SDD al cerrar etapa 2.

## Rulings
- Ruling 1: Task 1 also wires schema.Coerce into runModel (InputSchema after Fetch; nil on error/null) so `run` never regresses between T1 and T4; T4 keeps the rest of the flow — plan gap — cost if wrong: small overlap with T4 diff
- Ruling 2: plan T1 mandated literal guessing in Coerce for nil schema/unknown keys; final review shows it retypes --input JSON strings and @file contents (stage 1 sent them literal) — restrict guessing to plain -p values only, before MergeInput; Coerce leaves unknown/nil-schema values untouched — cost if wrong: an agent passing a number via --input to a null-schema model gets it as given (correct JSON already)

## Diferidos / notas
- Task 1: minor (deferred): @file content now literal-guessed for unknown keys / nil schema
- Task 1: minor (deferred): Coerce output aliases unchanged input sub-structures (shallow new map)
- Task 1: minor (deferred): float coercion round-trips via float64 (>17 sig digits lose precision; ints exact via FormatInt)
- Task 2: minor (deferred): required Problem message is custom "missing required field" (not library text)
- Task 2: minor (deferred): oneOf multi-match names branches from ROOT oneOf even for nested oneOf
- Task 2: minor (deferred): non-ValidationError from Validate (bad Go type) indistinguishable from compile error
- Task 2: minor (deferred): unknownFields ignores patternProperties; loose variant-N test assertions; ignored errs in TestValidateSeedance
- Task 2: ⚠️ resolved: Got 100-rune truncation not in Validate → carried to Task 4 (output layer)
- Task 3: ⚠️ resolved: PropSchema finds oneOf-branch props (TestHasURIFormatOneOf passes on separate-vocals audio_url)
- Task 3: minor (deferred): containsURIFormat walks default/example/enum values (theoretical false positive)
- Task 4: minor (deferred): same basename in different dirs → identical placeholders (path disambiguates)
- Task 4: minor (deferred): null-schema test asserts brittle substring
- Final review: minor (deferred): capGot only truncates strings; integer field "5.0"/"1e3" stays string; object file-field walks nested text strings as paths
