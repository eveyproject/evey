---
source_sha: af969a3c2ee02e3c968a79874fba9fc3a2e1b7c9925fd9888aacdf51fcda149b
reviewed: true
---
# Supervisión y registros { #monitoring-and-logs }

Dónde ver lo que está haciendo el servidor, para el administrador que quiere
comprobar su estado más que arreglar algo: desde lo que se reproduce ahora
mismo hasta las estadísticas del mes, con los registros por medio. Si tienes
un síntoma que investigar, empieza por
[Solución de problemas](troubleshooting.md).

## Lo que se reproduce ahora { #what-is-playing-now }

**Administración → Reproduciendo ahora** muestra todos los flujos en curso:
quién, qué, en qué dispositivo, y cómo, pista por pista (copiada,
transcodificada, y el backend que la codifica). Un administrador puede
**Detener** una sesión o **Enviar mensaje** al espectador.

La tarjeta **Quién está en línea** del panel principal muestra quién está
conectado ahora mismo, por perfil, con el tiempo que ha pasado desde la
última señal de cada app. Un Chromecast cuya televisión mantiene cargado el
receptor de Evey sigue en la lista después de que su reproducción haya
terminado y haya salido de **Reproduciendo ahora**; **Detener este
Chromecast** le envía una orden de parada, que la televisión aplica en 30
segundos como mucho.

**Administración → Estado del sistema** muestra el procesador, la memoria,
el ancho de banda y el almacenamiento, y qué parte de la capacidad de
transcodificación está en uso. También lista los discos de la máquina, cada
uno con el tiempo medio que esperó una petición durante el último minuto, su
ocupación y sus caudales de lectura y escritura. La espera es la cifra que hay
que vigilar: un disco puede estar ocupado al 100 % y seguir el ritmo, mientras
que uno que hace esperar a cada petición varias veces más que en reposo
convierte la reproducción en almacenamiento en búfer, aunque el procesador
parezca libre. Un disco se marca como **Saturado** cuando su espera alcanza al
menos 20 ms y al menos 2,5 veces su espera en reposo, con el disco ocupado o el
procesador esperándolo. La tarjeta de CPU indica qué parte del tiempo del
procesador se pasó esperando a los discos.

**Tendencias**, bajo las tarjetas en directo, dibuja las últimas 24 horas,
7 días o 30 días a partir de las muestras que el servidor registra cada minuto:
procesador y espera de disco, memoria, la espera de disco con sus líneas de
reposo y de saturación, ocupación y caudal de los discos, red entrante y
saliente junto al streaming realmente servido, flujos y transcodificaciones, y
el llenado de los discos de las bibliotecas. Procesador, discos y red son de
toda la máquina: una copia u otro servicio que lea los mismos discos aparece
ahí también. El panel lleva una tarjeta **Discos** con el disco que más espera.

