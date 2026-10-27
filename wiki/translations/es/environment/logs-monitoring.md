---
source_sha: 84d4aedb514ecf260e351de19cd0ddda1771d2b1ed9173179da8a7b9ff4e192a
reviewed: true
---
# Copias de seguridad, registros y supervisión { #backups-logs-and-monitoring }

Las variables que hay detrás de las copias de seguridad de la base de datos,
del registro del servidor, del tiempo que se conserva el historial y de los
puntos de acceso para un sistema de supervisión externo. El uso diario está
en [Copias de seguridad y actualizaciones](../backups.md) y en
[Supervisión y registros](../operations/monitoring.md).

## Copias de seguridad { #backups }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Dónde se escriben las copias de seguridad de la base de datos. | ninguno |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (desactivado) | Una copia cada N horas, independiente de la programación del panel, que es la forma habitual. | Copia de seguridad y restauración → **Programación** (expresión cron), inmediato |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Las copias más antiguas se borran cuando el intervalo anterior está definido, al arrancar y después de cada ejecución. La programación del panel tiene su propia **Retención**. | Copia de seguridad y restauración → Programación → **Retención**, inmediato |

## Registro { #log }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` o `error`. | Configuración del servidor → General → **Nivel de registro**, inmediato |
| `EVEY_LOG_DIR` | `/data/logs` | Dónde se escribe el archivo de registro diario `evey-YYYY-MM-DD.log`. La define la imagen. Este archivo es la referencia: `docker logs` muestra el mismo flujo, pero Docker lo rota. | ninguno |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Tamaño de un archivo de registro antes de rotarlo. | ninguno |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Días que se conservan los archivos de registro. | ninguno |

## Conservación del historial { #history-retention }

Cada limpieza diaria vuelve a leer los plazos de conservación, así que un
valor guardado en **Administración → Configuración del servidor → Datos y conservación** se aplica en la siguiente limpieza.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (siempre) | Las sesiones de reproducción terminadas del historial. | Configuración del servidor → Datos y conservación → **Retención del historial de sesiones**, en la siguiente limpieza |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (siempre) | El registro de auditoría de las acciones de administración. | Configuración del servidor → Datos y conservación → **Retención del registro de auditoría**, en la siguiente limpieza |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Las muestras de recursos del servidor (procesador, memoria, ancho de banda). | Configuración del servidor → Datos y conservación → **Retención de métricas del servidor**, en la siguiente limpieza |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Los informes de calidad de reproducción de los reproductores. | Configuración del servidor → Datos y conservación → **Retención de calidad de reproducción (QoE)**, en la siguiente limpieza |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | Cada cuánto se toman esas muestras de recursos. | ninguno |

## Supervisión externa { #external-monitoring }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | vacío | Un token fijo aceptado en `GET /metrics` además de una sesión de administrador, para que Prometheus o Grafana puedan recoger las métricas sin renovar un inicio de sesión. Vacío: solo se acepta una sesión de administrador. | ninguno |
| `EVEY_OTLP_ENDPOINT` | vacío | Un punto de acceso OpenTelemetry al que enviar las trazas. Vacío: sin trazas. | ninguno |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | El nombre del servicio en las trazas. | ninguno |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | La proporción de peticiones trazadas, de 0 a 1 (una proporción, no un porcentaje). | ninguno |

## Solo para desarrollo { #development-only }

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Modo desarrollador: las comprobaciones de seguridad al arrancar solo avisan, y los plugins pueden ejecutarse sin la biblioteca del sandbox. Solo surte efecto junto con `EVEY_ENV=dev`; por sí sola se ignora con un aviso. Nunca en un servidor real. | ninguno |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode, sample ratio)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days) ; internal/adapters/driving/http/settings_handler.go (log.level live via logger.SetLevel ; retention keys) ; i18n admin.serverSettings.logLevel, *RetentionDays, retentionSweepHint
backup: cmd/evey/main.go (StartScheduler env interval + DB cron scheduler both started) ; internal/app/backup/service.go - StartScheduler (returns when interval <= 0 ; CleanupOldBackups first, then each tick) ; i18n admin.backup.schedule.*
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
