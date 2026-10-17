# Etapa 7 (Verificación real) — Resultados, rulings y pendientes

## Corrida real

- **Créditos:** 80 → 61.4, es decir, se gastaron 18.6 de un presupuesto de 30. La corrida fue una sola, manual, y no forma parte del CI.
- **Pasaron:**
  - z-image
  - la cadena upload → recraft/remove-background
  - `--no-wait` seguido de `task wait`
  - `@imagen-rapida`
  - `@instrumental` (Suno V6): 2 `audio_ids` y 2 archivos de audio, más el dry-run de extend.
- **Falló `@voz`** (ElevenLabs multilingual v2): el proveedor respondió 500 "Internal Error, Please try again later" dos veces, con `creditsConsumed: 0`. El request es válido según el schema. No hay fixture de TTS.
- **Se saltó suno-recovery-audio:** su `pricingDesc` viene vacío y el test no gasta si no conoce el precio.
- **Fixtures:** `testdata/live/records/` guarda 3 records con las URLs reescritas a `cdn.example`. La key aparece 0 veces y no hay campos que identifiquen la cuenta.

## Hallazgos y rulings

1. **F1 — detección de parámetros de archivo con schema** (`upload.SchemaFileKey`).
   - Antes, cualquier clave cuyo nombre contuviera "audio", "image", etc. se trataba como archivo. Eso incluía `audio_id`, `image_size`, `audio_weight` y otras, y era un riesgo de privacy: se podía subir un archivo local por error.
   - Ahora una clave que el schema conoce cuenta como archivo en dos casos:
     - si su subárbol tiene `format: uri`, lo que cubre objetos anidados y arrays de objetos;
     - si es un string, o un array de strings, sin enum y su nombre parece de archivo.
   - Sin schema se usa solo el nombre, con una lista de exclusiones.
   - El review final encontró una regresión: los parámetros anidados (gemini-omni, kling-3.0-omni, pixverse-v6, gemini TTS) habían dejado de subirse. Ya está corregida.
   - El CLI y el sweep usan el mismo predicado.
2. **F2:** `models search` ahora también busca por proveedor, así que "suno" encuentra ai-music-api/generate.
3. **F3:** suno-recovery-audio no tiene precio publicado. No requiere código.
4. **F4:** los arrays se pasan repitiendo `-p` (`-p image_urls=a -p image_urls=b`). No se separa por comas porque URLs y prompts pueden contenerlas. Está documentado en el README y en la skill.
5. **F5:** el README dice "180+ modelos" (el catálogo tiene 187 sin contar chat).
6. **F6:** el harness también captura los records de tareas fallidas, tomando el `taskId` de `error.details`.
7. **F7:** el record de Suno trae `image_url` y `stream_audio_url` por track. El CLI baja solo los 2 audios, y un test de regresión lo fija.

## Pendientes (diferidos, no bloquean)

- **Reprobar `@voz`** cuando ElevenLabs se estabilice (≈1 crédito) y capturar el fixture de TTS:
  `KIE_E2E=1 KIE_E2E_PAID=1 KIE_E2E_CAPTURE=1 KIE_E2E_BUDGET=2 go test -tags live -run TestLivePaid5 ./internal/cli/`.
- **`verify_url` y otros strings no-enum** que el schema conoce, cuyo nombre parece de archivo y que no tienen `format: uri`, todavía se suben si el valor es un archivo local existente. El riesgo es bajo. Se podría exigir `format: uri` cuando el schema declara la clave.
- **Un array sin `items`** no se considera candidato a archivo. Es poco probable en los schemas de kie.
- **El test de Suno no distingue `stream_audio_url`**, porque en el fixture es igual a `audio_url`. Hoy lo cubre solo el chequeo del sufijo `.mp3`.
- **`sanitize` solo reescribe URLs `https://`**; las `http://` quedarían sin tocar. La escritura igual está protegida por el chequeo de la key.
- **El presupuesto depende de que el saldo se actualice al instante.** Se podría usar como piso la suma de los costos declarados.
- **`TestLiveDryRunPresets` solo verifica el exit code.** Podría verificar también los `pendingUploads`.
- **`looksLikePath` no reconoce rutas de Windows** (`.\`, `C:\`). Viene de la etapa 6.
- **`SWEEP.md`:** cuando un modelo no tiene campos de archivo, la línea queda `- model: ` con un espacio al final. Es solo cosmético.
