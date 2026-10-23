---
source_sha: e3b24610d4942cf71d19babf82add41a0f2e47fd513b1cc840fba554421a75e9
reviewed: true
---
# Bibliotheken und Metadaten { #libraries-and-metadata }

Die Variablen hinter dem Scannen, den Metadatenquellen und der
Intro-Erkennung. Die meisten haben ein Feld im Verwaltungsbereich, das
Vorrang hat, sobald dort ein Wert gespeichert ist. Die Seite des
Verwaltungsbereichs ist unter
[Bibliotheken und Metadaten](../administration/libraries.md) beschrieben.

## Scannen { #scanning }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Überwachung der Dateien in Echtzeit. | Serverkonfiguration → Bibliotheken → **Bibliotheks-Dateiüberwachung**, Neustart |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | Wie oft die Ordner auf das geprüft werden, was der Überwachung entgangen ist (Netzwerkfreigaben). | Serverkonfiguration → Bibliotheken → **Intervall für periodischen Scan**, Neustart |
| `EVEY_LIBRARY_SCAN_WORKERS` | aus dem CPU-Anteil | Dateien, die bei einem Scan gleichzeitig analysiert werden. Von Hand gesetzt, umgeht der Wert für Scans den Anteil unter **Zugeteilte Rechenleistung**. | keine |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Wo der Scan unter Medienzustand seine Ergebnisse und seinen laufenden Auftrag über Neustarts hinweg aufbewahrt. Leer: wird nicht aufbewahrt. | keine |

## Metadaten { #metadata }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | Die Sprache von Titeln und Zusammenfassungen. Die im Einrichtungsassistenten gewählte Sprache hat Vorrang vor dieser Variable. | Serverkonfiguration → Sprache & Region → **Metadatensprache**, sofort |
| `EVEY_TMDB_API_KEY` | eingebauter Schlüssel | Ihr eigener TMDB-Schlüssel, für Ihr eigenes Kontingent. | Serverkonfiguration → API-Schlüssel → **TMDB API-Schlüssel**, Neustart |
| `EVEY_OMDB_API_KEY` | leer | Ein OMDb-Schlüssel, eine zweite Quelle für Filme. | Serverkonfiguration → API-Schlüssel → **OMDB API-Schlüssel**, Neustart |
| `EVEY_OPENSUBTITLES_API_KEY` | leer | Ein OpenSubtitles-Schlüssel, für die Suche nach Untertiteln über die API. | Serverkonfiguration → API-Schlüssel → **OpenSubtitles API-Schlüssel**, Neustart |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | leer | Ein OpenSubtitles-Konto, mit dem Evey sich bei der API anmeldet. | keine |

## Intro, Rückblick und Abspann { #intro-recap-and-credits }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Nutzt die Community-Datenbanken (TheIntroDB, dann IntroDB) vor der eigenen Erkennung des Servers. | Serverkonfiguration → Intro-Skip-Anbieter → **Externe Intro-Skip-Anbieter aktivieren**, Neustart |
| `EVEY_INTRO_TIDB_API_KEY` | leer | Ein TheIntroDB-Schlüssel: 500 Abfragen pro Tag statt 100. | Serverkonfiguration → API-Schlüssel → **TheIntroDB API-Schlüssel**, Neustart |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Jeder Player überspringt erkannte Intros von selbst. | Serverkonfiguration → Bibliotheken → **Intros automatisch überspringen**, sofort |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Bietet auch an, die Vorschau auf die nächste Folge zu überspringen. | Serverkonfiguration → Bibliotheken → **Vorschau-Segmente überspringen**, sofort |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | aus dem CPU-Anteil | Titel, die gleichzeitig analysiert werden. Von Hand gesetzt, umgeht der Wert für diese Aufgabe den Anteil unter **Zugeteilte Rechenleistung**. | keine |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads pro ffmpeg-Prozess während der Erkennung (1 bis 16). | keine |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
watcher / scan interval: cmd/evey/wire_events.go (loadOverride library.watcher_enabled, library.periodic_scan_interval) ; internal/adapters/driving/http/settings_handler.go (requiresRestart)
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin, resolveAPIKey ; cmd/evey/wire_services.go (opensubtitles) ; i18n admin.serverConfig.apiKeys.*
language: cmd/evey/wire_infra.go (metadata.language wins) ; internal/app/config/locale_defaults.go
intro: cmd/evey/wire.go (intro_providers.enabled, api.tidb_key, EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go ; i18n admin.serverConfig.introProvidersRestart ; admin.serverSettings.introAutoSkip, skipPreview
-->
