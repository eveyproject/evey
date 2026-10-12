---
source_sha: 0bcab9ec2d5eb546c48ee32a751bbd41320d9cb2c60cab549b010ec73e26f367
reviewed: true
---
# Bibliotecas y metadatos

Las variables que hay detrás del escaneo, de las fuentes de metadatos y de la
detección de intros. La mayoría tiene un campo en el panel de administración,
que tiene prioridad en cuanto se guarda un valor ahí; la parte del panel se describe en
[Bibliotecas y metadatos](../administration/libraries.md).

## Escaneo

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Vigilancia de los archivos en tiempo real. | Configuración del servidor → Ajustes del servidor → **Monitor de archivos de biblioteca**, al reiniciar |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | Cada cuánto se revisan las carpetas en busca de lo que la vigilancia no detectó (carpetas compartidas por red). | Configuración del servidor → Ajustes del servidor → **Intervalo de escaneo periódico**, al reiniciar |
| `EVEY_LIBRARY_SCAN_WORKERS` | según la parte de CPU | Archivos analizados en paralelo durante un escaneo. Si la defines a mano, no se aplica la parte de **Potencia asignada** para los escaneos. | ninguno |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Dónde guarda el escaneo de Salud de medios sus resultados y su tarea en curso entre reinicios. Vacía: no se guardan. | ninguno |

## Metadatos

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | El idioma de los títulos y los resúmenes. El idioma elegido en el asistente de configuración tiene prioridad sobre ella. | Configuración del servidor → Idioma y región → **Idioma de metadatos**, inmediato |
| `EVEY_TMDB_API_KEY` | clave integrada | Tu propia clave TMDB, para tener tu propia cuota. | Configuración del servidor → Claves API → **Clave API de TMDB**, al reiniciar |
| `EVEY_OMDB_API_KEY` | vacío | Una clave OMDb, una segunda fuente para las películas. | Configuración del servidor → Claves API → **Clave API de OMDB**, al reiniciar |
| `EVEY_OPENSUBTITLES_API_KEY` | vacío | Una clave OpenSubtitles, para buscar subtítulos a través de su API. | Configuración del servidor → Claves API → **Clave API de OpenSubtitles**, al reiniciar |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | vacío | Una cuenta de OpenSubtitles, usada para iniciar sesión en su API. | ninguno |

## Intro, resumen y créditos

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Usa las bases de datos de la comunidad (TheIntroDB y después IntroDB) antes de la detección propia del servidor. | Configuración del servidor → Proveedores de salto de intro → **Activar proveedores externos de salto de intro**, al reiniciar |
| `EVEY_INTRO_TIDB_API_KEY` | vacío | Una clave TheIntroDB: 500 consultas al día en lugar de 100. | Configuración del servidor → Claves API → **Clave API de TheIntroDB**, al reiniciar |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Todos los reproductores se saltan solos las intros detectadas. | Configuración del servidor → Ajustes del servidor → **Saltar intros automáticamente**, inmediato |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | También ofrece saltar el avance del episodio siguiente. | Configuración del servidor → Ajustes del servidor → **Saltar segmentos de avance**, inmediato |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | según la parte de CPU | Títulos analizados a la vez. Si la defines a mano, no se aplica la parte de **Potencia asignada** para esta tarea. | ninguno |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Hilos por proceso ffmpeg durante la detección (de 1 a 16). | ninguno |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
watcher / scan interval: cmd/evey/wire_events.go (loadOverride library.watcher_enabled, library.periodic_scan_interval) ; internal/adapters/driving/http/settings_handler.go (requiresRestart)
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin, resolveAPIKey ; cmd/evey/wire_services.go (opensubtitles) ; i18n admin.serverConfig.apiKeys.*
language: cmd/evey/wire_infra.go (metadata.language wins) ; internal/app/config/locale_defaults.go
intro: cmd/evey/wire.go (intro_providers.enabled, api.tidb_key, EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go ; i18n admin.serverConfig.introProvidersRestart ; admin.serverSettings.introAutoSkip, skipPreview
-->
