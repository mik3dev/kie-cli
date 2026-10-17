# Etapa 6 (Distribución) — Rulings y pendientes

## Rulings

1. **Versiones de las actions.** Las actions usan la última versión mayor de cada una (`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v9`, `goreleaser/goreleaser-action@v7`) en lugar de las que fijaba el plan (v5/v6/v8/v6). Las del plan corren sobre node20, que GitHub está deprecando. Se verificó con `gh api` que los tags y los inputs existen.
2. **Fallback de versión.** Un binario instalado con `go install …@latest` muestra la versión de `debug.ReadBuildInfo` en lugar de `dev`. Si `-ldflags` define la versión, esa gana.
3. **`go.mod` tidy.** Desde la etapa 5, `pflag` estaba como dependencia indirecta y el paso de tidy del CI habría fallado en su primera corrida; ahora va como directa. Se agregó `make tidy-check`.
4. **Windows en CI.** Solo se verifica que compila (`GOOS=windows go vet` y el build de goreleaser). Los tests no corren en un host Windows.

## Pendientes (diferidos, no bloquean)

- **CI real sin probar.** Nunca corrió en GitHub, ni se probó un release con tag. Antes de mergear o de tagear conviene abrir un PR draft y mirar la primera corrida.
- **Rutas de Windows en el upload** (`internal/upload/upload.go`): `~\file` no se expande, y `looksLikePath` no reconoce `.\` ni `C:\`. Un archivo inexistente con extensión que no es de medios se manda como texto. Los archivos que sí existen funcionan.
- **Pinning de las actions.** Las actions están fijadas por tag mayor (mutable) y goreleaser va con `~> v2` en el job de release, que tiene permiso `contents: write`. Se podría fijar por SHA y agregar Dependabot. El release tampoco depende de que el CI pase; solo corre tests con `-race`, sin lint.
- **Huecos del test guardián de workflows:** no detecta `${{ secrets }}` suelto, `toJSON( secrets )` con espacios o en mayúsculas, y `-tags integration live`, donde se mira solo el primer token.
- **Tests débiles.** `TestGoreleaserInjectsVersion` y `TestReadmeDocumentsExitCodes` solo comparan substrings.
- **README:**
  - Menciona los smoke tests `live` (`KIE_E2E`), que todavía no existen (llegan en la etapa 7).
  - No menciona `--no-validate`.
- **Test de la variable `goos`.** El test que la reemplaza la restaura a mano, no con `t.Cleanup`. Hoy no hay `t.Parallel` en ese paquete, así que no causa problemas.
- **`upload.go` usa `os.UserHomeDir`**, que funciona en Windows, pero no pasa por la inyección de `deps.Env`.

## Para el dueño

- **Primer release:** después del merge, `git tag v0.1.0 && git push origin v0.1.0`. Eso dispara `release.yml`, que publica el release privado.
