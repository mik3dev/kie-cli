# kie-cli — Diseño (Fase 00)

Fecha: 2026-10-17 · Estado: aprobado por partes en conversación; pendiente aprobación del documento completo

## 1. Intención

CLI en Go (binario único `kie-cli`) para generar **imágenes, videos y audio** con la API de kie.ai, diseñado como **herramienta para agentes de IA**. El usuario principal es Claude Code, pero el CLI no depende de ningún agente. Los assets generados se usan en los proyectos del dueño.

El motivo de crearlo: kie.ai reúne más de 200 modelos detrás de una sola API, pero usarla bien obliga a manejar muchos detalles. Hay que subir archivos locales, hacer polling de tareas asíncronas, respetar rate limits, parsear JSON dentro de JSON, lidiar con nombres de campo distintos en cada modelo y descargar los resultados antes de que expiren. La skill oficial `kie-models` (ver `docs/research/kie-official-skill-reference.md`) deja todo eso al agente con curl+jq, lo que gasta tokens y produce errores. Este CLI encapsula esos detalles y viene con skills propias que enseñan al agente a usarlo.

## 2. Objetivos, no-objetivos, anti-objetivos

**Objetivos (MVP)**
- Cubrir imagen, video y audio (música, TTS, efectos) en **todos los modos de entrada que exponga la API**: texto, frame inicial, inicial+final, referencias, video-a-video, extend, upscale, avatar/lip-sync, motion control, operaciones Suno, etc.
- Ser **dirigido por catálogo**: el agente consulta el catálogo vivo, lee el schema y nombra el modelo exacto. No hay lista de modelos hardcodeada.
- Ejecutar el ciclo completo con un solo comando: subir, enviar, esperar y descargar.
- Ofrecer **presets curados** opcionales.
- Incluir skills instalables para Claude Code y otros agentes.
- Distribuir como binario fácil de instalar (macOS, Linux, Windows).

**No-objetivos (fuera del MVP)**
- Modelos de chat (el agente ya es el LLM).
- Recepción de webhooks (`callBackUrl`). El CLI hace polling.
- TUI o modo interactivo.
- Gestión de recursos persistentes: clones de voz Suno, personajes/voces de Gemini Omni, personas. Sí se pueden *invocar* con `kie-cli run` porque son modelos del catálogo, pero no hay comandos dedicados.
- Endpoints legacy bajo `/old-model/` (4o-image, flux-kontext y suno con rutas propias). Los modelos actuales equivalentes ya usan `createTask`.

**Anti-objetivos (fracaso aunque "funcione")**
- Que el agente tenga que adivinar nombres de modelo o de campo.
- Gastar créditos en una tarea que se podía rechazar localmente por input inválido.
- Que se pierdan assets porque solo quedó la URL y expiró.
- Una salida que el agente no pueda parsear: texto mezclado en stdout, prompts interactivos o errores ambiguos.
- Que la API key se filtre a logs, stdout o archivos del repo.

## 3. Hechos de la API que condicionan el diseño

Detalle completo en `docs/research/`. Resumen:

