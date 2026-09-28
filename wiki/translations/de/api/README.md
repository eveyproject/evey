---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
---
# Evey-API-Dokumentation

OpenAPI-Spezifikationen (Swagger 2.0) für den Medienserver Evey.

## Aufbau

| Bereich | Beschreibung |
|---------|-------------|
| [Öffentliche API](public/README.md) | Endpunkte für Benutzer - Medien durchsuchen, Wiedergabe, Sammlungen, Playlists usw. |
| [Administrations-API](admin/README.md) | Serververwaltung - Systemkonfiguration, Transkodierung, Sicherungen, Metadaten usw. |
| [Erweiterungs-API](plugins/README.md) | Verwaltung von Erweiterungen und Webhook-Integrationen. |

## Verwendung

Jeder Ordner enthält einzelne `.swagger.json`-Dateien, die sich in jedes OpenAPI-fähige Werkzeug importieren lassen (Swagger UI, Postman, Insomnia usw.).

## Neu erzeugen

Diese Spezifikationen werden mit `buf generate` aus den Protobuf-Definitionen erzeugt. Zum Aktualisieren:

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
