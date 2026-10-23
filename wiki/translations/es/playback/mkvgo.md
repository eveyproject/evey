---
source_sha: 9be0f9fe12d2d5606cefd03463fe0dd43f1000f645afe396700673ac2c53dcea
reviewed: true
---
# MKVGo en Evey { #mkvgo-in-evey }

**En resumen:**

- MKVGo es una parte de Evey que lee y reempaqueta archivos de vídeo (MKV,
  WebM, MP4, MOV) sin convertir la imagen ni el sonido.
- Cuando puede, es lo que sirve las reproducciones en **Remux**, extrae los
  subtítulos y lee tus archivos durante un escaneo. Todo lo que necesita una
  conversión de verdad va a FFmpeg, una herramienta de vídeo aparte.
- No tienes nada que configurar. **Administración → Reproduciendo ahora**
  muestra, pista por pista, si una reproducción la ha servido MKVGo o FFmpeg.

El resto de esta página es para quien quiera saber qué hace el servidor sin
arrancar FFmpeg, y en qué casos sigue interviniendo FFmpeg. MKVGo funciona
dentro del proceso del servidor Evey.

MKVGo trabaja con la estructura de un archivo: sus cabeceras, su índice, sus
pistas y sus paquetes. Nunca descodifica ni codifica una imagen o un sonido;
todo lo que necesita descodificarse en el servidor va a FFmpeg. Se desarrolla
como una biblioteca Go aparte:
[github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo).

## Tres niveles de trabajo { #three-levels-of-work }

| Nivel | Lo que pasa | Lo que le cuesta al servidor |
| --- | --- | --- |
| **Reproducción directa** | El archivo se sirve tal cual. | Leer un archivo. |
| **MKVGo** | Trabajo de estructura: leer un archivo, reempaquetar sus pistas para el streaming, reconstruir su índice, mover una pista en la línea de tiempo. | Código dentro del proceso del servidor: no se arranca FFmpeg ni ffprobe por cada flujo, y no se escriben segmentos en disco para el empaquetado bajo demanda. |
| **FFmpeg** | Transformaciones de verdad: volver a codificar, convertir HDR a SDR (tone-mapping), descodificar una imagen. También es el recurso de reserva cada vez que MKVGo no puede procesar un archivo. | Un proceso por tarea. |

Una copia con FFmpeg (`-c copy`) tampoco vuelve a codificar nada y sigue
siendo barata; lo que MKVGo ahorra además es el proceso que se arranca para
cada sesión y los segmentos escritos en la caché de transcodificación. Volver
a codificar es el caso caro, y volver a codificar el vídeo es el único
trabajo que ocupa una plaza de transcodificación: consulta
[Transcodificación y capacidad del servidor](transcoding.md).

El inicio y los saltos no esperan a un proceso ni a un primer segmento en
disco: el segmento se construye cuando se pide, a partir del índice del
archivo. Que un archivo vaya por este camino depende de lo que descodifica el
reproductor: [Cómo elige Evey el modo de reproducción](how-playback-is-decided.md).

## Lo que hace MKVGo { #what-mkvgo-does }

| Trabajo | Dónde se nota | Cuando MKVGo no puede |
| --- | --- | --- |
| **Leer archivos** | Durante un escaneo de la biblioteca, los códecs, la resolución, el HDR, el bitrate y la duración se leen del principio del archivo. | Lo lee ffprobe. También se usa ante cualquier duda: un error de lectura, o información de color que MKVGo no ha podido leer en un códec que admite HDR. |
| **Índice de fotogramas clave** | Las posiciones desde las que puede empezar la reproducción, leídas del índice del archivo; permiten que el Remux y la copia de vídeo corten los segmentos en fotogramas clave. | ffprobe. |
| **Streaming sin volver a codificar** | Remux: segmentos HLS en MP4 fragmentado (CMAF), construidos cuando se piden a partir de un plan guardado para cada archivo. Un segmento se escribe en la conexión directamente desde el archivo de origen a medida que sale: no se monta nada antes en memoria, no se escribe nada en la caché de transcodificación. Sobre los mismos segmentos se genera un manifiesto DASH; las apps usan HLS. | FFmpeg copia las pistas, también sin volver a codificar, en segmentos escritos en la caché. |
| **MP4 para el navegador** | Un archivo Matroska que un navegador no puede abrir se reescribe una sola vez como un MP4 de inicio rápido, guardado en una caché. | - |
| **Subtítulos** | Las pistas de subtítulos incluidas en un MKV se extraen cuando se piden: las de texto a WebVTT y las de imagen (PGS) a imágenes. Cuando el propio índice del archivo (Cues) cubre una pista entera, el servidor lee solo los bloques a los que apunta en lugar de todo el archivo, y guarda ese índice de bloques en su carpeta de datos (`subtitle-index/`) para la próxima petición. A una pista que todavía necesita una lectura completa se le responde «en preparación» con un plazo para volver a intentarlo, y los reproductores vuelven a preguntar hasta que está lista. | ffmpeg extrae la pista leyendo el archivo de principio a fin. |
| **Imágenes de vista previa** | El fotograma clave más cercano a la posición pedida se encuentra gracias al índice, y FFmpeg descodifica solo ese fotograma. Se usa cuando ese fotograma clave está a menos de unos dos segundos de lo pedido. | FFmpeg salta y descodifica hasta la posición exacta. |
| **Cadenas de episodios** | El servidor puede empaquetar varios episodios como un solo flujo continuo (`?gapless=1` en la dirección del flujo). Ninguna app ofrece todavía esta opción. | Cada episodio se reproduce por separado. |
| **Salud de medios** | El diagnóstico (índice de posicionamiento, inicio del audio, pistas guardadas en bloques), el índice reconstruido, la resincronización después de un tramo dañado, y el desplazamiento de una pista de audio. Consulta [Salud de medios](../runbooks/media-health.md). | Reorganizar las pistas guardadas en bloques lo hace FFmpeg. |

