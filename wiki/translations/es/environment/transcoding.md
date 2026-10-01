---
source_sha: 943149aa81fcdc21ea4425ec9e43d282740de164c026d29cc366577b71c0fd72
reviewed: false
---
# Transcodificación y hardware

Capacidad, tarjeta gráfica, límites de calidad y caché de transcodificación. La
mayoría de las instalaciones no definen ninguna: el servidor mide la máquina y
elige el hardware por sí solo (consulta [Transcodificación y capacidad del servidor](../playback/transcoding.md)).
Cuando un ajuste existe en **Admin → Transcodificación**, prevalece el valor
guardado ahí; la tabla lo indica.

## Aceleración por hardware

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | vacía | `auto`, `off`, `nvenc`, `vaapi`, `qsv` o `rkmpp`. Vacía: se deduce de las dos variables siguientes. La anula **Aceleración por hardware** en el panel, que vuelve a medir en caliente. |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | El antiguo interruptor de activado/desactivado. Lo anula el panel. |
| `EVEY_TRANSCODING_HW_BACKEND` | vacía (detectar) | Fija un backend (`nvenc`, `vaapi`, `qsv`): solo se mide ese. Permite que varios contenedores Evey en un mismo equipo usen cada uno una vía de GPU distinta. |
| `EVEY_TRANSCODING_HW_DEVICE` | vacía (todos) | Fija un dispositivo: un nodo de render (`/dev/dri/renderD129`) o un índice CUDA (`1`). Con `EVEY_TRANSCODING_HW_BACKEND`, asocia un contenedor a una GPU en un equipo con varias GPU. |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Una sesión por hardware que no ha producido su primer segmento en este plazo se trata como una GPU atascada y pasa al procesador. |
| `EVEY_TRANSCODING_HW_TONEMAP` | activado | `0` desactiva el mapeo de tonos HDR en la GPU y usa el procesador para ello, para un controlador que se sabe que falla. |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Un ffmpeg por calidad en la vía GPU en lugar de un proceso para todas: aísla los fallos y evita el grafo de varias salidas que hace caer a los controladores VA frágiles. La anulan los **Límites avanzados**; se lee al arrancar. |

Exponer la GPU al contenedor: [Aceleración por hardware](../administration/hardware-acceleration.md).

## Capacidad y límites

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (número de CPU) | Total de sesiones de transcodificación simultáneas. En una máquina que apenas sostiene una transcodificación 4K HDR por software, pon `1` o `2`. La anulan los **Límites avanzados**; se lee al arrancar. |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Tareas de transcodificación fuera de la reproducción en directo. La anula **Máximo concurrente** en **Admin → Caché de transcodificación**. |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Codifica solo la calidad que está viendo un espectador. `false` comparte una descodificación entre todas las calidades pero codifica las que nadie ve; solo para un equipo sin descodificación por hardware y con fuentes pesadas. La anulan los **Límites avanzados**; se lee al arrancar. |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (medido) | Fuerza el techo de resolución del procesador en lugar del medido. La anulan los **Límites avanzados**; se lee al arrancar. |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (medido) | Lo mismo, para las fuentes HDR convertidas en el procesador. |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | sin límite | Hilos por codificador por software (de 1 a 64). Los codificadores por hardware la ignoran. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (sigue a la fuente) | Un techo para el bitrate del vídeo transcodificado. La anula **Tope de bitrate de vídeo**. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (ninguno) | El mismo techo para los clientes de fuera de la red de casa. La anula **Techo de bitrate de vídeo, espectadores remotos**. |

## Caché

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | El tamaño de toda la caché de transcodificación (segmentos, remux, descargas preparadas); se elimina primero lo usado hace más tiempo. Siempre se mantienen 2 GiB libres en el volumen. La anula **Presupuesto de caché de transcodificación** en **Configuración del servidor**; se lee al arrancar. |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Dónde vive. La define la imagen. |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Dónde viven las miniaturas de la barra de búsqueda. La define la imagen: son caras de regenerar, así que se quedan en `/data`. |

## Clip de calibración

La medición codifica un clip 4K. Por orden de preferencia: tu archivo, un clip
descargado una vez y verificado por su hash, o un clip sintético. Un clip real
mide con más honestidad que el sintético.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | vacías | Un clip tuyo, HDR y SDR. |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | vacías | Un clip que descargar una vez, por `https`. |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | vacías | El SHA256 de esa descarga; obligatorio con una URL. |
