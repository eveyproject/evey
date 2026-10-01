---
source_sha: daa61595acb5937c7640f83d1baaec5b538968a8324e20be12058340e8e77c15
reviewed: false
---
# Bibliotecas y metadatos

Escaneo, fuentes de metadatos y detección de intros. La mayoría tiene un
equivalente en el panel de administración, que prevalece en cuanto se guarda ahí
un valor.

## Escaneo

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Vigilancia de archivos en tiempo real. |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | Cada cuánto se revisan las carpetas en busca de lo que la vigilancia no vio (recursos compartidos de red). **Admin → Configuración del servidor → Configuración del servidor** la anula. |
| `EVEY_LIBRARY_SCAN_WORKERS` | según la parte de CPU | Archivos analizados en paralelo durante un escaneo. Definida a mano, se salta la parte de **Potencia asignada** para los escaneos. |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Dónde guarda el escaneo de Salud de medios sus resultados y su tarea en curso entre reinicios. Vacía: no se guardan. |

## Metadatos

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | El idioma de los títulos y resúmenes. Prevalece el idioma elegido en el asistente de configuración o en **Admin → Configuración del servidor → Idioma y región**. |
| `EVEY_TMDB_API_KEY` | clave integrada | Tu propia clave de TMDB, para tu propia cuota. Prevalece la clave definida en **Admin → Configuración del servidor → Claves API**. |
| `EVEY_OMDB_API_KEY` | vacía | Una clave de OMDb, una segunda fuente para películas. Misma anulación. |
| `EVEY_OPENSUBTITLES_API_KEY` | vacía | Una clave de OpenSubtitles, para buscar subtítulos a través de la API. Misma anulación. |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | vacías | Una cuenta de OpenSubtitles, usada para iniciar sesión en su API. |

## Intro, resumen y créditos

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Usa las bases de datos comunitarias (TheIntroDB y después IntroDB) antes que la detección propia del servidor. Prevalece **Admin → Configuración del servidor → Proveedores de salto de intro**; reinicia tras cambiarla. |
| `EVEY_INTRO_TIDB_API_KEY` | vacía | Una clave de TheIntroDB: 500 consultas al día en lugar de 100. |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Todos los reproductores saltan solos las intros detectadas. Se aplica en caliente desde el panel. |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Ofrece también saltar el avance del siguiente episodio. Se aplica en caliente desde el panel. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | según la parte de CPU | Títulos analizados a la vez. Definida a mano, se salta la parte de **Potencia asignada** para esta tarea. |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Hilos por proceso ffmpeg durante la detección (de 1 a 16). |
