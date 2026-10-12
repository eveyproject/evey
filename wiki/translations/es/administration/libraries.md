---
source_sha: ed64eaf04c09d740775060639c8ba122fb90d8ee17950d66a387a572c3fea16d
reviewed: true
---
# Bibliotecas y metadatos

Una **biblioteca** es un tipo de contenido (películas, series, música o
fotos) guardado en una o varias carpetas. Evey lee las carpetas, reconoce
cada archivo por su nombre y busca sus datos en internet: título, resumen,
póster, reparto, valoraciones. Estos datos se llaman **metadatos**. Para las
películas y las series vienen de The Movie Database (TMDB), una base de datos
de cine gratuita en internet.

Lo primero que harás está al principio de esta página: añadir una
biblioteca, nombrar tus archivos y corregir un título mal reconocido.
Después vienen cómo funcionan los escaneos, la música y las fotos, las
colecciones y las fuentes de metadatos.

<figure class="evey-shot" markdown="span">
  ![Administración → Bibliotecas en el servidor de demostración](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Administración → Bibliotecas</strong>: una fila por biblioteca, con sus carpetas, su número de elementos y su último escaneo (interfaz en inglés).</figcaption>
</figure>

## Añadir una biblioteca

1. **Administración → Bibliotecas → Añadir biblioteca**.
2. Ponle un **Nombre** y un **Tipo**. Guarda las películas y las series en
   bibliotecas separadas: el tipo le dice al escáner qué tiene que buscar.
3. En **Rutas**, elige una carpeta de la lista, o **Introducir ruta
   personalizada...** para escribirla; **+ Añadir ruta** añade una segunda
   carpeta a la misma biblioteca.
4. Pulsa **Crear biblioteca**. Evey lee las carpetas en un par de minutos;
   pulsa **Escanear** en la fila de la biblioteca para empezar en el
   momento. Los pósteres y los resúmenes llegan en segundo plano, así que
   una biblioteca grande se va completando en unos minutos.

| Tipo | Se reconoce como | Metadatos de |
| --- | --- | --- |
| **Películas** | Una película por archivo (o por carpeta) | TMDB, completado con OMDb si hay una clave |
| **Series de TV** | Series, temporadas, episodios | TMDB |
| **Música** | Artistas, álbumes, pistas | Las etiquetas de los archivos; portadas de los archivos, de la carpeta del álbum, de Cover Art Archive (MusicBrainz) y, por último, de iTunes |
| **Fotos** | Álbumes, fotos con sus datos de toma | Los archivos |

!!! note "Las carpetas se nombran como las ve Evey"
    La lista de carpetas muestra las carpetas tal como las ve Evey, dentro
    de su contenedor. Con el archivo compose de la
    [instalación](../installation.md), tu carpeta de contenido es `/media`.
    Así, una carpeta `/srv/media/films` de tu ordenador es `/media/films` en
    Evey. Una segunda carpeta de contenido (`/media2`) funciona igual.

## Nombrar los archivos para que se reconozcan

El escáner reconoce nombres como estos:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

- **Para una película**, lo más importante es el año: `The Mirror (1999)`.
- **Para un episodio**, el número de temporada y de episodio: `S01E05` o
  `1x05`.
- Las palabras de más en el nombre se ignoran: resolución, origen, códec,
  audio, etiquetas de idioma como `VOSTFR` o `MULTI`, ediciones como
  "Director's Cut".

Si tus archivos llevan una palabra que Evey todavía no ignora, añádela en
**Administración → Configuración del servidor → Tokens del analizador**:
**Tokens de calidad adicionales** para palabras sueltas (`HDRIP`), **Sufijos
de edición adicionales** para frases. Se aplica a partir del siguiente
escaneo.

## Corregir un título equivocado o que falta

¿Una película muestra el póster de otra película con el mismo nombre? La
forma más rápida de arreglarlo:

1. Abre el título y elige **Editar metadatos**.
2. Pulsa **Identificar en TMDB** y busca el título correcto.
3. Pulsa **Usar esta coincidencia**.

Otras opciones de **Editar metadatos**:

- **Cambiar un campo a mano.** Un valor que cambias se conserva: las
  actualizaciones posteriores no lo sobrescriben. Si cambias el **ID TMDB**
  o el **ID IMDB**, al guardar se descargan los datos de ese identificador.
- **Actualizar metadatos** vuelve a descargar los datos de la coincidencia
  actual.
- **Restablecer metadatos** y **Re-emparejar metadatos** vuelven a empezar
  desde el nombre del archivo y descartan tus cambios. Restablecer también
  quita las imágenes y el reparto, y las temporadas y los episodios de una
  serie.

Para una biblioteca entera, su fila en **Administración → Bibliotecas**
ofrece:

- **Vincular solo huérfanos** vuelve a intentarlo con los archivos que nunca
  se reconocieron. Es rápido.
- **Volver a vincular todo con TMDB** vuelve a buscar cada título. Es lento.
  Úsalo después de renombrar muchos archivos.
- **Reparar sagas** vuelve a unir a su saga las películas que no están en
  ninguna colección, cuando las otras películas de la saga ya están en la
  biblioteca.

## Cómo se detectan los archivos nuevos

No tienes que hacer nada: un archivo nuevo, renombrado o borrado aparece en
Evey en unos minutos.

Para forzarlo, o para añadir un escaneo completo regular, usa la fila de la
biblioteca:

| Acción | Qué hace |
| --- | --- |
| **Escanear** | Lee las carpetas ahora. La fila muestra **Escaneando...** y después el resultado. Si una carpeta no se ha podido leer, aparece su nombre. |
| **Programar** | Un escaneo completo **Cada 6 horas**, **Cada 12 horas**, **Diario (3 AM)**, **Semanal (domingo 3 AM)**, o según una **Expresión cron**. |

??? info "Cómo revisa el servidor las carpetas"

    - El servidor lee todas las bibliotecas cada vez que arranca, y después
      cada 2 minutos (**Administración → Configuración del servidor →
      Configuración del servidor → Intervalo de escaneo periódico**, se
      aplica al reiniciar). Mientras se reproduce algo, espera 15 minutos
      entre lecturas para no ralentizar la reproducción.
    - El sistema también puede avisar al servidor cuando cambia un archivo
      (**Administración → Configuración del servidor → Configuración del
      servidor → Monitor de archivos de biblioteca**), en las bibliotecas que
      tienen activada la supervisión en tiempo real. Para comprobar que esto
      sigue funcionando, escribe un pequeño archivo de prueba en la carpeta
      durante unos segundos. Una biblioteca en la que se ha comprobado que
      funciona se lee una vez por hora en lugar de cada 2 minutos.
    - Las bibliotecas creadas desde el panel no usan el monitor de archivos:
      las lecturas cada 2 minutos encuentran sus archivos.

!!! note "Carpetas compartidas en red"
    En una carpeta compartida NFS o SMB, un archivo escrito por otra máquina
    no envía ninguna señal al servidor. Las lecturas regulares lo encuentran.

## Bibliotecas de música

El escáner lee archivos MP3, FLAC, M4A, OGG, Opus, WAV y AAC, y toma los
datos de cada pista de sus etiquetas: título (el nombre del archivo si
falta), artista, artista del álbum, álbum, año, número de pista y de disco,
género. Los artistas se agrupan por artista del álbum, o por artista cuando
esa etiqueta está vacía.

**Un álbum es una carpeta.** Guarda cada álbum en una sola carpeta. Evey
agrupa las pistas por la carpeta en la que están, no por su etiqueta de
álbum. Así, dos carpetas con la misma etiqueta de álbum dan dos álbumes, y un
álbum repartido en subcarpetas `CD1` y `CD2` aparece como dos. Dentro de un
álbum, las pistas se ordenan por número de disco y después por número de
pista, y el álbum toma el nombre que lleva la mayoría de sus pistas.

La portada es la imagen incluida en los archivos; si no hay, una imagen de
la carpeta del álbum (`cover.jpg`, `folder.jpg` o `front.jpg`); si no,
Cover Art Archive (encontrada a través de MusicBrainz); y si no, iTunes.

## Bibliotecas de fotos

El escáner lee archivos JPEG, PNG, HEIC, WebP, GIF, TIFF y BMP. El álbum de
una foto es su carpeta, relativa a la carpeta de la biblioteca; una foto que
está en el primer nivel no tiene álbum, y si mueves un archivo a otra
carpeta, pasa a ese álbum en el siguiente escaneo. La fecha de la toma y el
modelo de cámara salen de los datos EXIF de los archivos JPEG; los demás
formatos aparecen sin ellos. Las miniaturas se redimensionan en el servidor
y se guardan en la caché de transcodificación.

## Colecciones

**Colecciones** reúne títulos bajo un mismo nombre, para todas las cuentas.
Hay tres tipos:

- **Colecciones TMDB**: las sagas, que se rellenan solas cuando TMDB coloca
  una película en una. **Reparar sagas** (más arriba) recupera las películas
  que se quedaron fuera de la suya.
- **Nueva colección**: una lista que un administrador rellena a mano, con
  **Añadir a colección** en un título o en una selección, y que ordena.
- **Nueva colección inteligente**: reglas sobre **Género**, **Año**,
  **Tipo**, **Valoración ≥** y **Biblioteca**, que un título tiene que
  cumplir todas. Su contenido se calcula cada vez que se abre, así que los
  títulos nuevos aparecen sin hacer nada.

Solo los administradores crean y editan colecciones. Cualquiera puede
verlas, y **Descargar colección** en la web descarga los archivos originales
en un solo ZIP (hasta 1000 títulos), sin lo que ocultan el acceso a
bibliotecas o el límite de edad de la cuenta.

## Claves de metadatos

Evey trae una clave de TMDB, así que los metadatos funcionan sin configurar
nada. Las otras fuentes son opcionales. Para usarlas, consigues una clave
(una especie de contraseña) de ese servicio y la pegas en
**Administración → Configuración del servidor → Claves API**. Una clave
puesta ahí tiene prioridad sobre la misma clave dada en el entorno (ver
[Entorno: bibliotecas y metadatos](../environment/libraries.md)).

| Clave | Añade |
| --- | --- |
| TMDB (la tuya) | Tu propia cuota de peticiones en lugar de la compartida. |
| OMDb | Una segunda fuente para las películas de las que TMDB sabe poco. |
| OpenSubtitles | Buscar y descargar subtítulos que un archivo no trae, a través de la API del servidor (las apps todavía no lo ofrecen). Ver [Subtítulos](../playback/subtitles.md). |
| TheIntroDB | Los tiempos de intro, resumen y créditos de una base de datos comunitaria: 500 consultas al día con clave en lugar de 100 sin ella. |

## Intro, resumen y créditos

Evey encuentra la intro, el resumen y los créditos de los episodios y las
películas, para que los reproductores ofrezcan saltarlos. Los tiempos vienen
de bases de datos comunitarias (TheIntroDB y después IntroDB) cuando
conocen el título. Si no, el servidor los calcula analizando los archivos.
Un administrador puede ajustarlos a mano para un título, o volver a lanzar
la detección.

Los reproductores muestran por defecto los botones **Saltar intro**,
**Saltar resumen** y **Saltar créditos**. **Saltar intros automáticamente**
(**Administración → Configuración del servidor → Configuración del
servidor**, desactivado por defecto) hace que todos los reproductores las
salten solos.

<!-- sources
add library: frontend/apps/web/src/features/admin/LibraryForm.tsx (name, type, paths only - no realtime/scan-on-startup toggles) ; i18n admin.libraries.addLibrary, tvShows, paths, enterCustomPath, addPath, createLibrary ; internal/domain/library/model.go NewLibrary (RealtimeMonitoring false by default) ; grpcapi/library_handler.go CreateLibrary (no scan trigger ; only watcher + feed subscribe library.created)
types: i18n admin.libraries.types.* ; internal/adapters/driven/metadata/musicbrainz_provider.go ; internal/app/music/album_art_chain.go
parser: internal/adapters/driven/filesystem/parser.go (reEditionSuffix, knownQualities, SetExtraParserTokens) ; internal/app/library/parser_config.go ApplyParserConfig ; i18n admin.serverConfig.parserTokens*
fix a title: frontend/apps/web/src/features/library/MediaDetailEdit.tsx (save, refresh, reset, rematch, identify) ; IdentifyMetadataModal.tsx ; http/admin_handler_media.go (identify search) ; internal/app/metadata/command_service_overrides.go SetOverride, ResetItemMetadata (overrides, artwork, persons, Catalog=nil, children) ; command_service_refresh.go RefreshItemMetadata ; i18n media.detail.editMetadata, identify.*, tmdbId, imdbId, idChangedWarning, refreshMetadata, resetMetadata, rematchMetadata, rematchConfirm
per library: i18n admin.libraries.matchOrphans, refreshMetadata, relinkCollections (+Tooltip) ; internal/app/metadata/relink_collections.go
scans: internal/app/library/command_service.go StartPeriodicScan (startup pass, ticker, runScanPass) ; scan_service.go playbackScanBackoff 15m, watchedScanInterval 1h, shouldScanNow ; cmd/evey/wire_events.go (library.periodic_scan_interval, all libraries) ; internal/config/config.go EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL 2m ; i18n admin.serverSettings.periodicScanInterval, restartHint, libraryWatcher ; admin.libraries.scan, scanning, scanPartial, schedule, presets.* ; routes/_auth/admin/libraries.lazy.tsx PRESET_SCHEDULES ; internal/app/library/scheduler.go
watcher: internal/app/library/watcher.go Start (RealtimeMonitoring && Enabled) ; watcher_probe.go probeOnce, UnhealthyLibraries (network share blind spot) ; cmd/evey/wire_helpers.go (WatcherEnabled) ; command_service.go WithWatchHealth
music: internal/adapters/driven/filesystem/music_scanner.go (audioExts) ; internal/app/music/command_service.go - parseTrack (AlbumFolder = parent dir, title fallback) ; postgres/music_track_repo.go - GetAlbums GROUP BY album_folder, mode() display_album, ORDER BY disk_number, track_number ; GetArtists COALESCE(album_artist, artist)
album art order: internal/app/music/album_art_folder.go (folderCoverNames, embedded wins) ; cmd/evey/wire_services.go (musicbrainz then itunes)
photos: internal/adapters/driven/filesystem/photo_scanner.go (imageExts) ; internal/app/photo/command_service.go - albumOf, processOnePhoto (EXIF JPEG only) ; http/photo_handler.go - serveTranscoded (cache photos/)
collections: internal/domain/library/model.go Collection (TmdbID, Rules), MaxCollectionItems 1000 ; internal/adapters/driven/postgres/collection_repo.go (genre, type, year, library_id, rating_gte, AND) ; internal/app/collection/query_service.go GetItems ; interceptors/rbac.go CollectionService ; http/collection_handler.go (library access + parental filter) ; i18n media.collections.*, media.library.addToCollection, media.playlist.field*
keys: i18n admin.serverConfig.apiKeys.* ; admin.serverConfig.introProvidersTidbAuthed ; cmd/evey/wire_infra.go resolveAPIKeyWithBuiltin
opensubtitles: internal/app/subtitle/service.go
intro providers: docs/adr/0011-intro-detection-providers.md ; i18n admin.serverSettings.introAutoSkip
labels: i18n admin.libraries.scan, createLibrary, addPath ; media.detail refreshMetadata, resetMetadata, rematchMetadata ; media identify openButton, useThis
-->