| Hecho | Consecuencia |
|---|---|
| Casi todo usa `POST /api/v1/jobs/createTask {model, input, callBackUrl?}` + `GET /api/v1/jobs/recordInfo?taskId=`. Esto incluye Veo, Runway y Suno (`ai-music-api/<op>`). | Un solo motor genérico sirve para los tres tipos de asset. |
| Catálogo vivo: `GET /api/v1/models?taskType=&provider=&q=`. Schema por modelo: `GET /api/v1/models/{model}/schema`, que devuelve un OpenAPI 3.1 inline con `$ref` sin resolver, puede ser `null` y puede traer `oneOf`. También hay `/price` y `/success-rate`. | El CLI descubre, valida y resume a partir del schema. El slash del id de modelo **no** se codifica. |
| Rate limit: catálogo, schema, price y success-rate comparten **1 req/s por cuenta**. `createTask` permite 20/10s. `recordInfo` permite 10/s por taskId. | Caché local y limitador por clase de endpoint. |
| Los errores llegan con HTTP 200 y `code` en el body (401, 402 sin créditos, 404, 422, 429, 433, 455, 500). | Revisar siempre `code` y mapearlo a códigos de salida. |
| Los archivos de entrada deben ser URL. La subida va a otro host, `https://kieai.redpandaai.co` (`/api/file-stream-upload`, `/api/file-base64-upload`, `/api/file-url-upload`), y expira en 24h. | Subida automática de rutas locales. |
| Los resultados expiran a los **14 días**. `recordInfo.data.param.input` es JSON-en-string-en-JSON. | Descargar siempre por defecto. |
| La forma del resultado varía: `response.resultUrls[]` (mayoría), `response.data[].audio_url` (Suno audio, con `id` = audio_id), `response.resultObject` (texto/análisis), `lastFrameUrl` (Seedance). | Un extractor de resultados que conozca estas formas. |
| Los campos de media tienen más de 15 nombres distintos (`image_url`, `image_urls[]`, `input_urls`, `first_frame_url`, `tail_image_url`, `end_image_url`, `video_list[{url}]`…). Los tipos cambian (`duration` string vs int). | El agente usa **claves exactas del schema** y el CLI convierte tipos según el schema. |
| Encadenamiento: extend/upscale/4K piden el `task_id` de una tarea previa; Suno pide `task_id`+`audio_id`; Grok segment-edit pide el task de segment-map. | La salida JSON debe exponer todos los ids que se necesitan para encadenar. |
| Hay modelos síncronos: el path del schema no es `createTask` (p. ej. `/api/v1/omni/audio/create`, timestamped lyrics). | `run` decide según el path que declara el schema. |

### Prior art (detalle en `docs/research/endpoints-and-prior-art.md`)
- No existe CLI ni SDK oficial. Lo único oficial es la skill `kie-models` (curl+jq).
- Ninguno de los 13 repos revisados usa el catálogo vivo `/api/v1/models`. Todos hardcodean modelos o generan el catálogo desde la documentación, y por eso se desactualizan.
- Referencia más cercana: **douhashi/kie-ai-cli** (Go, MIT, activo, 0 estrellas). Tiene catálogo generado desde la documentación y embebido, flags derivados del schema, `--json`, ledger SQLite, `skill install` y rechazo de campos desconocidos antes de enviar. Conviene estudiarlo antes de la etapa 1 para tomar ideas, no para hacer fork (ver §10).
- Ideas que se adoptan: `--dry-run`, schema cacheado 24h (VelsVisual), lista de hosts permitidos para subir/descargar (kie-mcp) y fallback a `/api/v1/common/download-url` si el GET directo del resultado falla (douhashi).
- No existe endpoint para listar tareas. Para no perder tareas pagadas si el agente pierde el `taskId`, el CLI agrega cada tarea creada a `~/.kie-cli/tasks.jsonl` (taskId, modelo, fecha, `--out`). `kie-cli task list` lo lee. Es un archivo JSONL, sin SQLite.

## 4. Interfaz (aprobada)

```
kie-cli auth login | status                      # guarda key (o usa KIE_API_KEY)
kie-cli credits                                  # saldo
kie-cli models list   [--type video] [--provider Kling] [--refresh]
kie-cli models search "<texto>" [--type ...]
kie-cli models schema <model> [--raw]            # resumen de campos de input; --raw = OpenAPI
kie-cli models price  <model>
kie-cli models health <model>                    # success-rate últimas 24h

kie-cli run <model|@preset> -p clave=valor ... [--input file.json] [--out DIR]
        [--dry-run] [--no-wait] [--timeout 30m] [--no-download]
kie-cli task list [--limit 20]                   # ledger local
kie-cli task get  <taskId>
kie-cli task wait <taskId> [--out DIR] [--timeout ...]

kie-cli preset list | show <name>
kie-cli skills install [--target DIR]
kie-cli version
```