## Lo que sigue haciendo FFmpeg { #what-stays-with-ffmpeg }

- Cualquier recodificación, incluida la conversión de HDR a SDR
  (tone-mapping).
- Descodificar imágenes: las miniaturas de la barra de avance, el fotograma
  de una imagen de vista previa.
- Los análisis que necesitan datos descodificados: detectar el entrelazado,
  las huellas de audio para detectar las intros.
- Los contenedores que no son MKV/WebM ni MP4/MOV, como AVI o MPEG-TS.

## Carga, archivos sustituidos y actualizaciones { #load-replaced-files-and-updates }

**Cuánto lee a la vez.** Para construir un segmento se lee una ventana del
archivo de origen, vídeo y audio juntos: de 6 a 10 MiB para un archivo
1080p, unos 15 MiB para un remux UHD. El servidor lee a la vez tantas
ventanas como procesadores tiene, y menos en una máquina con poca memoria:
todas las ventanas en curso deben caber en una cuarta parte de la memoria que
el servidor se permite usar (la mitad del límite del contenedor, o de la
memoria del equipo). Las peticiones que pasan de ahí esperan a que quede un
hueco libre, así que la memoria usada no crece con el número de
espectadores; en un equipo pequeño, si mucha gente empieza a la vez, ve
llegar sus primeros segmentos uno tras otro, en lugar de que el servidor se
quede sin memoria.

**Un archivo sustituido durante la reproducción.** Cada dirección de segmento
indica el archivo a partir del cual se planificó. Cuando un archivo se
sustituye en el disco (una copia mejor, una reparación), un segmento que se
pide en ese momento se reconstruye una vez a partir del archivo nuevo, y las
direcciones de la lista de reproducción del archivo antiguo responden
`410 Gone` en lugar de mezclar los dos archivos; al volver a abrir el título
se reproduce el nuevo.

**Después de una actualización.** Las direcciones de los segmentos llevan la
versión de MKVGo, así que la caché de un navegador o de un proxy nunca sirve
un segmento construido por una versión anterior, y no hay ningún segmento de
este camino guardado en disco que haya que vaciar. Las copias MP4 hechas para
los navegadores llevan en su nombre la versión de su formato: cuando cambia,
cada una se reconstruye la próxima vez que se pide.

**Vigilarlo.** En `GET /metrics` (consulta
[Supervisión y registros](../operations/monitoring.md#prometheus-metrics)),
`evey_mkvgo_walks_in_flight` es el número de ventanas que se están leyendo en
este momento, y `evey_mkvgo_plan_stats` suma los contadores del empaquetador
por `stat`. Si `walks_in_flight` se queda en su techo mientras
`stat="slot_waits"` sube, los espectadores están haciendo cola para el
procesador; `stream_fallbacks` cuenta los segmentos que han tenido que
montarse en memoria en lugar de enviarse sobre la marcha.

## Ver qué motor ha servido un flujo { #seeing-which-engine-served-a-stream }

**Administración → Panel** y **Administración → Reproduciendo ahora** indican
el motor de cada pista: `mkvgo` o `ffmpeg`. Un Remux puede mostrar los dos,
por ejemplo el vídeo empaquetado por MKVGo y el audio copiado por FFmpeg.

mkvgo se publica bajo la licencia MIT.

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
streamed segments: internal/app/transcoding/ondemand.go - OnDemandResource (Bytes nil, WriteRange) ; internal/adapters/driving/http/transcode_ondemand_stream.go - servePlanResourceStreamed
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driving/http/subtitle_handler.go - extractSubtitleVTT ; subtitle_pgs.go ; subtitle_index_cache.go - fromCues, resolveAndStore, newSubtitleIndexCache ; subtitle_preparing.go (202 + Retry-After) ; subtitle_prewarm.go - ensureIndex
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
reads at once: internal/adapters/driven/mkvgo/budget.go - BudgetFor (one walk per CPU, walks within memLimit/4 at 15 MiB each) ; cmd/evey/wire_infra.go (applied after the memory limit) ; cmd/evey/memlimit.go - applyMemoryLimit (half the cgroup or host budget)
replaced file: internal/adapters/driving/http/transcode_ondemand.go - planSourceParam, servePlanSourceGone (410), replay once on a rebuilt plan ; internal/adapters/driving/http/transcode_vod_manifest_rebake.go - stampPlanSourceURIs
after an update: packager version in segment URLs (?v=) ; internal/adapters/driving/http/remux_handler.go - remuxCacheFormat, remuxCacheName
metrics: internal/pkg/metrics/metrics.go - MkvgoWalksInFlight, MkvgoPlanStats ; cmd/evey/wire.go - startMkvgoMetrics
plain opening: rules below ; i18n admin.sessions.track.engine
-->
