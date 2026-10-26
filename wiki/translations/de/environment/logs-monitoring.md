---
source_sha: 0bab686b9f686329111402f3fd66ecca742fbcfae479769927962bdcf8eeadcd
reviewed: true
---
# Sicherungen, Protokolle und Überwachung { #backups-logs-and-monitoring }

Die Variablen hinter den Datenbanksicherungen, dem Serverprotokoll, der
Aufbewahrungsdauer des Verlaufs und den Endpunkten für eine externe
Überwachung. Die tägliche Praxis steht unter
[Sicherungen und Updates](../backups.md) und
[Überwachung und Protokolle](../operations/monitoring.md).

## Sicherungen { #backups }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | Wohin die Datenbanksicherungen geschrieben werden. | keine |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (aus) | Eine Sicherung alle N Stunden, unabhängig vom Zeitplan im Verwaltungsbereich, der der übliche Weg ist. | Sicherung & Wiederherstellung → **Zeitplan** (Cron-Ausdruck), sofort |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | Ältere Sicherungen werden gelöscht, wenn das Intervall darüber gesetzt ist: beim Start und nach jedem seiner Läufe. Der Zeitplan im Verwaltungsbereich hat seine eigene **Aufbewahrung**. | Sicherung & Wiederherstellung → Zeitplan → **Aufbewahrung**, sofort |

## Protokoll { #log }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` oder `error`. | Serverkonfiguration → Allgemein → **Protokollstufe**, sofort |
| `EVEY_LOG_DIR` | `/data/logs` | Wohin die tägliche Protokolldatei `evey-YYYY-MM-DD.log` geschrieben wird. Vom Image gesetzt. Diese Datei ist maßgeblich: `docker logs` zeigt denselben Strom, aber Docker rotiert ihn. | keine |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | Größe einer Protokolldatei, bevor sie rotiert wird. | keine |
| `EVEY_LOG_RETENTION_DAYS` | `90` | Tage, die Protokolldateien aufbewahrt werden. | keine |

## Aufbewahrung des Verlaufs { #history-retention }

Die Aufbewahrungsfristen werden bei jeder täglichen Bereinigung neu gelesen.
Ein unter **Verwaltung → Serverkonfiguration → Daten & Aufbewahrung**
gespeicherter Wert gilt also ab dem nächsten Durchlauf.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (für immer) | Beendete Wiedergabesitzungen im Verlauf. | Serverkonfiguration → Daten & Aufbewahrung → **Aufbewahrung des Sitzungsverlaufs**, nächster Durchlauf |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (für immer) | Das Audit-Protokoll der Verwaltungsaktionen. | Serverkonfiguration → Daten & Aufbewahrung → **Aufbewahrung des Audit-Protokolls**, nächster Durchlauf |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | Die Ressourcenmessungen des Servers (Prozessor, Arbeitsspeicher, Bandbreite). | Serverkonfiguration → Daten & Aufbewahrung → **Aufbewahrung der Servermetriken**, nächster Durchlauf |
| `EVEY_QOE_RETENTION_DAYS` | `30` | Die Berichte der Player zur Streaming-Qualität. | Serverkonfiguration → Daten & Aufbewahrung → **Aufbewahrung der Wiedergabequalität (QoE)**, nächster Durchlauf |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | Wie oft diese Ressourcenmessungen erfasst werden. | keine |

## Externe Überwachung { #external-monitoring }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | leer | Ein festes Token, das auf `GET /metrics` neben einer Administratorsitzung akzeptiert wird. So können Prometheus oder Grafana Daten abrufen, ohne eine Anmeldung zu erneuern. Leer: Nur eine Administratorsitzung wird akzeptiert. | keine |
| `EVEY_OTLP_ENDPOINT` | leer | Ein OpenTelemetry-Endpunkt, an den Traces gesendet werden. Leer: kein Tracing. | keine |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | Der Dienstname in den Traces. | keine |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | Der Anteil der Anfragen, die verfolgt werden, 0 bis 1 (ein Verhältnis, kein Prozentwert). | keine |

## Nur für die Entwicklung { #development-only }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | Entwicklermodus: Sicherheitsprüfungen beim Start warnen nur, und Plugins dürfen ohne die Sandbox-Bibliothek laufen. Wirkt nur zusammen mit `EVEY_ENV=dev`; allein wird sie mit einer Warnung ignoriert. Nie auf einem echten Server. | keine |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode, sample ratio)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days) ; internal/adapters/driving/http/settings_handler.go (log.level live via logger.SetLevel ; retention keys) ; i18n admin.serverSettings.logLevel, *RetentionDays, retentionSweepHint
backup: cmd/evey/main.go (StartScheduler env interval + DB cron scheduler both started) ; internal/app/backup/service.go - StartScheduler (returns when interval <= 0 ; CleanupOldBackups first, then each tick) ; i18n admin.backup.schedule.*
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
