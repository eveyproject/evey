---
source_sha: 7d46731b1dfd44908d653fde2e55780a150fa9681de75fa6518859def0fe4dbd
---
# Erweiterungs-API

Endpunkte zur Verwaltung von Erweiterungen und Webhook-Integrationen.

## Spezifikationen

| Spezifikation | Beschreibung |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Erweiterungen installieren, aktivieren, deaktivieren, deinstallieren. Konfiguration und Logs der Erweiterungen. Laufzeitumgebungen. |
| [webhook.swagger.json](webhook.swagger.json) | Webhooks anlegen, lesen, ändern, löschen; Zustellverlauf |

## Erweiterungen entwickeln

Erweiterungen sprechen über gRPC mit dem Evey-Server. Die vollständige Entwicklungsanleitung steht in der [Dokumentation der Erweiterungen](../../plugins.md), unter anderem:
- Lebenszyklus einer Erweiterung (installieren, aktivieren, konfigurieren, deaktivieren, deinstallieren)
- Verfügbare Ereignis-Hooks
- Konfigurationsschema
- Sandbox-Isolation (seccomp, Prozessisolation)
