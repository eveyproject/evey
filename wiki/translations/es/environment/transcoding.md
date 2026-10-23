---
source_sha: 11f247e9600d2a9954d2efb9c3e83b1b5c383eeff81aea6c581b95520eafccc1
reviewed: true
---
# Transcodificación y hardware { #transcoding-and-hardware }

Las variables que hay detrás de la capacidad, la tarjeta gráfica, los límites
de calidad y la caché de transcodificación. La mayoría de las instalaciones
no define ninguna: el servidor mide la máquina y elige el hardware por sí
mismo, como se explica en
[Transcodificación y capacidad del servidor](../playback/transcoding.md).
Cuando un ajuste existe en **Administración → Transcodificación**, tiene
prioridad el valor guardado ahí.

## Aceleración por hardware { #hardware-acceleration }

Cómo dar acceso a la GPU al contenedor:
[Aceleración por hardware](../administration/hardware-acceleration.md).

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | vacío | `auto`, `off`, `nvenc`, `vaapi`, `qsv` o `rkmpp`. Vacío: se deduce de las dos variables siguientes. | Transcodificación → **Aceleración por hardware**, inmediato (vuelve a medir) |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | El antiguo interruptor de activado/desactivado. | Transcodificación → **Aceleración por hardware**, inmediato |
| `EVEY_TRANSCODING_HW_BACKEND` | vacío (detección) | Fija un solo mecanismo (`nvenc`, `vaapi`, `qsv`): solo se mide ese. Permite que varios contenedores Evey en un mismo equipo usen cada uno una vía de GPU distinta. | ninguno |
| `EVEY_TRANSCODING_HW_DEVICE` | vacío (todos) | Fija un solo dispositivo: un nodo de render (`/dev/dri/renderD129`) o un índice CUDA (`1`). Con `EVEY_TRANSCODING_HW_BACKEND`, asocia un contenedor a una GPU en un equipo con varias GPU. | ninguno |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Una sesión por hardware que no ha producido su primer segmento en este plazo se trata como una GPU bloqueada y pasa al procesador. | ninguno |
| `EVEY_TRANSCODING_HW_TONEMAP` | activado | `0` desactiva el mapeo de tonos HDR en la GPU y lo hace con el procesador, para un controlador que se sabe que funciona mal. | ninguno |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Un ffmpeg por calidad en la vía de GPU en lugar de un solo proceso para todas: aísla los fallos y evita el grafo de varias salidas que hace fallar los controladores VA poco estables. | Transcodificación → Límites avanzados → **Un proceso de codificación por hardware por peldaño**, al reiniciar |

El codificador Intel de bajo consumo y la salida AV1 (interruptor y tope) no
tienen variable: se ajustan solo en **Administración → Transcodificación**.

## Capacidad y límites { #capacity-and-limits }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (número de CPU) | Total de sesiones de transcodificación simultáneas. En una máquina que apenas puede hacer una transcodificación 4K HDR por software, pon `1` o `2`. | Transcodificación → Límites avanzados → **Sesiones HLS simultáneas máximas (vacío = sin límite)**, al reiniciar |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Tareas de transcodificación fuera de la reproducción en directo. | Caché de transcodificación → **Máximo simultáneo**, inmediato |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Codifica solo la calidad que está viendo un espectador. `false` comparte una sola decodificación entre todas las calidades, pero codifica las que nadie ve; solo para un equipo sin decodificación por hardware y con fuentes pesadas. | Transcodificación → Límites avanzados → **Codificar un peldaño a la vez**, al reiniciar |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (medido) | Impone el tope de resolución del procesador en lugar del medido. | Transcodificación → Límites avanzados → **Tope de altura en transcodificación por software (vacío = sin tope)**, al reiniciar |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (medido) | Lo mismo, para las fuentes HDR cuyo mapeo de tonos hace el procesador. | Transcodificación → Límites avanzados → **Tope de altura HDR por software (vacío = sin tope)**, al reiniciar |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | sin tope | Hilos por codificador por software (de 1 a 64; cualquier otro valor significa sin tope). Los codificadores por hardware la ignoran. | ninguno |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (sigue a la fuente) | Un tope para el bitrate del vídeo transcodificado. | Transcodificación → **Techo de bitrate de vídeo**, inmediato |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (ninguno) | El mismo tope para los clientes de fuera de la red de casa. | Transcodificación → **Techo de bitrate de vídeo, espectadores remotos**, inmediato |

## Caché { #cache }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | El tamaño de toda la caché de transcodificación (segmentos, remux, descargas preparadas); lo usado hace más tiempo se borra primero. | Transcodificación → **Presupuesto de caché de transcodificación (GB)**, al reiniciar |
| `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` | `2048` | Espacio libre que se reserva en el disco de la caché. Por debajo, se expulsan los segmentos inactivos y después se rechazan los segmentos nuevos en lugar de escribirlos; los subtítulos extraídos dejan de guardarse y se avisa a los administradores. `0` desactiva este margen. Consulta [La caché de transcodificación](../playback/transcoding.md#the-transcoding-cache). | ninguno |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Dónde está. La define la imagen. | ninguno |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Dónde están las miniaturas de la barra de progreso. La define la imagen: cuesta mucho regenerarlas, así que se quedan en `/data`. | ninguno |

## Clip de calibración { #calibration-clip }

La medición codifica un clip 4K. Por orden de preferencia: tu archivo, un
clip descargado una vez y comprobado por su hash, o un clip sintético. Un
clip real mide de forma más fiel que el sintético. Ninguna de estas variables
tiene equivalente en el panel.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | vacío | Un clip tuyo, HDR y SDR. | ninguno |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | vacío | Un clip que se descarga una vez, por `https`. | ninguno |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | vacío | El SHA256 de esa descarga; obligatorio con una URL. | ninguno |

<!-- sources
cmd/evey/wire.go - EVEY_TRANSCODE_CACHE_MIN_FREE_MB (read at boot, not in config.go) ; internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SingleVariant, SoftwareMaxHeightOverride, SoftwareAV1MaxHeightOverride (panel-only), AV1Enabled, AV1MaxHeight, HWLowPowerEnabled
panel overrides: cmd/evey/main.go - loadOverride (transcoding.*) ; cmd/evey/wire.go (transcoding.hw_mode read directly) ; internal/adapters/driving/http/settings_handler.go
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: frontend/apps/web/src/routes/_auth/admin/transcode-cache.lazy.tsx (transcoding.max_concurrent) ; i18n admin.transcodeCache.maxConcurrent
labels: i18n admin.transcoding.advancedTitle, maxSessions, softwareMaxHeight, softwareHDRMaxHeight, singleVariant, hwPerVariant, hwModeLabel, maxBitrateLabel, maxBitrateRemoteLabel ; admin.serverSettings.transcodeCacheMaxGB
-->
