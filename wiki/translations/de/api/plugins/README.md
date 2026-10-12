---
source_sha: a960630483d1b2a05980e34cb7d0eacb371e9de2acf7fcd4af72e9609a09a8cf
reviewed: true
---
# Plugin-API

Die Endpunkte rund um Plugins: sie installieren und verwalten
(Administrator), Webhooks (Administrator) und die Einstellungen, die die
Oberfläche eines Plugins pro Benutzer speichert. Ein Plugin zu schreiben nutzt
eine andere Schnittstelle, über gRPC auf einem lokalen Socket: siehe
[Ein Plugin schreiben](../../plugins/dev-setup.md) und die
[Plugin-API-Referenz](../../plugins/api-reference.md).

| Spezifikation | Beschreibung |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | Plugins installieren, aktivieren, deaktivieren, deinstallieren; Konfiguration, Protokolle, Ressourcenlimits, Laufzeiten; Vorschau und Anwenden von Updates; `IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | Webhooks erstellen, auflisten, ändern und löschen; Zustellverlauf |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | Die Einstellungen der Oberfläche eines Plugins pro Benutzer (die Berechtigung `prefs`) |

Ein Paket wird per Multipart-`POST /api/v1/admin/plugins/upload` hochgeladen
(Felder `plugin`, optional `sig`, `accept_permission_changes`). `POST
/api/v1/admin/plugins/preview` analysiert, ohne zu installieren. Beides sind
Routen für Administratoren.

## Authentifizierung

Plugin-Verwaltung und Webhooks verlangen einen Administrator, wie die
[Admin-API](../admin/README.md). `plugin_prefs` ist der einzige Dienst, der
für einen Plugin-Token vorgesehen ist: Die Hostseite stellt ihn für ein Plugin
und einen Benutzer aus, und der Dienst antwortet nur mit den Zeilen genau
dieses Paares ([Server-API](../README.md#authentication)).

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
