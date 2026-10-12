---
source_sha: 91a46caa213d6048de832a4352bd5972b5f2d393a40a66301f693b3d908f084c
reviewed: true
---
# MKVGo in Evey

**Kurz gesagt:**

- MKVGo ist ein Teil von Evey, der Videodateien (MKV, WebM, MP4, MOV) liest
  und umverpackt, ohne Bild oder Ton umzuwandeln.
- Wo es geht, liefert MKVGo die Wiedergaben als **Remux** aus, extrahiert
  Untertitel und liest Ihre Dateien beim Scan. Alles, was eine echte
  Umwandlung braucht, geht an FFmpeg, ein eigenes Videowerkzeug.
- Sie müssen nichts einstellen. **Verwaltung → Aktuelle Wiedergabe** zeigt
  Spur für Spur, ob MKVGo oder FFmpeg eine Wiedergabe ausgeliefert hat.

Der Rest dieser Seite ist für alle, die wissen wollen, was der Server ohne
FFmpeg erledigt und wo FFmpeg noch übernimmt. MKVGo läuft im Prozess des
Evey-Servers selbst.

MKVGo arbeitet an der Struktur einer Datei: an ihren Kopfdaten, ihrem Index,
ihren Spuren und deren Paketen. Es dekodiert oder kodiert nie ein Bild oder
einen Ton; alles, was auf dem Server dekodiert werden muss, geht an FFmpeg.
MKVGo wird als eigene Go-Bibliothek entwickelt:
[github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo).

## Drei Stufen der Arbeit

| Stufe | Was passiert | Was es den Server kostet |
| --- | --- | --- |
| **Direktwiedergabe** | Die Datei wird ausgeliefert, wie sie ist. | Das Lesen einer Datei. |
| **MKVGo** | Arbeit an der Struktur: eine Datei lesen, ihre Spuren für das Streaming umverpacken, ihren Index neu aufbauen, eine Spur auf der Zeitachse verschieben. | Code im Prozess des Servers: kein FFmpeg oder ffprobe pro Stream gestartet, und beim Verpacken auf Abruf keine Segmente auf die Festplatte geschrieben. |
| **FFmpeg** | Echte Umwandlungen: neu kodieren, Tone-Mapping, ein Bild dekodieren. Außerdem der Ersatz, wann immer MKVGo mit einer Datei nicht zurechtkommt. | Ein Prozess pro Aufgabe. |

Auch eine Kopie mit FFmpeg (`-c copy`) kodiert nichts neu und bleibt
günstig. MKVGo spart darüber hinaus den Prozess, der für jede Sitzung
startet, und die Segmente, die in den Transkodierungs-Cache geschrieben
werden. Teuer ist das Neukodieren, und nur das Neukodieren von Video belegt
einen Transkodierungsplatz: siehe
[Transkodierung und Serverkapazität](transcoding.md).

Start und Springen warten weder auf einen Prozess noch auf ein erstes Segment
auf der Festplatte: Das Segment wird bei Anfrage aus dem Index der Datei
gebaut. Ob eine Datei diesen Weg nimmt, hängt davon ab, was der Player
dekodiert: [Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md).

## Was MKVGo erledigt

