---
source_sha: 2ff1222cb6483b259b30b0a40ec51e6e4e8d1e7a7f4a5877a9761a0186adff2d
---
# Ein Video ausliefern - der Weg der HLS-Wiedergabe

Hier geht es um den **Lesepfad**: Was geschieht, wenn ein Player ein Video
anfordert, von der ersten Anfrage nach `master.m3u8` bis hinunter zu den Bytes
eines Segments. Er ist das Gegenstück zur Scan-Pipeline aus
[ARCHITECTURE.md](ARCHITECTURE.md) (dem Schreibpfad, der das Lesemodell füllt)
und die ausführliche Fassung dessen, was in
[CODEBASE_MAP.md](CODEBASE_MAP.md) unter „Golden path 4“ steht.

Der Weg der Wiedergabe ist **lebenswichtig** und verteilt sich auf rund acht
Dateien im `http`-Adapter. Eine einzelne Funktion, die den ganzen Ablauf
durchspielt, gibt es nicht - dieses Dokument ist die Karte dazu.

## Das Denkmodell: 3 Fragen, 2 Maschinen, 1 Schlüssel

Ein Video auszuliefern läuft darauf hinaus, drei Fragen zu beantworten und die
Antwort danach zu verpacken - adressiert wird dabei alles über einen einzigen
Hash:

- **F1 - Komme ich billig an die Datei heran?** Die Verteilung der Prüfung
  richtet sich nach der **Dateiendung**: `.mkv/.webm` und `.mp4/.mov` gehen an
  **mkvgo** (nur der Dateikopf, kein eigener Prozess); alles andere und jeder
  Fehlschlag auf dem schnellen Weg geht an **ffprobe**, das maßgeblich bleibt.
- **F2 - Kann der Client die Datei so abspielen, wie sie ist?** Die
  **Entscheidung** über die Wiedergabe, gefällt *je Spur*: beim Bild `copy`
  gegen `transcode`, beim Ton `copy` gegen `transcode` gegen den AC-3-Weg über
  wasm. Bild und Ton werden **unabhängig voneinander** entschieden - der übliche
  Fall lautet „das Bild kopieren, den Ton nach AAC neu kodieren“.
- **F3 - Wie viel kann diese Maschine kodieren?** Die kalibrierte
  **Leistungsfähigkeit** (aus dem Benchmark beim Start) und dazu die
  **Zulassung** (welche Stufe der Leiter hineinpasst und ob ein Platz frei ist).

Zwei Maschinen ziehen sich durch das Ganze:

- **mkvgo** - liest und verpackt neu (Remux, auf Abruf), ohne einen Prozess zu starten. **Kostenlos.**
- **ffmpeg / ffprobe** - dekodiert, kodiert neu und prüft, indem es einen Prozess startet. **Teuer.**

Und ein Schlüssel: Alles (das Cache-Verzeichnis, die ffmpeg-Sitzung, die URLs
der Segmente) wird über den **Fingerabdruck der Entscheidung** adressiert, also
über einen Hash dieser Entscheidung. Gleiche Bytes ⇒ gleicher Hash ⇒ eine
einzige gemeinsam genutzte Kodierung. Ob das ganze System richtig arbeitet,
hängt daran, dass dieser Hash zwischen dem Manifest, das ein Player in Händen
hält, und jedem später abgerufenen Segment **stabil** bleibt.

## Lesereihenfolge - einer Anfrage durch den Code folgen

Öffnen Sie diese Dateien der Reihe nach, um eine einzelne Wiedergabeanfrage von
Anfang bis Ende zu verfolgen:

1. **Eintritt** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` authentifiziert und verteilt danach auf Master, Variante, Segment
   oder Ton.
2. **F2 · Entscheidung** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (rein): das Gehirn, das je Spur über copy
   oder transcode entscheidet. Umhüllt wird es von `makeDecision` in
   `http/transcode_vod_fingerprint.go`, das die Festlegungen aus der Anfrage
   (`vd=` / `ad=`), die Übersteuerung für den Tonabgleich und weiteres darüber
   legt.
3. **F3 · Leistungsfähigkeit** · `internal/app/transcoding/calibration_service.go` -
   `Current()` liefert `domain/transcoding.Derive(report, policy)`: die
   Obergrenzen der Maschine, abgeleitet aus dem letzten Benchmark.
4. **Die Leiter formen** · `http/transcode_vod_profiles.go` -
   `profilesForDecision`: Komplexitätsklasse → Producer-Profile → Obergrenzen
   aus Quelle und Leistungsfähigkeit → Obergrenzen aus CMCD und AV1 → Route und
   Obergrenze der Zulassung, und dann **friert es die geformte Leiter** auf der
   Festlegung der Zulassung **ein** (bei jedem späteren Aufruf wird sie
   unverändert gelesen - die Invariante des Cache-Hashes, baulich verankert).
5. **Zulassung** · `internal/app/transcoding/admission_shaper.go`
   (das unveränderliche `StreamPlan`) und `video_session_limiter.go`: die Route
   (samt Ausweichen von Hardware auf Software) und der Platz im Kontingent.
6. **Fingerabdruck** · `http/transcode_vod_fingerprint.go` (`buildFingerprint`)
   baut einen `DecisionFingerprint` (Typ und `Hash()` in
   `app/transcoding/segment_cache.go`). Dieser Hash ist der Schlüssel zum Cache.
7. **Die Bytes erzeugen** · copy → **mkvgo** `app/transcoding/ondemand.go` (Port)
   und `adapters/driven/mkvgo/ondemand.go`; transcode → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` ist rein; die
   Segmente landen in `/cache/transcode/segments/<hash>/`).
