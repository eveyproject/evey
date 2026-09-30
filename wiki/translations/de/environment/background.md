---
source_sha: 25e92baf53807987435c8781d87f2af02a8a02782951e049a23e404082ad7980
reviewed: false
---
# Hintergrundaufgaben

Die Durchläufe, die der Server von selbst ausführt, und wie sie sich den
Prozessor teilen. Wie sie sich verhalten:
[Hintergrundaufgaben und Serverlast](../operations/background-work.md).

## CPU-Anteil

Der Anteil des Prozessors, den Hintergrundaufgaben nutzen dürfen, wird unter
**Admin → Serverkonfiguration → Zugeteilte Rechenleistung** festgelegt, sofort
und ohne Neustart. Dafür gibt es keine Umgebungsvariable. Die beiden folgenden
bemessen einen Pool von Hand und umgehen diesen Anteil dafür:

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | aus dem Anteil | Während eines Scans parallel analysierte Dateien. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | aus dem Anteil | Gleichzeitig auf Intros und Abspann analysierte Titel. |

## Durchläufe

Jede Variable hat einen Schlüssel, der auch als Zeile unter **Admin →
Serverkonfiguration → Konfigurationseinträge** akzeptiert wird, die diese
Schlüssel vorschlägt; die Zeile gewinnt. Alle werden beim Start gelesen, und
**Hintergrundarbeiten** im selben Bildschirm setzt die wichtigsten.

| Variable | Schlüssel im Admin-Bereich | Standard | Wirkung |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | `transcoding.pause_bg_on_playback` | an | Pausiert Hintergrundarbeit, während ein Stream läuft. `off` lässt sie weiterlaufen. |
| `EVEY_KEYFRAME_BACKFILL` | `transcoding.keyframe_backfill` | an | Indexiert die Keyframes gescannter Dateien und klassifiziert ihre Bildrate. Direktwiedergabe und Kopie hängen davon ab. `off` schaltet es ab. |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | `transcoding.keyframe_backfill_batch` | Serverstandard | Dateien pro Durchlauf (> 0). |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | `transcoding.keyframe_backfill_pause_ms` | Serverstandard | Millisekunden zwischen Dateien (>= 0). |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | `transcoding.keyframe_backfill_idle_sec` | Serverstandard | Sekunden zwischen Durchläufen im Leerlauf (> 0). |
| `EVEY_LEVEL_BACKFILL` | `transcoding.level_backfill` | an | Liest das H.264/HEVC-Level von Dateien, die gescannt wurden, bevor es erfasst wurde. `off` schaltet es ab. |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec` | Serverstandard | Dieselbe Taktung für diesen Durchlauf. |
| `EVEY_SNAPSHOT_FASTPATH` | `transcoding.snapshot_fastpath` | an | Extrahiert Vorschaubilder mit dem schnellen Decoder im Prozess. `off` nutzt nur den exakten ffmpeg-Extraktor, um ihn auszuschließen, wenn ein Vorschaubild das falsche Bild zeigt. |
| `EVEY_TITLE_PROBE` | `transcoding.title_probe` | an | Prüft Titel im Hintergrund mit niedriger Priorität, pausiert während der Wiedergabe. `off` schaltet es ab. |
