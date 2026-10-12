---
source_sha: d0631abae137226c008a3954088887193957c49501d53f8c38282e9314512c14
reviewed: true
---
# Trabajo en segundo plano

Las variables que hay detrás de las pasadas que el servidor hace por su
cuenta y de cómo se reparten el procesador. Qué hace cada pasada y cómo se
protege la reproducción de ellas:
[Trabajo en segundo plano y carga del servidor](../operations/background-work.md).

## Parte de CPU

La parte del procesador que pueden usar las tareas en segundo plano se
ajusta en **Administración → Configuración del servidor → Potencia
asignada**, al momento y sin reiniciar; no tiene variable de entorno. Las dos
variables siguientes dimensionan a mano un grupo de trabajo y, para ese grupo,
sustituyen a esa parte.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | según la parte | Archivos analizados en paralelo durante un escaneo. | ninguno |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | según la parte | Títulos analizados a la vez para las intros y los créditos. | ninguno |

## Pasadas

Cada variable tiene una clave, que también se acepta como fila en
**Administración → Configuración del servidor → Entradas de configuración**,
que sugiere estas claves; la fila tiene prioridad sobre el entorno. La tarjeta **Trabajos en
segundo plano** de la misma pantalla ajusta las principales. Todas se leen al
arrancar, así que un cambio se aplica en el siguiente reinicio, se haga como
se haga.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | activado | Pone en pausa el trabajo en segundo plano mientras se reproduce un flujo. `off` lo deja en marcha. | Entradas de configuración `transcoding.pause_bg_on_playback`, al reiniciar |
| `EVEY_KEYFRAME_BACKFILL` | activado | Indexa los fotogramas clave de los archivos escaneados y clasifica su frecuencia de fotogramas. La reproducción directa y la copia dependen de ello. `off` lo desactiva. | Entradas de configuración `transcoding.keyframe_backfill`, al reiniciar |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | valor por defecto del servidor | Archivos por pasada (> 0). | Entradas de configuración `transcoding.keyframe_backfill_batch`, al reiniciar |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | valor por defecto del servidor | Milisegundos entre archivos (>= 0). | Entradas de configuración `transcoding.keyframe_backfill_pause_ms`, al reiniciar |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | valor por defecto del servidor | Segundos entre pasadas cuando no hay nada que hacer (> 0). | Entradas de configuración `transcoding.keyframe_backfill_idle_sec`, al reiniciar |
| `EVEY_LEVEL_BACKFILL` | activado | Lee el nivel H.264/HEVC de los archivos escaneados antes de que se empezara a recoger. `off` lo desactiva. | Entradas de configuración `transcoding.level_backfill`, al reiniciar |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | valor por defecto del servidor | El mismo ritmo, para esta pasada. | Entradas de configuración `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec`, al reiniciar |
| `EVEY_SNAPSHOT_FASTPATH` | activado | Extrae las miniaturas con el decodificador rápido integrado en el proceso. `off` usa solo el extractor exacto de ffmpeg, para descartarlo cuando una miniatura muestra el fotograma equivocado. | Entradas de configuración `transcoding.snapshot_fastpath`, al reiniciar |
| `EVEY_TITLE_PROBE` | activado | Analiza los títulos en segundo plano con prioridad baja, en pausa durante la reproducción. `off` lo desactiva. | Entradas de configuración `transcoding.title_probe`, al reiniciar |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