<figure class="evey-shot" markdown="span">
  ![Administración → Estado del sistema en el servidor de demostración](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Administración → Estado del sistema</strong>, con el trabajo en segundo plano que se ejecuta debajo (interfaz en inglés).</figcaption>
</figure>

## Registros { #logs }

**Administración → Registros y depuración** busca en el registro del servidor y
lo sigue en directo (**En vivo**), filtrado con **Zona** a una sola parte
(reproductor y transcodificación, índice de posicionamiento, subtítulos,
detección de intros, Chromecast, autenticación, infraestructura...). Es el
primer sitio donde mirar cuando un usuario informa de un problema.

El mismo registro está en el disco, un archivo JSON por día en
`./evey-data/logs/` (`/data/logs/evey-YYYY-MM-DD.log` en el contenedor), y
se guarda 90 días por defecto. `docker compose logs evey` solo muestra lo
que el contenedor escribe mientras arranca, que es lo que hay que leer
cuando el servidor no llega a arrancar. Ver
[Solución de problemas](troubleshooting.md#the-logs).

## Cómo ha ido de verdad la reproducción { #how-playback-actually-went }

**Administración → Calidad de streaming** se construye con lo que informan
los reproductores mientras reproducen (nivel del búfer, recargas,
velocidad, en formato CMCD): cada cuánto han tenido que esperar los
espectadores, los títulos que más se detienen y los tipos de dispositivo que
tienen problemas, en el último día o la última semana.

El servidor también actúa en consecuencia. Cuando un tipo de fuente hace
esperar a los espectadores una y otra vez, baja durante un tiempo la
calidad máxima para ese tipo de fuente, además del techo calibrado, y
después quita el límite por sí solo. Los límites activos aparecen en
**Límites de calidad automáticos**, con el motivo y la fecha de caducidad;
**Restablecer** quita uno en el momento.

## Historial y estadísticas { #history-and-statistics }

- **Administración → Analítica**: reproducciones de hoy y de esta semana,
  usuarios activos, los títulos más vistos de los últimos 30 días.
- **Administración → Actividad**: el historial de reproducciones y de
  acciones de administración, y el trabajo en segundo plano en curso.

El tiempo que se guarda cada tipo de historial se fija en **Administración
→ Configuración del servidor → Ajustes del servidor** (sesiones de
reproducción, registro de auditoría, métricas del servidor, calidad de
streaming). Un cambio se aplica en la siguiente limpieza diaria.

## Métricas de Prometheus { #prometheus-metrics }

`GET /metrics` sirve los contadores del servidor en el formato de texto de
Prometheus, todos con nombres `evey_...`. Responde a una sesión de
administrador, o a un token fijo puesto en `EVEY_METRICS_SCRAPE_TOKEN` y
enviado como `Authorization: Bearer <token>`, para que Prometheus o Grafana
puedan recogerlas sin volver a iniciar sesión
([Copias de seguridad, registros y supervisión](../environment/logs-monitoring.md)).
Las familias con las que conviene empezar los gráficos:

| Métrica | Qué indica |
| --- | --- |
| `evey_transcodes_active`, `evey_transcode_queue_depth` | Las tareas de transcodificación en segundo plano (fuera de la reproducción en directo) en marcha, y las que esperan un hueco. |
| `evey_hlsvod_sessions_active`, `evey_hlsvod_sessions_rejected_total` | Los procesos FFmpeg que producen segmentos de reproducción, y los arranques rechazados porque el servidor estaba en su límite. |
| `evey_hlsvod_cache_size_bytes`, `evey_hlsvod_cache_evicted_bytes_total` | El tamaño de la caché de segmentos en disco, y lo que ha liberado la limpieza. |
| `evey_mkvgo_walks_in_flight`, `evey_mkvgo_plan_stats` | El empaquetador integrado en el proceso: ventanas en lectura, lecturas en cola (`stat="slot_waits"`), ver [MKVGo en Evey](../playback/mkvgo.md#load-replaced-files-and-updates). |
| `evey_http_requests_total`, `evey_http_request_duration_seconds` | Peticiones y latencia por método y por ruta. |
| `evey_watcher_alive`, `evey_scan_queue_depth` | Si el monitor de archivos de cada biblioteca respondió a su última comprobación (1 o 0), y los archivos que esperan a ser escaneados. |

## Línea de comandos { #command-line }

`evey admin doctor` revisa todo el servidor en un solo informe; los demás
comandos de diagnóstico están en [Línea de comandos](../runbooks/cli.md).

<!-- sources
who's online: frontend/apps/web/src/features/admin/DashboardOnline.tsx (presence heartbeat, profile names, idle, SendCastReceiverCommand) ; i18n admin.online.*
logs: internal/pkg/logger/logger.go - New (file only outside DevMode), rotatingWriter ; config EVEY_LOG_RETENTION_DAYS ; docker-compose.yml ./evey-data:/data
labels: i18n admin.sessions.*, admin.systemHealth.*, admin.logs.where.*, admin.logs.live, admin.qoe.*, admin.analytics.*, admin.activity.*, admin.serverSettings.*RetentionDays, retentionSweepHint
automatic limits: i18n admin.qoe.overrides.*
metrics: internal/pkg/metrics/metrics.go (names) ; internal/adapters/driving/http/auth_middleware.go - httpScrapeOrAdminMiddleware (Bearer scrape token or admin JWT) ; docs/adr/0018-cmcd-qoe-pipeline.md
-->