| Arbeit | Wo man es sieht | Wenn MKVGo es nicht kann |
| --- | --- | --- |
| **Dateien lesen** | Beim Scan einer Bibliothek werden Codecs, Auflösung, HDR, Bitrate und Dauer vom Anfang der Datei gelesen. | ffprobe liest sie. Es übernimmt auch bei jedem Zweifel: bei einem Lesefehler, oder wenn MKVGo die Farbinformationen eines HDR-fähigen Codecs nicht lesen konnte. |
| **Keyframe-Index** | Die Stellen, an denen die Wiedergabe beginnen kann, gelesen aus dem Index der Datei. Damit schneiden Remux und Videokopie die Segmente an Keyframes. | ffprobe. |
| **Streaming ohne Neukodieren** | Remux: HLS-Segmente in fragmentiertem MP4 (CMAF), bei Anfrage gebaut nach einem Plan, der pro Datei aufbewahrt wird. Ein Segment wird beim Versand direkt aus der Quelldatei in die Verbindung geschrieben: Nichts wird vorher im Speicher zusammengesetzt, nichts in den Transkodierungs-Cache geschrieben. Über dieselben Segmente wird ein DASH-Manifest erzeugt; die Apps nutzen HLS. | FFmpeg kopiert die Spuren, ebenfalls ohne Neukodieren, in Segmente, die in den Cache geschrieben werden. |
| **MP4 für den Browser** | Eine Matroska-Datei, die ein Browser nicht öffnen kann, wird einmal als MP4 mit Schnellstart neu geschrieben und in einem Cache aufbewahrt. | - |
| **Untertitel** | In ein MKV eingebettete Untertitelspuren werden bei Anfrage extrahiert: Textspuren als WebVTT, Bildspuren (PGS) als Bilder. Deckt der eigene Index der Datei (Cues) eine Spur vollständig ab, liest der Server nur die Blöcke, auf die er zeigt, statt der ganzen Datei. Diesen Blockindex bewahrt er für die nächste Anfrage in seinem Datenverzeichnis auf (`subtitle-index/`). Muss eine Spur doch ganz gelesen werden, lautet die Antwort „wird vorbereitet“, mit einer Wartezeit für den nächsten Versuch, und die Player fragen nach, bis sie fertig ist. | ffmpeg extrahiert die Spur und liest dabei die ganze Datei. |
| **Vorschaubilder** | Der Keyframe, der der gewünschten Stelle am nächsten liegt, wird über den Index gefunden, und FFmpeg dekodiert nur dieses eine Bild. Das gilt, wenn dieser Keyframe höchstens etwa zwei Sekunden von der gewünschten Stelle entfernt ist. | FFmpeg springt und dekodiert bis zur genauen Stelle. |
| **Folgen am Stück** | Der Server kann mehrere Folgen als einen durchgehenden Stream verpacken (`?gapless=1` an der Adresse des Streams). Noch bietet keine App diesen Schalter an. | Jede Folge läuft für sich. |
| **Medienzustand** | Die Diagnose (Seek-Index, Tonstart, in Blöcken gespeicherte Spuren), der neu aufgebaute Index, das Neusynchronisieren hinter einer beschädigten Stelle und das Verschieben einer Tonspur um einen Versatz. Siehe [Medienzustand](../runbooks/media-health.md). | Das Neu-Muxen von Spuren, die in Blöcken gespeichert sind, erledigt FFmpeg. |

## Was bei FFmpeg bleibt

- Jedes Neukodieren, auch das HDR-Tone-Mapping.
- Bilder dekodieren: die Vorschaubilder der Zeitleiste, das Bild eines
  Vorschaubildes.
- Analysen, die dekodierte Daten brauchen: das Erkennen von Zeilensprung
  (Interlacing), Audio-Fingerabdrücke für die Intro-Erkennung.
- Andere Container als MKV/WebM und MP4/MOV, etwa AVI oder MPEG-TS.

## Last, ersetzte Dateien und Updates

**Wie viel es auf einmal liest.** Um ein Segment zu bauen, liest der Server
einen Ausschnitt der Quelle, Video und Ton zusammen: 6 bis 10 MiB bei einer
1080p-Datei, etwa 15 MiB bei einem UHD-Remux. Der Server liest so viele
Ausschnitte gleichzeitig, wie er Prozessoren hat, und weniger auf einer
Maschine mit wenig Arbeitsspeicher: Alle Ausschnitte, die gerade gelesen
werden, müssen in ein Viertel des Arbeitsspeichers passen, den der Server sich
selbst erlaubt (die Hälfte der Grenze des Containers oder des Speichers des
Rechners). Weitere Anfragen warten auf einen freien Platz. So wächst der
benutzte Speicher nicht mit der Zahl der Zuschauer. Startet auf einem kleinen
Rechner eine ganze Gruppe im selben Moment, kommen ihre ersten Segmente
nacheinander an, statt dass dem Server der Speicher ausgeht.

