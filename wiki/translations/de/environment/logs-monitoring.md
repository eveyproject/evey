---
source_sha: 4ea9a6d07b743691fa611b4154d0b15ea314d413a0aa20fafe038abaf561a972
reviewed: false
---
# Sicherungen, Logs und Überwachung

Sicherungen, das Server-Log, wie lange der Verlauf aufbewahrt wird, und Metriken
für einen externen Überwachungsstack.

## Sicherungen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Wohin Datenbanksicherungen geschrieben werden. |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (aus) | Eine Sicherung alle N Stunden. Nur genutzt, wenn unter **Admin → Sicherung & Wiederherstellung → Zeitplan** kein Zeitplan gesetzt ist, was der übliche Weg ist. |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Ältere Sicherungen werden beim Start und vom Zeitplan oben gelöscht. Ein im Admin-Bereich gesetzter Zeitplan hat seine eigene Aufbewahrung. |

Siehe [Sicherungen und Updates](../backups.md).

## Log

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` oder `error`. **Admin → Serverkonfiguration → Servereinstellungen** übersteuert ihn. |
| `EVEY_LOG_DIR` | `/data/logs` | Wohin die tägliche Logdatei `evey-YYYY-MM-DD.log` geschrieben wird. Vom Image gesetzt. Diese Datei ist die Referenz: `docker logs` zeigt denselben Strom, aber Docker rotiert ihn. |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Größe einer Logdatei, bevor sie rotiert. |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Tage, die Logdateien aufbewahrt werden. |

## Aufbewahrung des Verlaufs

Jede wird vom passenden Feld unter **Admin → Serverkonfiguration →
Servereinstellungen** übersteuert, angewendet bei der nächsten täglichen
Bereinigung.

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (für immer) | Beendete Wiedergabesitzungen im Verlauf. |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (für immer) | Das Audit-Log der administrativen Aktionen. |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Die Ressourcenmessungen des Servers (Prozessor, Speicher, Bandbreite). |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Die Berichte der Player zur Streaming-Qualität. |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | Wie oft diese Ressourcenmessungen erfolgen. |

## Externe Überwachung

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | leer | Ein statisches Token, das auf `GET /metrics` neben einer Administratorsitzung akzeptiert wird, damit Prometheus oder Grafana abfragen können, ohne eine Anmeldung zu erneuern. Leer: Nur eine Administratorsitzung wird akzeptiert. |
| `EVEY_OTLP_ENDPOINT` | leer | Ein OpenTelemetry-Endpunkt, an den Traces gesendet werden. Leer: kein Tracing. |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | Der Dienstname in den Traces. |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | Der Anteil der verfolgten Anfragen, 0 bis 1. |

## Nur für die Entwicklung

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Entwicklermodus: Sicherheitsprüfungen beim Start warnen nur, und Plugins dürfen ohne die Sandbox-Bibliothek laufen. Nie auf einem echten Server; mit `EVEY_ENV=production` verweigert der Server den Start. |
