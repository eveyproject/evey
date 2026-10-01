---
source_sha: 4ea9a6d07b743691fa611b4154d0b15ea314d413a0aa20fafe038abaf561a972
reviewed: false
---
# Copias, registros y supervisión

Copias de seguridad, el registro del servidor, cuánto tiempo se conserva el
historial y métricas para una pila de supervisión externa.

## Copias de seguridad

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Dónde se escriben las copias de la base de datos. |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (desactivado) | Una copia cada N horas. Solo se usa cuando no hay programación definida en **Admin → Copia de seguridad y restauración → Programación**, que es la forma habitual. |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Las copias con más de ese número de días se eliminan al arrancar y con la programación anterior. Una programación definida en el panel tiene su propia retención. |

Consulta [Copias de seguridad y actualizaciones](../backups.md).

## Registro

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` o `error`. **Admin → Configuración del servidor → Configuración del servidor** la anula. |
| `EVEY_LOG_DIR` | `/data/logs` | Dónde se escribe el archivo de registro diario `evey-YYYY-MM-DD.log`. La define la imagen. Este archivo es la referencia: `docker logs` muestra el mismo flujo, pero Docker lo rota. |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Tamaño de un archivo de registro antes de rotarlo. |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Días que se conservan los archivos de registro. |

## Retención del historial

Cada una la anula el campo correspondiente de **Admin → Configuración del
servidor → Configuración del servidor**, que se aplica en la siguiente limpieza
diaria.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (para siempre) | Las sesiones de reproducción terminadas del historial. |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (para siempre) | El registro de auditoría de las acciones de administración. |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Las muestras de recursos del servidor (procesador, memoria, ancho de banda). |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Los informes de calidad de streaming de los reproductores. |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | Cada cuánto se toman esas muestras de recursos. |

## Supervisión externa

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | vacía | Un token fijo aceptado en `GET /metrics` además de una sesión de administrador, para que Prometheus o Grafana puedan recoger métricas sin renovar un inicio de sesión. Vacía: solo se acepta una sesión de administrador. |
| `EVEY_OTLP_ENDPOINT` | vacía | Un endpoint de OpenTelemetry al que enviar trazas. Vacía: sin trazas. |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | El nombre del servicio en las trazas. |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | La parte de las peticiones que se trazan, de 0 a 1. |

## Solo para desarrollo

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Modo desarrollo: las comprobaciones de seguridad al arrancar solo avisan, y los plugins pueden ejecutarse sin la biblioteca del sandbox. Nunca en un servidor real; con `EVEY_ENV=production` el servidor se niega a arrancar. |