### Reglas de `run`
- **Claves exactas del schema** en `-p`. Si se repite una clave que es array, los valores se acumulan. Si un valor empieza con `@`, se lee de archivo (`-p prompt=@prompt.txt`). `--input` acepta JSON con el objeto `input` completo, y los `-p` lo sobrescriben.
- **Conversión de tipos** según el schema: string, integer, number, boolean, array y object. Los objetos o arrays de objetos solo se aceptan vía `--input` o con `-p clave=<json>`.
- **Subida automática**: si un valor (incluidos los elementos de arrays y los objetos anidados) es una ruta local existente, el CLI lo sube con stream upload y lo reemplaza por la `downloadUrl`. Se aplica en campos `string` que el schema describa como URL/archivo. La heurística queda en manos del builder (nombre del campo `*_url(s)`, `format: uri` o descripción). Las URLs `http(s)://` se dejan tal cual.
- **Validación previa** del `input` contra el JSON Schema del modelo, con `$ref` resueltos: requeridos, enums, tipos, `oneOf`, min/max. Cada error indica el campo, el valor recibido y lo esperado. Si el schema es `null`, se envía sin validar y se avisa por stderr.
- **`--no-validate`** *(decisión del builder: hay schemas con errores conocidos, como los `required` de Suno)*: convierte los errores de validación en avisos por stderr y envía igual. La coerción de tipos se aplica de todos modos. La skill indica usarlo solo si el agente cree que el schema está mal.
- **Reintentos de `createTask`** *(decisión del builder: no se puede pedir confirmación interactiva)*: solo se reintenta cuando hay certeza de que la tarea no se creó (`code` 429, o fallo de conexión antes de enviar el request). Si el resultado es ambiguo (timeout de lectura, 5xx/455 tras enviar), **no se reintenta**. El CLI devuelve el error `submit_uncertain` (código de salida 8), lo registra en el ledger con estado `unknown` y en `message` pide revisar kie.ai/logs antes de volver a enviar. `recordInfo` y las descargas sí se reintentan con backoff.
- **`--dry-run`**: valida, no sube nada, imprime el body final que se enviaría (con las rutas locales marcadas como pendientes de subir) y el `pricingDesc`.
- **Por defecto**: enviar → polling (backoff de 3s a 15s, tope `--timeout`, por defecto 10m para imagen/audio y 30m para video según `taskType`) → descargar a `--out` (por defecto `./kie-output/`).
- **`--no-wait`**: devuelve `{taskId}` al instante. `task wait` retoma con el mismo comportamiento de descarga.
- **Modelos síncronos** (path ≠ `createTask`): POST al path del schema y devolver el body en `result`. Si contiene URLs de media, se descargan.

### Contrato de salida
- **stdout**: solo un objeto JSON por comando. **stderr**: progreso y avisos (desactivable con `--quiet`).
- Ejemplo `run` exitoso:
```json
{"ok":true,"taskId":"…","model":"kling-3.0/video","state":"success",
 "files":[{"path":"kie-output/kling-3.0-video_<taskId>_0.mp4","url":"https://…","kind":"video"}],
 "ids":{"audio_ids":["…"]},"result":{…response crudo…},
 "creditsConsumed":160,"costTime":95}
```
- En error: `{"ok":false,"error":{"code":"validation|auth|credits|rate_limit|submit_uncertain|task_failed|timeout|api|io","message":"…","details":{…}}}`.
- **Códigos de salida**: 0 ok · 2 uso/validación · 3 auth · 4 créditos · 5 tarea falló · 6 timeout · 7 rate limit/servicio no disponible · 8 envío incierto (`submit_uncertain`) · 1 otro.
- Nombre de archivo: `<model con / → ->_<taskId>_<n>.<ext>`. La extensión sale de Content-Type o de la URL. Nunca se sobrescribe un archivo existente: se añade un sufijo.

### Configuración y secretos (pedido del dueño: todo en un directorio propio)
Todo el estado del CLI vive en **`~/.kie-cli/`** (directorio 0700). Se puede sobrescribir con `KIE_CLI_HOME`.

```
~/.kie-cli/
  config.json     # 0600 — api_key, defaults (out_dir, timeouts), otros secrets futuros
  presets.yaml    # presets del usuario
  cache/          # catálogo (TTL 1h) y schemas (TTL 24h); --refresh invalida
  tasks.jsonl     # ledger local de tareas creadas
```

- La key se resuelve en este orden: `KIE_API_KEY` (env), luego `config.json`. `auth login` escribe el archivo leyendo la key de stdin sin eco. `auth status` muestra de dónde viene la key, nunca su valor.
- Al leer `config.json` se verifican sus permisos. Si son más abiertos que 0600, se avisa por stderr (estilo ssh).
- La key nunca se imprime. En los errores y en `--debug` aparece redactada.

