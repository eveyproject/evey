---
source_sha: f7107d811a1a25b00c55dbd3789c083144308003ef8ab0ea3e52905c56b1bc7c
---
# Guía de publicación de versiones

Cómo una versión de Evey pasa de una rama a algo que un operador puede fijar.
Escrita antes de la primera versión etiquetada, para que la primera siga un
procedimiento en lugar de inventarlo.

## Qué hace la CI con una etiqueta (léalo antes de etiquetar)

`.github/workflows/ci.yml` construye y publica imágenes en cada push y en cada
etiqueta `v*.*.*`, en tres variantes de entornos de extensiones (`` (lean),
`-nodejs`, `-full`), en `docker.io/gravityz/evey` y `ghcr.io/evey-project/evey`:

| Usted hace push de | Etiquetas de imagen publicadas (cada una también `-nodejs` / `-full`) |
|---|---|
| una rama (p. ej. `beta-0.0.1`) | `beta-0.0.1`, `sha-<short>` |
| la etiqueta `v0.2.0-beta.1` (versión previa: lleva un guion) | `0.2.0-beta.1`, `sha-<short>` |
| la etiqueta `v0.2.0` (estable) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Tres consecuencias:

- **La etiqueta de una rama se mueve.** `beta-0.0.1-full` apunta a lo último que
  se subió; dos `docker compose pull` con un día de diferencia pueden ejecutar
  código distinto, y una vuelta atrás «a `beta-0.0.1-full`» no lleva a ninguna
  parte. Los operadores deben fijar una etiqueta `sha-<short>` (o una etiqueta
  de versión), nunca un nombre de rama - eso es lo que `scripts/evey-update.sh`
  imprime al final para que lo pongan en `.env`.
- **Una versión previa nunca toca `latest`.** `latest` significa la versión
  estable más reciente; un corte de beta cerrada es invisible para quien lo
  sigue. (Una versión previa tampoco recibe etiquetas `X.Y` / `X`: son solo para
  las estables.)
- **Multiarquitectura en cada build** (amd64 + arm64), para que un operador ARM
  pueda fijar cualquiera de ellas.

El commit queda sellado en el binario (`evey version`, `/healthz`) y en las
etiquetas de la imagen (`org.opencontainers.image.revision`), así que «¿qué
código es este?» siempre tiene respuesta sin fiarse de una etiqueta.

## Cortar una versión previa (beta cerrada)

1. **Controles en verde sobre el commit exacto**: `make pre-push` en local. La
   CI ejecuta lo mismo en el push.
2. **Probada antes en una instalación real.** El commit tiene que haber corrido
   ya en el despliegue de referencia como etiqueta `sha-<short>`, actualizado con
   `scripts/evey-update.sh --tag sha-<short> --apply` y verificado con
   `--commit <sha>`. Una etiqueta es una promesa a otros operadores; hágala
   después de la prueba, no antes.
3. **`CHANGELOG.md`**: mueva las entradas de `[Unreleased]` bajo la nueva
   versión, con la fecha. Formato Keep a Changelog, redacción solo para
   usuarios.
4. **Etiquete desde ese commit**, con etiqueta anotada, y suba solo la etiqueta:
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **Vigile la CI** mientras publica `0.2.0-beta.1` (+ variantes). Después, en el
   despliegue de referencia, `scripts/evey-update.sh --tag 0.2.0-beta.1
   --apply` - fijar la etiqueta de versión sustituye la fijación `sha-`.
6. **Anuncie la etiqueta** con su sección del changelog. Los operadores fijan
   `gravityz/evey:0.2.0-beta.1` (o `-full`) en su línea `image:`.

Las etiquetas de versión previa son baratas: `-beta.2`, `-beta.3` según haga
falta. Lo que no son es retirables - una etiqueta publicada sigue publicada.
Para deshacer un corte malo, corte el siguiente; nunca redirija una etiqueta.

## Cortar una versión estable

Los mismos pasos, con la etiqueta `vX.Y.Z` sin sufijo. Una cosa que comprobar y
que una versión previa no necesita: la versión mayor de Postgres que integra la
imagen debe constar en el changelog, porque un operador con la base de datos
integrada tiene que seguir la sección de actualización de
[installation.md](../installation.md) cuando cambia.

## Volver atrás una versión

Las imágenes se quedan en el registro. Un operador vuelve atrás con
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(el script de actualización imprime exactamente esa línea). No hay nada que
deshacer del lado de la publicación; publique un `-beta.N+1` corregido cuando
llegue la corrección.
