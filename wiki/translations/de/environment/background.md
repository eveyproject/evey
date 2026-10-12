---
source_sha: d0631abae137226c008a3954088887193957c49501d53f8c38282e9314512c14
reviewed: true
---
# Hintergrundaufgaben

Die Variablen hinter den Durchläufen, die der Server von selbst ausführt, und
wie sie sich den Prozessor teilen. Was jeder Durchlauf tut und wie die
Wiedergabe davor geschützt wird:
[Hintergrundaufgaben und Serverlast](../operations/background-work.md).

## CPU-Anteil

Den Anteil des Prozessors, den Hintergrundaufgaben nutzen dürfen, stellen Sie
unter **Verwaltung → Serverkonfiguration → Zugeteilte Rechenleistung** ein,
sofort wirksam, ohne Neustart. Dafür gibt es keine Umgebungsvariable. Die
beiden Variablen unten legen die Größe eines Pools von Hand fest und umgehen
dafür diesen Anteil.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | aus dem Anteil | Dateien, die bei einem Scan gleichzeitig analysiert werden. | keine |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | aus dem Anteil | Titel, die gleichzeitig auf Intros und Abspann analysiert werden. | keine |

## Durchläufe

Jede Variable hat einen Schlüssel, der auch als Zeile unter **Verwaltung →
Serverkonfiguration → Konfigurationseinträge** akzeptiert wird; dort werden
diese Schlüssel vorgeschlagen. Die Zeile hat Vorrang vor der Umgebung. Die
Karte **Hintergrundarbeiten** auf demselben Bildschirm stellt die wichtigsten
ein. Alle werden beim Start gelesen: Eine Änderung gilt also ab dem nächsten
Neustart, egal auf welchem Weg sie gemacht wurde.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | an | Pausiert die Hintergrundarbeit, solange ein Stream läuft. `off` lässt sie weiterlaufen. | Konfigurationseinträge `transcoding.pause_bg_on_playback`, Neustart |
| `EVEY_KEYFRAME_BACKFILL` | an | Indexiert die Keyframes gescannter Dateien und ordnet ihre Bildrate ein. Direktwiedergabe und Kopieren hängen davon ab. `off` schaltet es ab. | Konfigurationseinträge `transcoding.keyframe_backfill`, Neustart |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | Serverstandard | Dateien pro Durchlauf (> 0). | Konfigurationseinträge `transcoding.keyframe_backfill_batch`, Neustart |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | Serverstandard | Millisekunden zwischen zwei Dateien (>= 0). | Konfigurationseinträge `transcoding.keyframe_backfill_pause_ms`, Neustart |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | Serverstandard | Sekunden zwischen zwei Durchläufen, wenn nichts zu tun ist (> 0). | Konfigurationseinträge `transcoding.keyframe_backfill_idle_sec`, Neustart |
| `EVEY_LEVEL_BACKFILL` | an | Liest das H.264/HEVC-Level von Dateien, die gescannt wurden, bevor es erfasst wurde. `off` schaltet es ab. | Konfigurationseinträge `transcoding.level_backfill`, Neustart |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | Serverstandard | Dasselbe Tempo, für diesen Durchlauf. | Konfigurationseinträge `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec`, Neustart |
| `EVEY_SNAPSHOT_FASTPATH` | an | Extrahiert Vorschaubilder mit dem schnellen Decoder im Prozess. `off` nutzt nur den exakten Extraktor von ffmpeg, um ihn als Ursache auszuschließen, wenn ein Vorschaubild das falsche Bild zeigt. | Konfigurationseinträge `transcoding.snapshot_fastpath`, Neustart |
| `EVEY_TITLE_PROBE` | an | Untersucht Titel im Hintergrund mit niedriger Priorität, pausiert während der Wiedergabe. `off` schaltet es ab. | Konfigurationseinträge `transcoding.title_probe`, Neustart |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