## 5. Presets (aprobado)
- YAML. Los de fábrica van embebidos con `go:embed`. Los del usuario están en `~/.kie-cli/presets.yaml` y sobrescriben por nombre.
- Formato: `name`, `description` (cuándo usarlo), `model`, `params` (defaults, que los `-p` sobrescriben).
- Se usan como `kie-cli run @video-calidad -p prompt=…`.
- Lista inicial: la propone el builder a partir de `docs/research/` y de los precios (imagen rápida/calidad/edición, video t2v/i2v/first-last/barato, música, instrumental, TTS). El dueño la revisa.

## 6. Skills (aprobado)
- En el repo, carpeta `skills/kie-generate/`: `SKILL.md` + `references/{image,video,audio}.md`. Formato estándar Agent Skills.
- `SKILL.md` contiene el flujo (search/list → `models schema` → `run --dry-run` si es caro → `run` → usar `files[].path`), reglas críticas (nunca inventar modelos ni claves, revisar `credits` antes de video, `--no-wait` para paralelizar, encadenar con los ids de la salida) y el contrato de errores y códigos de salida.
- Las references cubren los patrones de modo por tipo de asset: cómo se nombran first/last frame, referencias y encadenamiento con Suno. Son guía y no reemplazan el schema.
- Se embeben en el binario. `kie-cli skills install` las copia a `~/.claude/skills/` por defecto o a `--target`. `npx skills add <repo>` funcionará solo si el repo se hace público.
- Van versionadas con el binario: un test verifica que cada comando y flag mencionado en las skills exista.

## 7. Arquitectura (decisión del builder, sugerencia)

```
cmd/kie-cli/main.go
internal/
  api/       cliente HTTP, envelope {code,msg,data}, mapeo de errores, limitador por clase
  catalog/   list/search/price/health + caché
  schema/    fetch, resolución $ref (percent-decode, sin trim), resumen, coerción, validación
  upload/    stream upload a redpandaai, detección de rutas locales
  task/      createTask, polling con backoff, modelos síncronos
  result/    extracción de URLs e ids (resultUrls / data[].audio_url / resultObject / lastFrameUrl)
  download/  descarga y nombrado
  preset/    carga y merge
  skills/    embed + install
  output/    JSON stdout, códigos de salida
skills/kie-generate/...
presets/default.yaml
```

Módulo Go: `github.com/mik3dev/kie-cli` (repo privado `https://github.com/mik3dev/kie-cli.git`).

Dependencias sugeridas: `spf13/cobra` (subcomandos), `gopkg.in/yaml.v3` y un validador JSON Schema con `oneOf` (p. ej. `santhosh-tekuri/jsonschema`). Todo lo demás con stdlib. Repo **privado** por ahora: la instalación es `make install` / `go install` (con `GOPRIVATE`) o descargar el binario de GitHub Releases privado vía `gh release download`. `goreleaser` arma los binarios multiplataforma. Homebrew tap queda para cuando el repo se haga público.

## 8. Etapas y estimaciones (aprobado)

*(Decisión del builder)* Se planifica e implementa **una etapa a la vez**: primero el plan de la etapa 1, y al final de cada etapa el dueño revisa antes de pasar a la siguiente. **Uso de la API real** (decisión del dueño): hay una key con **80 créditos**, pero solo se usa en smoke tests de validación, **nunca en CI**. Las etapas 1–6 se prueban contra mocks hechos con los ejemplos y schemas de la documentación (`docs/research/`). La etapa 7 verifica contra la API real y corrige lo que aparezca. Si ahí el catálogo o los schemas no se comportan como dice la documentación (por ejemplo, muchos schemas `null` o Suno, Veo o Runway ausentes del catálogo), el builder se detiene y lo consulta con el dueño antes de cambiar el diseño. Se pueden agregar flags nuevos si no rompen la interfaz aprobada, siempre que se mencionen en la revisión de la etapa. La revisión de presets que hace el dueño no bloquea la etapa 5: se publica la lista inicial y él la ajusta después.

