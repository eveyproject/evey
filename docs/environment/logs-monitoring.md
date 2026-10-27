# Backups, logs and monitoring

The variables behind database backups, the server log, how long history is
kept, and the endpoints for an external monitoring stack. The day-to-day
side is in [Backups and updates](../backups.md) and
[Monitoring and logs](../operations/monitoring.md).

## Backups

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Where database backups are written. | none |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (off) | A backup every N hours, independent of the panel schedule, which is the usual way. | Database Backup → **Schedule** (cron expression), live |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Backups older than this are deleted when the interval above is set, at startup and after each of its runs. The panel schedule has its own **Retention**. | Database Backup → Schedule → **Retention**, live |

## Log

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. | Server Config → General → **Log Level**, live |
| `EVEY_LOG_DIR` | `/data/logs` | Where the daily log file `evey-YYYY-MM-DD.log` is written (with an hour suffix once a file passes 50 MB). Set by the image. This file is the reference: `docker logs` shows the same stream, but Docker rotates it. | none |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Size of a log file before it rotates. | none |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Days log files are kept. | none |

## History retention

The retention windows are read again by each daily cleanup, so a value saved
in **Admin → Server Config → Data & retention** applies at the next sweep.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (forever) | Finished playback sessions in the history. | Server Config → Data & retention → **Play session history retention**, next sweep |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (forever) | The audit log of administrative actions. | Server Config → Data & retention → **Audit log retention**, next sweep |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | The server's resource samples (processor, memory, bandwidth). | Server Config → Data & retention → **Server metrics retention**, next sweep |
| `EVEY_QOE_RETENTION_DAYS` | `30` | The streaming quality reports of the players. | Server Config → Data & retention → **Playback quality (QoE) retention**, next sweep |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | How often those resource samples are taken. | none |

## External monitoring

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | empty | A static token accepted on `GET /metrics` besides an administrator session, so Prometheus or Grafana can scrape without renewing a login. Empty: only an administrator session is accepted. | none |
| `EVEY_OTLP_ENDPOINT` | empty | An OpenTelemetry endpoint to send traces to. Empty: no tracing. | none |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | The service name in the traces. | none |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | The share of requests traced, 0 to 1 (a ratio, not a percentage). | none |

## Development only

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Developer mode: security checks at startup only warn, and plugins may run without the sandbox library. Only takes effect together with `EVEY_ENV=dev`; on its own it is ignored with a warning. Never on a real server. | none |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode, sample ratio)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days) ; internal/adapters/driving/http/settings_handler.go (log.level live via logger.SetLevel ; retention keys) ; i18n admin.serverSettings.logLevel, *RetentionDays, retentionSweepHint
backup: cmd/evey/main.go (StartScheduler env interval + DB cron scheduler both started) ; internal/app/backup/service.go - StartScheduler (returns when interval <= 0 ; CleanupOldBackups first, then each tick) ; i18n admin.backup.schedule.*
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
