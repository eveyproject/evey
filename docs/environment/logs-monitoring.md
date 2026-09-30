# Backups, logs and monitoring

Backups, the server log, how long history is kept, and metrics for an external
monitoring stack.

## Backups

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Where database backups are written. |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (off) | A backup every N hours. Used only when no schedule is set in **Admin → Database Backup → Schedule**, which is the usual way. |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Backups older than this are deleted at startup and by the schedule above. A schedule set in the panel has its own retention. |

See [Backups and updates](../backups.md).

## Log

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. **Admin → Server Config → Server Settings** overrides it. |
| `EVEY_LOG_DIR` | `/data/logs` | Where the daily log file `evey-YYYY-MM-DD.log` is written. Set by the image. This file is the reference: `docker logs` shows the same stream, but Docker rotates it. |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Size of a log file before it rotates. |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Days log files are kept. |

## History retention

Each is overridden by the matching field of **Admin → Server Config → Server
Settings**, applied at the next daily cleanup.

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (forever) | Finished playback sessions in the history. |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (forever) | The audit log of administrative actions. |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | The server's resource samples (processor, memory, bandwidth). |
| `EVEY_QOE_RETENTION_DAYS` | `30` | The streaming quality reports of the players. |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | How often those resource samples are taken. |

## External monitoring

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | empty | A static token accepted on `GET /metrics` besides an administrator session, so Prometheus or Grafana can scrape without renewing a login. Empty: only an administrator session is accepted. |
| `EVEY_OTLP_ENDPOINT` | empty | An OpenTelemetry endpoint to send traces to. Empty: no tracing. |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | The service name in the traces. |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | The share of requests traced, 0 to 1. |

## Development only

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Developer mode: security checks at startup only warn, and plugins may run without the sandbox library. Never on a real server; with `EVEY_ENV=production` the server refuses to start. |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days)
backup schedule precedence: cmd/evey/main.go (env fallback scheduler)
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
