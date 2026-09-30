---
source_sha: eea62b3dd7b2be9c176a9b8431d6ee58738ecf22c64c0f154bb1b1211878f293
reviewed: false
---
# Plugin-API

Endpunkte zur Verwaltung von Plugins und Webhook-Integrationen.

## Spezifikationen

| Spezifikation | Beschreibung |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Plugins installieren, aktivieren, deaktivieren, deinstallieren. Plugin-Konfiguration und Logs. Plugin-Laufzeiten. |
| [webhook.swagger.json](webhook.swagger.json) | Webhooks anlegen, lesen, ändern, löschen, Zustellungsverlauf |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Die Vorlieben pro Benutzer der Oberfläche eines Plugins (die Berechtigung `prefs`) |

## Plugin-Entwicklung

Plugins kommunizieren über gRPC mit dem Evey-Server. Siehe die [Plugin-Dokumentation](../../plugins.md) für die vollständige Entwicklungsanleitung, darunter:
- Lebenszyklus eines Plugins (installieren, aktivieren, konfigurieren, deaktivieren, deinstallieren)
- Verfügbare Ereignisse
- Konfigurationsschema
- Sandbox-Sicherheit (seccomp, Prozessisolation)
