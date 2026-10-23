---
source_sha: 7e38ef34503d1582f3a4a5fc3e6171c1d2f137aa52dae78d9840504057637151
---

# Guía de publicación de versiones

Cómo una versión de Evey pasa de una rama a una imagen que un operador puede
fijar. Escrita antes de la primera versión etiquetada, para que la primera siga
un procedimiento en lugar de inventarlo.

## Qué hace la CI con una etiqueta (léelo antes de etiquetar)

`.github/workflows/ci.yml` construye y publica imágenes en cada push y en cada
etiqueta `v*.*.*`, en tres variantes del entorno de ejecución de los plugins
(`` (ligera), `-nodejs`, `-full`), en `docker.io/eveyproject/evey` y
`ghcr.io/eveyproject/evey`:

| Lo que haces push | Etiquetas de imagen publicadas (cada una también `-nodejs` / `-full`) |
|---|---|
| una rama (por ejemplo `master`) | `master`, `sha-<short>` |
| la etiqueta `v0.2.0-rc.1` (versión previa: lleva un guion) | `0.2.0-rc.1`, `sha-<short>` |
| la etiqueta `v0.2.0` (estable) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Tres consecuencias:

- **Una etiqueta de rama cambia.** `master-full` apunta a lo último que se
  hizo push; dos `docker compose pull` con un día de diferencia pueden ejecutar
  código distinto, y volver «a `master-full`» no designa ninguna versión
  concreta. Los operadores deben fijar una etiqueta `sha-<short>` (o una
  etiqueta de versión), nunca un nombre de rama: es lo que
  `scripts/evey-update.sh` muestra al final para ponerlo en `.env`.
- **Una versión previa nunca modifica `latest`.** `latest` es la versión
  estable más reciente; una versión previa de la beta cerrada no la ve nadie
  que siga `latest`. (Una versión previa tampoco recibe las etiquetas `X.Y` /
  `X`: son solo para versiones estables.)
- **Multiarquitectura solo en las etiquetas de versión.** Una etiqueta
  `v*.*.*` (estable o previa) construye amd64 + arm64; un push de rama solo
  construye amd64, salvo que el mensaje del commit lleve `[arm64]` o que el
  workflow se lance a mano. Un operador ARM fija una etiqueta de versión.

El commit queda grabado en el binario (`evey version`, `/healthz`) y en las
etiquetas de la imagen (`org.opencontainers.image.revision`), así que siempre
se puede saber qué código se ejecuta sin fiarse de una etiqueta.

## Publicar una versión previa (beta cerrada)

1. **Comprobaciones superadas en el commit exacto**: `make pre-push` en local.
   La CI ejecuta las mismas comprobaciones al hacer push.
2. **Probada antes en una instalación real.** El commit tiene que haberse
   ejecutado ya en el despliegue de referencia como etiqueta `sha-<short>`,
   actualizado con `scripts/evey-update.sh --tag sha-<short> --apply` y
   verificado con `--commit <sha>`. Una etiqueta es un compromiso con otros
   operadores: créala después de esa verificación, no antes.
3. **`CHANGELOG.md`**: mueve las entradas de `[Unreleased]` bajo la nueva
   versión, con la fecha. Formato Keep a Changelog, solo con frases pensadas
   para los usuarios.
4. **Etiqueta ese commit**, con una etiqueta anotada, y haz push solo de la
   etiqueta:
   ```bash
   git tag -a v0.2.0-rc.1 -m "v0.2.0-rc.1"
   git push origin v0.2.0-rc.1
   ```
5. **Vigila la CI** mientras publica `0.2.0-rc.1` (+ variantes). Después, en
   el despliegue de referencia,
   `scripts/evey-update.sh --tag 0.2.0-rc.1 --apply`: la etiqueta de versión
   sustituye la fijación `sha-`.
6. **Anuncia la etiqueta** con su sección del changelog. Los operadores fijan
   `eveyproject/evey:0.2.0-rc.1` (o `-full`) en su línea `image:`.

Crear etiquetas de versión previa no cuesta nada: `-rc.2`, `-rc.3` según
haga falta. Lo que no se puede es retirarlas: una etiqueta publicada sigue
publicada. Para corregir una versión previa defectuosa, publica la siguiente;
nunca muevas una etiqueta.

## Publicar una versión estable

Los mismos pasos, con una etiqueta `vX.Y.Z` sin sufijo. Hay que comprobar algo
que una versión previa no necesita: la versión mayor de Postgres que incluye la
imagen debe figurar en el changelog, porque un operador con la base de datos
integrada tiene que seguir la sección de actualización de
[installation.md](../installation.md) cuando cambia.

## Volver atrás una versión

Las imágenes se quedan en el registro. Un operador vuelve atrás con
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(el script de actualización muestra esa línea exacta). En el lado de la
publicación no hay nada que deshacer; publica una `-rc.N+1` corregida cuando
la corrección esté lista.
