---
source_sha: 6bbcf7c54732dc7eb7955206675ddd1923616486ecdec3c988c6ad0463cf8a04
reviewed: true
---
# Transkodierung und Hardware

Die Variablen hinter der Kapazität, der Grafikkarte, den Qualitätsgrenzen und
dem Transkodierungs-Cache. Die meisten Installationen setzen keine davon: Der
Server misst die Maschine und wählt die Hardware selbst, wie unter
[Transkodierung und Serverkapazität](../playback/transcoding.md) erklärt.
Gibt es eine Einstellung unter **Verwaltung → Transkodierung**, gewinnt der
dort gespeicherte Wert.

## Hardwarebeschleunigung

Wie Sie die GPU für den Container freigeben:
[Hardwarebeschleunigung](../administration/hardware-acceleration.md).

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | leer | `auto`, `off`, `nvenc`, `vaapi`, `qsv` oder `rkmpp`. Leer: wird aus den beiden Variablen darunter abgeleitet. | Transkodierung → **Hardware-Beschleunigung**, sofort (misst neu) |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | Der ältere Ein/Aus-Schalter. | Transkodierung → **Hardware-Beschleunigung**, sofort |
| `EVEY_TRANSCODING_HW_BACKEND` | leer (Erkennung) | Legt ein Backend fest (`nvenc`, `vaapi`, `qsv`): Nur dieses wird gemessen. So kann jeder von mehreren Evey-Containern auf einem Host einen anderen GPU-Weg nutzen. | keine |
| `EVEY_TRANSCODING_HW_DEVICE` | leer (alle) | Legt ein Gerät fest: einen Render-Knoten (`/dev/dri/renderD129`) oder einen CUDA-Index (`1`). Zusammen mit `EVEY_TRANSCODING_HW_BACKEND` bindet es einen Container auf einem Host mit mehreren GPUs an eine GPU. | keine |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Eine Hardware-Sitzung, die in dieser Zeit ihr erstes Segment nicht geliefert hat, gilt als hängende GPU und wird auf den Prozessor verlegt. | keine |
| `EVEY_TRANSCODING_HW_TONEMAP` | an | `0` schaltet das HDR-Tone-Mapping auf der GPU ab und überlässt es dem Prozessor, für einen Treiber, der bekanntermaßen Probleme macht. | keine |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Ein ffmpeg pro Qualitätsstufe auf dem GPU-Weg statt eines Prozesses für alle: Fehler bleiben voneinander getrennt, und der Graph mit mehreren Ausgaben, an dem empfindliche VA-Treiber abstürzen, wird vermieden. | Transkodierung → Erweiterte Grenzwerte → **Ein Hardware-Encoder-Prozess pro Stufe**, Neustart |

Der stromsparende Intel-Encoder und die AV1-Ausgabe (Schalter und Obergrenze)
haben keine Variable: Sie werden nur unter **Verwaltung → Transkodierung**
eingestellt.

## Kapazität und Grenzen

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (Zahl der CPUs) | Gleichzeitige Transkodierungssitzungen insgesamt. Auf einer Maschine, die gerade so eine 4K-HDR-Transkodierung in Software schafft, setzen Sie `1` oder `2`. | Transkodierung → Erweiterte Grenzwerte → **Maximale gleichzeitige HLS-Sitzungen**, Neustart |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Transkodierungsaufträge außerhalb der laufenden Wiedergabe. | Transkodierungs-Cache → **Max. gleichzeitig**, sofort |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Kodiert nur die Qualität, die ein Zuschauer gerade schaut. `false` teilt ein Dekodieren zwischen allen Qualitäten, kodiert aber auch die, die niemand schaut; nur für einen Host ohne Hardware-Dekodierung mit schweren Quellen. | Transkodierung → Erweiterte Grenzwerte → **Nur eine Ladder-Stufe gleichzeitig kodieren**, Neustart |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (gemessen) | Erzwingt die Auflösungsobergrenze des Prozessors statt der gemessenen. | Transkodierung → Erweiterte Grenzwerte → **Höhengrenze für Software-Transcoding**, Neustart |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (gemessen) | Dasselbe für HDR-Quellen, deren Tone-Mapping der Prozessor übernimmt. | Transkodierung → Erweiterte Grenzwerte → **HDR-Höhengrenze für Software**, Neustart |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | keine Grenze | Threads pro Software-Encoder (1 bis 64; jeder andere Wert bedeutet keine Grenze). Hardware-Encoder ignorieren sie. | keine |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (folgt der Quelle) | Eine Obergrenze für die Bitrate des transkodierten Videos. | Transkodierung → **Obergrenze Videobitrate**, sofort |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (keine) | Dieselbe Obergrenze für Clients außerhalb des Heimnetzes. | Transkodierung → **Video-Bitratenobergrenze, entfernte Zuschauer**, sofort |

## Cache

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | Die Größe des gesamten Transkodierungs-Caches (Segmente, Remux-Dateien, vorbereitete Downloads). Was am längsten nicht benutzt wurde, wird zuerst entfernt. | Serverkonfiguration → Servereinstellungen → **Budget des Transcode-Caches**, Neustart |
| `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` | `2048` | Freier Platz, der auf dem Laufwerk des Caches frei bleibt. Darunter werden ungenutzte Segmente entfernt, dann werden neue Segmente abgelehnt statt geschrieben; extrahierte Untertitel werden nicht mehr gespeichert, und die Administratoren werden benachrichtigt. `0` schaltet diese Reserve ab. Siehe [Der Transkodierungs-Cache](../playback/transcoding.md#the-transcoding-cache). | keine |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Wo er liegt. Vom Image gesetzt. | keine |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Wo die Vorschaubilder der Zeitleiste liegen. Vom Image gesetzt: Sie neu zu erzeugen ist aufwendig, deshalb bleiben sie in `/data`. | keine |

## Kalibrierungsclip

Die Messung kodiert einen 4K-Clip. In dieser Reihenfolge: Ihre Datei, ein
Clip, der einmal heruntergeladen und anhand seines Hashwerts geprüft wird,
oder ein künstlich erzeugter Clip. Ein echter Clip misst ehrlicher als der
künstliche. Keine dieser Variablen hat ein Gegenstück im Verwaltungsbereich.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | leer | Ein eigener Clip, HDR und SDR. | keine |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | leer | Ein Clip, der einmal über `https` heruntergeladen wird. | keine |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | leer | Der SHA256 dieses Downloads; Pflicht zusammen mit einer URL. | keine |

<!-- sources
cmd/evey/wire.go - EVEY_TRANSCODE_CACHE_MIN_FREE_MB (read at boot, not in config.go) ; internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SingleVariant, SoftwareMaxHeightOverride, SoftwareAV1MaxHeightOverride (panel-only), AV1Enabled, AV1MaxHeight, HWLowPowerEnabled
panel overrides: cmd/evey/main.go - loadOverride (transcoding.*) ; cmd/evey/wire.go (transcoding.hw_mode read directly) ; internal/adapters/driving/http/settings_handler.go
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: frontend/apps/web/src/routes/_auth/admin/transcode-cache.lazy.tsx (transcoding.max_concurrent) ; i18n admin.transcodeCache.maxConcurrent
labels: i18n admin.transcoding.advancedTitle, maxSessions, softwareMaxHeight, softwareHDRMaxHeight, singleVariant, hwPerVariant, hwModeLabel, maxBitrateLabel, maxBitrateRemoteLabel ; admin.serverSettings.transcodeCacheMaxGB
-->
