---
source_sha: 943149aa81fcdc21ea4425ec9e43d282740de164c026d29cc366577b71c0fd72
reviewed: false
---
# Transkodierung und Hardware

Kapazität, Grafikkarte, Qualitätsgrenzen und der Transkodierungs-Cache. Die
meisten Installationen setzen nichts davon: Der Server vermisst die Maschine und
wählt die Hardware selbst (siehe
[Transkodierung und Serverkapazität](../playback/transcoding.md)). Wo es eine
Einstellung unter **Admin → Transkodierung** gibt, gewinnt der dort gespeicherte
Wert; die Tabelle sagt es.

## Hardwarebeschleunigung

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | leer | `auto`, `off`, `nvenc`, `vaapi`, `qsv` oder `rkmpp`. Leer: aus den beiden folgenden Variablen abgeleitet. Übersteuert durch **Hardware-Beschleunigung** im Admin-Bereich, die sofort neu misst. |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | Der ältere An/Aus-Schalter. Vom Admin-Bereich übersteuert. |
| `EVEY_TRANSCODING_HW_BACKEND` | leer (Erkennung) | Legt ein Backend fest (`nvenc`, `vaapi`, `qsv`): Nur dieses wird gemessen. Lässt mehrere Evey-Container auf einem Host jeweils einen anderen GPU-Weg nutzen. |
| `EVEY_TRANSCODING_HW_DEVICE` | leer (alle) | Legt ein Gerät fest: einen Render-Knoten (`/dev/dri/renderD129`) oder einen CUDA-Index (`1`). Mit `EVEY_TRANSCODING_HW_BACKEND` bindet es einen Container auf einem Host mit mehreren GPUs an eine GPU. |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | Eine Hardware-Sitzung, die in dieser Zeit ihr erstes Segment nicht erzeugt hat, gilt als hängende GPU und wechselt auf den Prozessor. |
| `EVEY_TRANSCODING_HW_TONEMAP` | an | `0` schaltet HDR-Tone-Mapping auf der GPU ab und nutzt dafür den Prozessor, für einen Treiber, der bekanntermaßen Probleme macht. |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | Ein ffmpeg pro Qualität auf dem GPU-Weg statt eines Prozesses für alle: isoliert Fehler und vermeidet den Graphen mit mehreren Ausgaben, der empfindliche VA-Treiber abstürzen lässt. Übersteuert durch **Erweiterte Grenzwerte**; beim Start gelesen. |

Die GPU für den Container freigeben: [Hardwarebeschleunigung](../administration/hardware-acceleration.md).

## Kapazität und Grenzen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (Zahl der CPUs) | Gesamtzahl gleichzeitiger Transkodierungssitzungen. Auf einer Maschine, die kaum eine 4K-HDR-Transkodierung in Software hält, setzen Sie `1` oder `2`. Übersteuert durch **Erweiterte Grenzwerte**; beim Start gelesen. |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Transkodierungsaufträge außerhalb der laufenden Wiedergabe. Übersteuert durch **Max. gleichzeitig** unter **Admin → Transkodierungs-Cache**. |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Kodiert nur die Qualität, die ein Zuschauer schaut. `false` teilt eine Dekodierung zwischen allen Qualitäten, kodiert aber auch die, die niemand schaut; nur für einen Host ohne Hardware-Dekodierung mit schweren Quellen. Übersteuert durch **Erweiterte Grenzwerte**; beim Start gelesen. |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (gemessen) | Erzwingt die Auflösungsobergrenze des Prozessors statt der gemessenen. Übersteuert durch **Erweiterte Grenzwerte**; beim Start gelesen. |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (gemessen) | Dasselbe für HDR-Quellen mit Tone Mapping auf dem Prozessor. |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | keine Grenze | Threads pro Software-Encoder (1 bis 64). Hardware-Encoder ignorieren sie. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (folgt der Quelle) | Eine Obergrenze für die Bitrate transkodierten Videos. Übersteuert durch **Obergrenze Videobitrate**. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (keine) | Dieselbe Obergrenze für Clients außerhalb des Heimnetzes. Übersteuert durch **Video-Bitratenobergrenze, entfernte Zuschauer**. |

## Cache

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | Die Größe des ganzen Transkodierungs-Caches (Segmente, Remuxe, vorbereitete Downloads), das am längsten Ungenutzte wird zuerst entfernt. 2 GiB bleiben auf dem Volume immer frei. Übersteuert durch **Budget des Transcode-Caches** unter **Servereinstellungen**; beim Start gelesen. |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Wo er liegt. Vom Image gesetzt. |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Wo die Vorschaubilder der Zeitleiste liegen. Vom Image gesetzt: Sie sind teuer neu zu erzeugen und bleiben daher in `/data`. |

## Kalibrierungsclip

Die Messung kodiert einen 4K-Clip. In dieser Vorzugsreihenfolge: Ihre Datei, ein
einmal heruntergeladener und per Hash geprüfter Clip, oder ein synthetischer
Clip. Ein echter Clip misst ehrlicher als der synthetische.

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | leer | Ein eigener Clip, HDR und SDR. |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | leer | Ein einmal herunterzuladender Clip, über `https`. |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | leer | Der SHA256 dieses Downloads; mit einer URL erforderlich. |