Cada etapa tiene su propio plan de implementación (TDD, cobertura ≥80%). Las estimaciones son en días de trabajo asistido por agente, con incertidumbre de ±50%: al trabajar sin API real, la etapa 7 puede revelar retrabajo.

| # | Etapa | Contenido | Estim. |
|---|---|---|---|
| 0 | Fase 00 | investigación, diseño (este doc) | hecho |
| 1 | Núcleo | **Paso 0 (decisión del dueño): captura real sin costo.** Con la key, se ejecuta una vez un script que solo lee: `models` (catálogo completo), `schema` de todos los modelos (respetando 1 req/s, ≈4 min), varios `price`/`success-rate` y `chat/credit`. Las respuestas se guardan en `testdata/live/` como fixtures de los mocks, sin la key ni headers. Se revisa que Suno, Veo y Runway estén en el catálogo y cuántos schemas vienen `null`; si algo contradice el diseño, se consulta con el dueño. Luego: auth/config, cliente API + errores, `models list/search/schema/price/health` + caché + limitador, `credits`, upload, `run` (createTask + polling + descarga), `task get/wait`, contrato JSON y códigos de salida. Todo contra mocks construidos con los ejemplos de la documentación. | 3–4 |
| 2 | Validación | resolución `$ref`, coerción por tipo, validación JSON Schema con `oneOf`, mensajes de error útiles, `--dry-run` con precio | 2 |
| 3 | Encadenar | exponer `ids` en la salida (taskId, audio ids de Suno, lastFrameUrl); `task get` con los mismos ids; pruebas de flujos encadenados (extend, upscale, Suno extend) | 0.5–1 |
| 4 | Audio y casos raros | extractor de Suno (`data[]`), modelos síncronos, `suno-recovery-audio` (polling propio), resultados de texto (`resultObject`) | 1–2 |
| 5 | Presets + skills | presets YAML + lista inicial; skills `kie-generate` + references; `skills install`; test de consistencia entre skill y CLI | 1–2 |
| 6 | Distribución | goreleaser, CI (test + lint), README, instalación | 0.5–1 |
| 7 | Verificación real | con una `KIE_API_KEY` válida: recorrer el catálogo, descargar todos los schemas y comprobar resumen, detección de campos de archivo y validación; smoke tests (imagen barata, TTS, Suno, `--dry-run` de video, un encadenamiento); corregir bugs y actualizar los mocks con respuestas reales | 1–2 |
| | **Total** | | **≈ 9–14 días** |

## 9. Pruebas
- Unitarias: coerción, resolución `$ref`, validación, extracción de resultados (con fixtures reales de `docs/research/`), nombrado de archivos y mapeo de errores.
- Integración: servidor `httptest` que imita createTask, recordInfo, upload, catálogo y schema, incluidos errores con HTTP 200 + `code`, 429 y tareas que fallan.
- Smoke tests reales: tras el build tag `live` (`go test -tags=live ./...`) y además `KIE_E2E=1`. Se ejecutan **solo a mano**. El workflow de CI no usa el tag ni tiene la key como secret. Presupuesto: **80 créditos en total**. Un video de 5s cuesta del orden de 160 créditos, así que en video solo se hace `--dry-run` y encadenamientos sin costo. Se generan de verdad: 1–2 imágenes baratas, 1 TTS y, si alcanza, 1 Suno. Antes de cada generación se comparan `credits` y `price`, y el test se salta si el costo supera el presupuesto restante.

## 10. Riesgos y preguntas abiertas
- **Sin validación real todavía.** Todo sale de la documentación. El desarrollo se prueba contra mocks y la verificación real (manual, 80 créditos) es la etapa 7.
- Documentación de Suno inconsistente (lista de `required` errónea, path de recovery contradictorio): validar en vivo.
- Schema `null` para modelos recién añadidos: se envía sin validar, con aviso.
- La detección de "campo que acepta archivo" es heurística: probarla contra todos los schemas del catálogo en la etapa 7.
- Recursos previos sin endpoint de creación conocido (Seedance `asset://`): fuera del MVP.
- La estabilidad de kie.ai es "best effort" según su documentación: ver política de reintentos de `createTask` en §4 (riesgo de doble cobro).
