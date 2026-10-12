---
source_sha: 5d6e1c496b373ae579e3de32d6359d6c1c9712b93ef8c17806d17afca066d8f8
reviewed: true
---
# Server-API

Die HTTP-API des Servers, beschrieben in OpenAPI-Dateien (Swagger 2.0), eine
pro Dienst. Dieser Überblick sagt Ihnen, welcher Ordner was enthält und wie
ein Aufruf sich ausweist. Die Dateien selbst sind die Referenz.

| Bereich | Inhalt |
|---------|-------------|
| [Öffentliche API](public/README.md) | Was ein angemeldeter Benutzer tun kann: stöbern, abspielen, sammeln, anfragen. |
| [Admin-API](admin/README.md) | Serververwaltung: Konfiguration, Transkodierung, Sicherungen, Metadaten, Benutzer. |
| [Plugin-API](plugins/README.md) | Plugin-Verwaltung, Webhooks und die Einstellungen, die die Oberfläche eines Plugins pro Benutzer speichert. |

Jede `.swagger.json`-Datei lässt sich in jedes OpenAPI-Werkzeug importieren
(Swagger UI, Postman, Insomnia). Die HTTP-Routen werden aus den
gRPC-Definitionen in `proto/evey/v1/` erzeugt. Ein Methodenname in einer
Datei ist also die gRPC-Methode dahinter.

## Authentifizierung

Drei Arten von Zugangsdaten erreichen die API, alle im Header
`Authorization`:

| Zugangsdaten | Wer | Wie |
|---|---|---|
| Sitzungstoken | Eine Person, die über eine App angemeldet ist | `Bearer <token>`, erhalten bei der Anmeldung und von der App erneuert. |
| API-Schlüssel | Ein Skript, im Namen des Administrators, der ihn erstellt hat | `Bearer evey_...` (das Präfix `Bearer` ist optional). Mit Bereichen, widerrufbar: [Webhooks und API-Schlüssel](../webhooks-api-keys.md). |
| Plugin-Token | Die Oberfläche eines Plugins, über die Hostseite | Für fünf Minuten ausgestellt, für ein Plugin und einen Benutzer. Er trägt nur die Bereiche `public` und `prefs`, die dem Plugin gewährt wurden. Jede andere Methode lehnt ihn ab. Der Frame erhält ihn nie: Die Hostseite leitet die Aufrufe weiter. |

Ein Plugin-Token erreicht nie eine Verwaltungsmethode, egal welche Bereiche er
hat ([ADR-0003](../adr/0003-plugin-scoped-tokens.md),
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

## Neu erzeugen

Für Mitwirkende: Die Dateien werden aus den Protobuf-Definitionen neu erzeugt.

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