**Eine Datei, die während der Wiedergabe ersetzt wird.** Jede Adresse eines
Segments nennt die Datei, nach der es geplant wurde. Wird eine Datei auf der
Festplatte ersetzt (eine bessere Kopie, eine Reparatur), wird ein Segment, das
genau dann angefragt wird, einmal aus der neuen Datei gebaut. Die Adressen in
der Playlist der alten Datei antworten mit `410 Gone`, statt die beiden
Dateien zu mischen; öffnen Sie den Titel neu, läuft die neue Datei.

**Nach einem Update.** Die Adressen der Segmente tragen die Version von
MKVGo. So liefert ein Browser oder ein Proxy-Cache nie ein Segment aus, das
eine ältere Version gebaut hat, und von diesem Weg liegt kein Segment auf der
Festplatte, das man leeren müsste. Die MP4-Kopien für Browser sind nach der
Version ihres Formats benannt: Ändert sie sich, wird jede Kopie bei der
nächsten Anfrage neu gebaut.

**Beobachten.** Unter `GET /metrics` (siehe
[Überwachung und Protokolle](../operations/monitoring.md#prometheus-metrics))
ist `evey_mkvgo_walks_in_flight` die Zahl der Ausschnitte, die gerade gelesen
werden, und `evey_mkvgo_plan_stats` summiert die Zähler des Verpackers nach
`stat`. Steht `walks_in_flight` an seiner Obergrenze, während
`stat="slot_waits"` steigt, warten Zuschauer auf den Prozessor;
`stream_fallbacks` zählt Segmente, die im Speicher zusammengesetzt werden
mussten, statt gestreamt zu werden.

## Sehen, welche Engine einen Stream ausgeliefert hat

**Verwaltung → Dashboard** und **Verwaltung → Aktuelle Wiedergabe** nennen die
Engine jeder Spur: `mkvgo` oder `ffmpeg`. Ein Remux kann beide zeigen, zum
Beispiel das Video von MKVGo verpackt und den Ton von FFmpeg kopiert.

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
streamed segments: internal/app/transcoding/ondemand.go - OnDemandResource (Bytes nil, WriteRange) ; internal/adapters/driving/http/transcode_ondemand_stream.go - servePlanResourceStreamed
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driving/http/subtitle_handler.go - extractSubtitleVTT ; subtitle_pgs.go ; subtitle_index_cache.go - fromCues, resolveAndStore, newSubtitleIndexCache ; subtitle_preparing.go (202 + Retry-After) ; subtitle_prewarm.go - ensureIndex
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
reads at once: internal/adapters/driven/mkvgo/budget.go - BudgetFor (one walk per CPU, walks within memLimit/4 at 15 MiB each) ; cmd/evey/wire_infra.go (applied after the memory limit) ; cmd/evey/memlimit.go - applyMemoryLimit (half the cgroup or host budget)
replaced file: internal/adapters/driving/http/transcode_ondemand.go - planSourceParam, servePlanSourceGone (410), replay once on a rebuilt plan ; internal/adapters/driving/http/transcode_vod_manifest_rebake.go - stampPlanSourceURIs
after an update: packager version in segment URLs (?v=) ; internal/adapters/driving/http/remux_handler.go - remuxCacheFormat, remuxCacheName
metrics: internal/pkg/metrics/metrics.go - MkvgoWalksInFlight, MkvgoPlanStats ; cmd/evey/wire.go - startMkvgoMetrics
plain opening: rules below ; i18n admin.sessions.track.engine
-->