8. **Verpacken** · `app/transcoding/manifest_generator.go` - `master.m3u8` und
   `stream_N.m3u8` (auf dem mkvgo-Weg zusätzlich `manifest.mpd`).

## Die zwei Maschinen, nach Kosten sortiert

Der **Probe-Dispatcher** (`adapters/driven/prober/dispatcher.go`) ist eine
Zuständigkeitskette entlang der Dateiendung: Er versucht es mit mkvgo und fällt
bei jedem Zweifel auf ffprobe zurück (Fehler beim Parsen, ein Panic, ein
Ergebnis, das sich nicht abbilden lässt, eine verlorene Medienspur im MP4,
Dolby Vision Profil 5, eine unlesbare Farbangabe). Beide schnellen Wege sind
immer aktiv (die Schalter in `wire.go` sind nil); ffprobe ist das
Sicherheitsnetz und wird nie übergangen. Ein zweiter, baugleicher Dispatcher
(`keyframe_dispatcher.go`) kümmert sich um den Suchindex.

| Vorgang | mkvgo (kostenlos) | ffprobe / ffmpeg (teuer) |
|---|---|---|
| Metadaten prüfen (Codec, Maße, HDR, Bitrate, Dauer) | `.mkv/.webm/.mp4/.mov`, nur der Dateikopf | alles Übrige, oder als Rückfall |
| Modus der Bildrate (cfr/vfr) | `AnalyzeFrameRateMode` | - (umgestellt) |
| Remux MKV→MP4 (ohne Neukodierung) | `RemuxToMP4` | - |
| HLS/DASH auf Abruf (die copy-Stufe) | `OnDemandPackager` | - |
| Bild neu kodieren / HDR-Tone-Mapping | - | `hls_transcoder.go` |
| Halbbilder erkennen (idet), Landen nach einem Sprung, chromaprint, DoVi P5 | - (braucht Dekodierung oder Side-Data) | ffmpeg / ffprobe |

Drei ffprobe-Aufrufe laufen weiterhin auf MKV- und MP4-Quellen, deren Daten
mkvgo bereits bereitstellt (die Dauer in `trickplay.go`, Bitrate und Maße in
`complexity_classifier.go`, die Tonverzögerung bei der Wiedergabe in
`audio_delay.go`) - Kandidaten für eine Umstellung; der Gewinn an CPU ist
gering, es ist überwiegend Verdrahtung.

## Der Ton wird getrennt vom Bild entschieden

`makeDecision` liefert `VideoDecision` und `AudioDecision` unabhängig
voneinander. Die „copy-Stufe“ (`isCopyRequest`) hängt allein am **Bild**; selbst
ein kopiertes Bild trägt in der Regel eine neu kodierte AAC-Spur neben sich. Für
den Ton gibt es drei Ausgänge: kopieren, nach AAC transkodieren (ffmpeg, wenig
Aufwand) oder - bei **AC-3 / E-AC-3** in einem Browser, der das nicht selbst
dekodieren kann - den **wasm-Weg**: Der Master enthält dann nur noch Bild und
trägt ein `#EVEY-AUDIO-URI`-Tag, das AC-3 wird **kopiert** (über mkvgo
`od_audio`, sofern sich der Plan bauen lässt, sonst über ffmpeg `-c:a copy`),
und `ac3go.wasm` dekodiert es im Browser nach PCM (`WasmAudioEngine.ts`),
taktgenau zum Bild. Safari wird anhand seiner Fähigkeiten ausgenommen, nicht
anhand seines Namens: `canWasmDecodeAC3()` liefert nur dann true, wenn der
Browser meldet, dass er AC-3 *nicht* selbst abspielen kann - Safari meldet, dass
er es kann.

## Verpacken: CMAF, HLS, DASH

