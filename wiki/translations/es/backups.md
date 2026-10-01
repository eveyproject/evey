---
source_sha: cd66a9194ddc930c849aec83b5d0a19bb86c2f982c6fcf9e5f4211e0a00e0cde
reviewed: false
---
# Copias de seguridad y actualizaciones

## Qué contiene una copia de seguridad

Una copia de seguridad es una copia de la **base de datos**: cuentas, perfiles,
bibliotecas y sus metadatos, historial y puntos de reanudación, colecciones y
listas de reproducción, ajustes, registros de plugins, webhooks y registros de
auditoría. Es un `.tar.gz` que contiene el volcado SQL y un manifiesto con el
SHA256 del volcado.

**No** contiene:

- tus archivos multimedia;
- la caché de transcodificación (se regenera bajo demanda);
- el índice de búsqueda (se reconstruye a partir de la base de datos);
- las claves guardadas en `./evey-data`: `jwt.key`, que firma las sesiones, y la
  carpeta `keys` (contraseña de la base de datos, clave de búsqueda);
- los archivos de los plugins instalados.

Una copia de la base de datos restaura por tanto el estado de Evey en la misma
instalación. Para poder reconstruir una máquina, guarda también una copia de toda
la carpeta `./evey-data`: las copias de la base de datos están dentro, en
`./evey-data/backups`.

## Copias programadas

**Admin → Copia de seguridad y restauración → Programación**: una expresión cron
(por ejemplo `0 3 * * *`, todos los días a las 3 de la madrugada) y una retención en días. Las
copias más antiguas que la retención se eliminan tras cada ejecución programada.
**Ejecutar ahora** crea una de inmediato.

<figure class="evey-shot" markdown="span">
  ![Admin → Copia de seguridad y restauración con una programación diaria](images/admin-backup.webp){ loading=lazy }
  <figcaption>Una copia todos los días a las 3 de la madrugada, conservada 30 días (interfaz en inglés).</figcaption>
</figure>

El servidor también vuelca la base de datos por su cuenta antes de aplicar una
migración durante una actualización, sea cual sea la programación (ver abajo).

## Restaurar

**Admin → Copia de seguridad y restauración** lista las copias y restaura una, o
restaura desde un archivo que subes. Restaurar sustituye la base de datos actual.

Una copia solo se puede restaurar en un servidor con el mismo esquema de base de
datos: el servidor comprueba primero la suma de control y la versión del esquema,
y rechaza una copia hecha por una versión con otro esquema en lugar de cargarla.
Para volver a una versión anterior con sus datos, consulta
[Volver atrás](#going-back) más abajo.

Cuando el servidor ya no arranca, la línea de comandos restaura los volcados
hechos antes de una migración (`pre-migration-*.sql.gz`), no las copias
`.tar.gz` de este panel: consulta [Recuperación](runbooks/recovery.md).

## Actualizar Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

Esa es toda la actualización. Cuando la nueva versión necesita cambiar la base de
datos, primero escribe un volcado en `./evey-data/backups/pre-migration-*` y
después aplica las migraciones al arrancar; un catálogo grande puede tardar unos
minutos, y el contenedor se declara sano en cuanto vuelve a servir. Evey
comprueba cada 6 horas si hay una imagen más reciente publicada y lo indica en el
panel de administración; nunca se actualiza solo.

**Fija lo que ejecutas.** `latest` sigue la versión estable más reciente. Para un
servidor del que dependes, pon una etiqueta de versión (`0.2.0`) o una etiqueta
inmutable `sha-<commit>` en la línea `image:`, para que dos descargas con un día
de diferencia ejecuten el mismo código. Una etiqueta de rama como `beta-0.0.1`
se mueve con cada cambio.

**Qué versión se ejecuta.** `docker exec evey-server evey version` muestra la
versión y el commit del binario en ejecución.

### Volver atrás

Pon la etiqueta anterior en la línea `image:` y ejecuta `docker compose up -d`.
Si la versión más reciente había migrado la base de datos, restaura primero su
volcado previo a la migración: `docker exec evey-server evey admin rollback`
muestra los pasos exactos con la ruta del volcado.

Las etiquetas de imagen, las variantes de ejecución y las variantes con GPU se
describen en [Despliegues avanzados](operations/advanced-deployments.md#image-tags).
