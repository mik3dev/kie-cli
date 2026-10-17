# Etapa 5 (Presets + skills) — Rulings y pendientes

## Rulings

1. **Suno V6 en los presets.** `musica` e `instrumental` usan `model: V6`, no V5 como decía la tabla del plan. El schema vivo de `ai-music-api/generate` dice "Required. Use V6, V6_MINI or V6_WILD", y de V4 a V5_5 están discontinuados. Era un defecto del plan. Ahora un test fija que el modelo esté en {V6, V6_MINI, V6_WILD}.
2. **`submit_uncertain` en la skill.** No se reenvía la tarea. El agente avisa al usuario que la tarea puede existir, le pide revisar https://kie.ai/logs y solo reenvía si el usuario confirma. `task list` muestra el intento como `state: unknown` y sin taskId, así que no sirve para decidir. Lo detectó la review final, y un test exige que `kie.ai/logs` aparezca en SKILL.md.
3. **Contenido de la skill y descripciones de presets en inglés.** Los nombres de los presets van en español, como el ejemplo del spec (`@video-calidad`).
4. **`"preset": "<name>"`** aparece en la salida de `run` (éxito, `--no-wait`, dry-run y sync) cuando se invoca con `@name`.

## Pendientes (diferidos, no bloquean)

- `task wait` no muestra `preset`, porque la tarea enviada con `--no-wait` no guarda de qué preset salió.
- La salida `"preset"` del camino sync no tiene test propio. Usa el mismo helper `withPreset` que el resto.
- `preset.Parse` ignora en silencio los documentos YAML que vienen después del primero (`---`).
- Un archivo de presets ilegible (sin permisos, o un directorio) da `usage`/exit 2; `io`/exit 1 sería más preciso.
- Sin HOME: `preset list` y `run @x` dan `io`/exit 1, pero `skills install` da `usage`/exit 2.
- `gopkg.in/yaml.v3` está archivado upstream. `go.yaml.in/yaml/v3` es el reemplazo mantenido.
- Si `skills install` falla a mitad de la copia, el error no lista los archivos ya escritos, y `target` sale tal como lo pasó el usuario (puede ser relativo).
- Los escalares YAML sin comillas cambian de tipo (por ejemplo, las fechas). Conviene documentar "pon comillas a los strings" en los docs de usuario (etapa 6).
- El checker de consistencia de la skill solo revisa el texto que viene después de `kie-cli `:
  - Las menciones sueltas a `--flag` no se revisan.
  - En los flags cortos solo se mira el primer carácter.
  - Para los shorthand no se miran los flags persistentes del padre.
- `TestInstallCopiesTree` usa `syscall.Umask`, que no existe en Windows. Si CI corre tests en Windows (etapa 6), hay que mover el test a un archivo con build tag unix.
- Las references de la skill traen nombres de campo por familia (Seedream, Wan, Seedance…) sacados de `docs/research`, sin test. Los de Kling, Veo, Topaz, Recraft, ElevenLabs y Suno se revisaron a mano contra los schemas vivos.

## Verificación en la etapa 7 (API real)

- Los presets de fábrica contra la API real, sobre todo `video-inicio-fin` (veo-3-1 fast, precio sin confirmar) y la voz `EkK5I93UQWFDigLMpZcX`.