Der Container ist immer **CMAF** (fMP4): je Variante eine `init.mp4` und dazu
`.m4s`-Segmente. (Der MPEG-TS-Zweig in `hls_transcoder.go` ist tot -
`needsFMP4Segments` liefert immer true.) Über *denselben* Segmenten liegen:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → `EXT-X-MAP`-Init und `.m4s`) - bei beiden Maschinen.
- **DASH** (`manifest.mpd`) - auf dem copy-Weg über mkvgo das eigene Manifest
  des Packagers (`OnDemandPlan.HasDASH`, Byte für Byte identisch, ohne Speicher,
  unter AES-128 abgeschaltet); auf dem ffmpeg-Weg ein **aus dem Plan
  deklariertes** Manifest über dieselben Fragmente, die die HLS-Playlists
  adressieren (`ManifestGenerator.MPD`), geschrieben, bevor es ein einziges
  Fragment gibt. Ehrlich macht es das **Segmentraster**
  (`domain/transcoding.SegmentGrid`): Jede transkodierte Stufe wird auf dieselbe
  ganze Zahl von Frames geschnitten (72 bei 23,976 fps, 75 bei 25, 90 bei
  29,97), sodass ein Segment in den Ticks der Videospur exakt und über das ganze
  Umschalt-Set identisch ist; der copy-Rückfall deklariert seinen
  Keyframe-Plan. Eine Quelle mit unbekannter oder variabler Bildrate bleibt beim
  alten 3-s-Takt, behält HLS und antwortet auf `manifest.mpd` mit 404. Kein
  Client wird auf DASH umgestellt: HLS bleibt überall der Standard. mkvgos
  `DASHFromCMAF` über die erzeugten Fragmente ist das Paritätsorakel
  (`internal/pkg/cmafparity`, `TestDASHGridIntegration` bei 23,976/25/29,97 fps:
  kalt, Sprung, Umschalt-Set mit zwei Stufen, getrenntes Audio innerhalb eines
  AAC-Frames).

Die URLs je Sitzung werden neu eingebacken
(`transcode_vod_manifest_rebake.go`), damit sie das Token des Streams tragen -
bei HLS wie bei DASH, den Chromecast ohne Cookies eingeschlossen.

## Die eingesetzten Entwurfsmuster

- **Ports und Adapter (hexagonale Architektur)** - die Anwendungsschicht spricht
  mit Schnittstellen (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`,
  `StreamProducer`), die ffmpeg und mkvgo umsetzen. Eine neue Maschine ist ein
  neuer Adapter.
- **Strategie** - `StreamProducer` (Software, Hardware, copy), ausgewählt von
  einem `ProducerSelector`.
- **Zuständigkeitskette mit Rückfall** - die Dispatcher für Prüfung und
  Keyframes (mkvgo → das maßgebliche ffprobe).
- **Inhaltsadressierter Cache** - der Hash des `DecisionFingerprint` ist der
  Schlüssel zum Cache und zur ffmpeg-Sitzung.
- **Reiner Kern, imperative Hülle** - `Derive`, `MakePlaybackDecision` und das
  Kostenmodell sind rein und werden ohne ffmpeg und ohne Datenbank per Unit-Test
  geprüft.
- **Unveränderliches Wertobjekt** - `StreamPlan`: die Entscheidung der Zulassung
  wird einmal ausgegeben und danach nie verändert, damit die Invariante zwischen
  URLs↔Bytes schon von der Bauart her hält.
- **Geordnetes Nachgeben** - auf jeder Stufe steht ein sicherer Rückfall bereit:
  720p als voreingestellte Leistungsfähigkeit, das Ausweichen von Hardware auf
  Software, copy → transcode, mkvgo → ffprobe.

## Das Entscheidungsgehirn: der PlanResolver in der Anwendungsschicht

Das Gehirn, das eine Anfrage bearbeitet - die Festlegungen der Entscheidung, das
Formen der Leiter, die Wegwahl bei der Hardware, das Zusammensetzen des
Fingerabdrucks, die Wegwahl für die copy-Stufe und die Auswahl des Producers -,
sitzt in der Anwendungsschicht: `PlanResolver`
(`app/transcoding/playback_plan_resolver.go`) mit der konkreten Umsetzung
`PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Jede
Anfrage wird genau **einmal** aufgelöst: Der http-Adapter überführt den
`*http.Request` in eine transportfreie `PlaybackRequest`
(`http/transcode_vod_plan.go`) und liefert danach aus dem zurückgegebenen,
unveränderlichen `PlaybackPlan` aus. Jeder Abnehmer - die Eingabe für das
Manifest, die Prüfung auf Lastabwurf, die Versiegelung gegen veraltete URLs, die
Auswahl des Producers, der Schlüssel des Segments - liest diese eine Auflösung.
*Innerhalb einer Anfrage* hält die Invariante des Cache-Hashes damit schon von
der Bauart her (und der heiße Pfad für ein Segment kam von rund fünf Abfragen
der Profileinstellungen auf eine einzige herunter). Die Helfer für die
Tatsachen über die Quelle (Codec, HDR, SAR, Zehn-Bit, …) liegen daneben in
`app/transcoding/source_facts.go`; im Adapter stehen nur noch dünne Aliasse.
Über mehrere Anfragen hinweg ruht die Invariante nach wie vor darauf, was in die
URLs eingebacken wird: **Jede Eingabe der Entscheidung muss in die
Kind-URLs eingebacken sein**, sonst kann eine Abfrage mitten in der Wiedergabe
den Plan unter dem Player wegziehen (`querySuffix` in
`manifest_generator.go`).
