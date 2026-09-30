---
source_sha: daa61595acb5937c7640f83d1baaec5b538968a8324e20be12058340e8e77c15
reviewed: false
---
# Bibliotheken und Metadaten

Scannen, Metadatenquellen und Intro-Erkennung. Die meisten haben ein Gegenstück
im Admin-Bereich, das gewinnt, sobald dort ein Wert gespeichert ist.

## Scannen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Dateiüberwachung in Echtzeit. |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | Wie oft die Ordner auf das geprüft werden, was die Überwachung verpasst hat (Netzwerkfreigaben). **Admin → Serverkonfiguration → Servereinstellungen** übersteuert ihn. |
| `EVEY_LIBRARY_SCAN_WORKERS` | aus dem CPU-Anteil | Während eines Scans parallel analysierte Dateien. Von Hand gesetzt, umgeht er den Anteil **Zugeteilte Rechenleistung** für Scans. |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Wo der Scan des Medienzustands seine Ergebnisse und seinen laufenden Auftrag über Neustarts hinweg aufbewahrt. Leer: nicht aufbewahrt. |

## Metadaten

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | Die Sprache von Titeln und Zusammenfassungen. Die im Einrichtungsassistenten oder unter **Admin → Serverkonfiguration → Sprache & Region** gewählte Sprache gewinnt. |
| `EVEY_TMDB_API_KEY` | eingebauter Schlüssel | Ihr eigener TMDB-Schlüssel, für Ihr eigenes Kontingent. Der unter **Admin → Serverkonfiguration → API-Schlüssel** gesetzte Schlüssel gewinnt. |
| `EVEY_OMDB_API_KEY` | leer | Ein OMDb-Schlüssel, eine zweite Quelle für Filme. Gleiche Übersteuerung. |
| `EVEY_OPENSUBTITLES_API_KEY` | leer | Ein OpenSubtitles-Schlüssel, für die Untertitelsuche über die API. Gleiche Übersteuerung. |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | leer | Ein OpenSubtitles-Konto für die Anmeldung an dessen API. |

## Intro, Rückblick und Abspann

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Nutzt die Community-Datenbanken (TheIntroDB, dann IntroDB) vor der eigenen Erkennung des Servers. **Admin → Serverkonfiguration → Intro-Skip-Anbieter** gewinnt; nach einer Änderung neu starten. |
| `EVEY_INTRO_TIDB_API_KEY` | leer | Ein TheIntroDB-Schlüssel: 500 Abfragen pro Tag statt 100. |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Jeder Player überspringt erkannte Intros von selbst. Gilt aus dem Admin-Bereich sofort. |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Bietet auch an, die Vorschau auf die nächste Folge zu überspringen. Gilt aus dem Admin-Bereich sofort. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | aus dem CPU-Anteil | Gleichzeitig analysierte Titel. Von Hand gesetzt, umgeht er den Anteil **Zugeteilte Rechenleistung** für diese Aufgabe. |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads pro ffmpeg-Prozess während der Erkennung (1 bis 16). |
