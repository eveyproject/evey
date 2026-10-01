---
source_sha: fbe5c63d26023c75b078bb43d1151792e32f425b107270e73ddeebd67c742e4a
reviewed: false
---
# Ein Video ausliefern - der HLS-Wiedergabeweg

Dies ist der **Leseweg**: was passiert, wenn ein Player ein Video anfordert, von
der ersten Anfrage nach `master.m3u8` bis zu den Bytes eines Segments. Er ist
das Gegenstück zur Scan-Pipeline in [ARCHITECTURE.md](ARCHITECTURE.md) (der
Schreibweg, der das Lesemodell befüllt) und die Vertiefung hinter
[CODEBASE_MAP.md](CODEBASE_MAP.md) „Golden path 4".

Der Wiedergabeweg ist **LEBENSWICHTIG** und über ~8 Dateien im Adapter `http`
verteilt. Es gibt keine einzelne Funktion, die den ganzen Ablauf ausführt -
dieses Dokument ist die Karte.

## Das Denkmodell: 3 Fragen, 2 Maschinen, 1 Schlüssel

Ein Video auszuliefern läuft darauf hinaus, drei Fragen zu beantworten und die
Antwort dann zu verpacken, wobei alles über einen einzigen Hash adressiert wird:

- **F1 - Kann ich die Datei günstig lesen?** Verteilung der Prüfung nach
  **Dateiendung**: `.mkv/.webm` und `.mp4/.mov` gehen an **mkvgo** (nur der
  Kopf, kein Prozess); alles andere und jedes Scheitern des schnellen Wegs geht
  an **ffprobe** (die Autorität).
- **F2 - Kann der Client sie so abspielen, wie sie ist?** Die
  Wiedergabe**entscheidung**, *pro Spur* getroffen: Video `copy` gegenüber
  `transcode`, Audio `copy` gegenüber `transcode` gegenüber dem wasm-Weg für
  AC-3. Video und Audio werden **unabhängig** entschieden - der häufige Fall ist
  „Video kopieren, Audio nach AAC neu kodieren".
- **F3 - Wie viel kann dieser Host kodieren?** Die kalibrierte **Fähigkeit**
  (aus der im ersten ruhigen Moment erstellten Messung) plus die **Zulassung**
  (welche Stufe der Leiter passt, ist ein Platz frei).

Zwei Maschinen laufen durch alles hindurch:

- **mkvgo** - liest und verpackt neu (Remux / auf Anfrage), ohne einen Prozess zu starten. **Kostenlos.**
- **ffmpeg / ffprobe** - dekodiert, kodiert neu, prüft, indem es einen Prozess startet. **Teuer.**

Ein Schlüssel: Alles (Cache-Verzeichnis, ffmpeg-Sitzung, Segment-URLs) wird über
den **Entscheidungs-Fingerabdruck** adressiert - einen Hash der Entscheidung.
Gleiche Bytes ⇒ gleicher Hash ⇒ eine gemeinsame Kodierung. Die Korrektheit des
ganzen Systems hängt davon ab, dass dieser Hash **stabil** bleibt zwischen dem
Manifest, das ein Player hält, und jedem Segment, das er später abruft.

## Lesereihenfolge - einer Anfrage durch den Code folgen

Öffnen Sie diese der Reihe nach, um eine einzelne Wiedergabeanfrage von Anfang
bis Ende zu verfolgen:

1. **Eingang** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` authentifiziert und verteilt dann an Master / Variante / Segment / Audio.
2. **Der Plan** · `http/transcode_vod_plan.go` übersetzt die Anfrage in einen
   `PlaybackRequest`; `app/transcoding/playback_plan_resolver.go` -
   `PlanResolver.Resolve` liefert einen einzigen unveränderlichen `PlaybackPlan`:
   die Entscheidung (F2), die Leiterklasse, die geformte Leiter, den
   Hardwareweg, den Paketierer und den Fingerabdruck. Jedes Manifest und jedes
   Segment der Sitzung wird aus ihm ausgeliefert
   ([ADR-0030](adr/0030-one-plan-one-decision.md)). `makeDecision` und
   `buildFingerprint` in `http/transcode_vod_fingerprint.go` sind dünne Hüllen
   für die Aufrufstellen ohne Plan (Vorabladen, Master auf Anfrage).
3. **F2 · Entscheidung** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (rein): das Gehirn für Kopieren oder
   Transkodieren pro Spur. Der Resolver fügt die Festlegungen der Anfrage
   (`vd=` / `ad=`), die Übersteuerung der Audio-Resynchronisation usw. hinzu.
4. **F3 · Fähigkeit** · `internal/app/transcoding/calibration_service.go` -
   `Current()` liefert `domain/transcoding.Derive(report, policy)`
   (`derive.go`): die Obergrenzen des Hosts, aus der letzten Kalibrierung
   abgeleitet.
5. **Leiter und Zulassung** · der Resolver formt die Leiter (Komplexitätsklasse
   → Profile der Produzenten → Obergrenzen von Quelle und Fähigkeit →
   Obergrenzen CMCD/AV1 → Weg und Obergrenze der Zulassung) und **friert sie
   dann ein** auf der Festlegung der Zulassung (`admission_shaper.go`,
   `FreezeLadder`), bei jedem späteren Aufruf unverändert gelesen - die
   Invariante des Cache-Hashs strukturell gemacht. `video_session_limiter.go`
   hält den Kapazitätsplatz; der Weg weicht von Hardware auf Software aus, wenn
   sie voll ist.
6. **Fingerabdruck** · ein `DecisionFingerprint` (Typ + `Hash()` in
   `app/transcoding/segment_cache.go`), vom Resolver zusammengesetzt. Der Hash
   ist der Cache-Schlüssel.
7. **Die Bytes erzeugen** · Kopie → **mkvgo** `app/transcoding/ondemand.go` (Port) /
   `adapters/driven/mkvgo/ondemand.go`; Transkodierung → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` ist rein; die
   Segmente landen in `/cache/transcode/segments/<hash>/`).
8. **Verpackung** · `app/transcoding/manifest_generator.go` - `master.m3u8` /
   `stream_N.m3u8`; `manifest_generator_dash.go` - `manifest.mpd`.

## Die zwei Maschinen, nach Kosten

Der **Prüfverteiler** (`adapters/driven/prober/dispatcher.go`) ist eine
Zuständigkeitskette nach Dateiendung: Er versucht mkvgo und fällt bei jedem
Zweifel auf ffprobe zurück (Parse-Fehler, Panic, nicht abbildbares Ergebnis,
eine verlorene MP4-Medienspur, DoVi-Profil 5, unlesbare Farbe). Beide schnellen
Wege sind immer an (nil-Schalter in `wire.go`); ffprobe ist das Sicherheitsnetz,
nie umgangen. Ein zweiter, identischer Verteiler (`keyframe_dispatcher.go`)
kümmert sich um den Seek-Index.

| Vorgang | mkvgo (kostenlos) | ffprobe / ffmpeg (teuer) |
|---|---|---|
| Prüfung der Metadaten (Codec, Maße, HDR, Bitrate, Dauer) | `.mkv/.webm/.mp4/.mov` nur der Kopf | alles andere, oder Rückfall |
| Bildratenmodus (cfr/vfr) | `AnalyzeFrameRateMode` | - (migriert) |
| Remux MKV→MP4 (ohne Neukodierung) | `RemuxToMP4` | - |
| HLS/DASH auf Anfrage (Kopierstufe) | `OnDemandPackager` | - |
| Video neu kodieren / HDR-Tone-Mapping | - | `hls_transcoder.go` |
| Zeilensprungerkennung (idet), Sprungziel, chromaprint, DoVi P5 | - (braucht Dekodierung / Seitendaten) | ffmpeg / ffprobe |

Drei ffprobe-Aufrufe laufen noch auf MKV/MP4-Quellen, deren Daten mkvgo bereits
liefert (Dauer in `trickplay.go`, Bitrate+Maße in `complexity_classifier.go`,
Audioverzögerung der Wiedergabe in `audio_delay.go`) - Kandidaten für eine
Migration; kleiner CPU-Gewinn, hauptsächlich Verdrahtung.

## Audio wird getrennt vom Video entschieden

`makeDecision` erzeugt `VideoDecision` und `AudioDecision` unabhängig voneinander.
Die „Kopierstufe" (`isCopyRequest`) hängt nur am **Video**; selbst ein kopiertes
Video trägt meist eine daneben nach AAC neu kodierte Spur. Die Audioentscheidung
hat drei Ergebnisse: kopieren, nach AAC transkodieren (ffmpeg, leicht) oder - für
**AC-3 / E-AC-3** in einem Browser, der es nicht nativ dekodieren kann - den
**wasm-Weg**: Der Master wird mit einem Tag `#EVEY-AUDIO-URI` zu reinem Video,
das AC-3 wird **kopiert** (mkvgo `od_audio`, wenn der Plan es baut, sonst ffmpeg
`-c:a copy`), und `ac3go.wasm` dekodiert es im Browser zu PCM
(`WasmAudioEngine.ts`), an die Uhr des Videos gekoppelt. Safari wird über die
Fähigkeit ausgeschlossen, nicht über den Namen: `canWasmDecodeAC3()` liefert nur
dann true, wenn der Browser meldet, AC-3 *nicht* nativ abspielen zu können -
Safari meldet, dass es kann.

## Verpackung: CMAF, HLS, DASH

Der Container ist immer **CMAF** (fMP4): ein `init.mp4` pro Variante plus
`.m4s`-Segmente. (Der MPEG-TS-Zweig in `hls_transcoder.go` ist tot -
`needsFMP4Segments` liefert immer true.) Über *dieselben* Segmente:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → Init `EXT-X-MAP` + `.m4s`) - beide Maschinen.
- **DASH** (`manifest.mpd`) - auf dem Kopierweg von mkvgo das eigene Manifest
  des Paketierers (`OnDemandPlan.HasDASH`, byte-identisch, kein Speicherbedarf,
  unter AES-128 abgeschaltet); auf dem ffmpeg-Weg ein **aus dem Plan
  deklariertes** Manifest über dieselben Fragmente, die die HLS-Playlists
  adressieren (`ManifestGenerator.MPD`), geschrieben, bevor ein Fragment
  existiert. Ehrlich macht es das **Segmentraster**
  (`domain/transcoding.SegmentGrid`): Jede transkodierte Stufe wird auf dieselbe
  ganze Zahl von Bildern geschnitten (72 bei 23,976 fps, 75 bei 25, 90 bei
  29,97), sodass ein Segment in den Ticks der Videospur exakt und über die
  Umschaltgruppe identisch ist; der Kopier-Rückfall deklariert seinen
  Keyframe-Plan. Eine Quelle mit unbekannter oder variabler Bildrate bleibt beim
  alten 3-s-Takt, behält HLS und antwortet auf `manifest.mpd` mit 404. Kein
  Client wird auf DASH umgestellt: HLS bleibt überall der Standard.
  `DASHFromCMAF` von mkvgo über die erzeugten Fragmente ist das Paritätsorakel
  (`internal/pkg/cmafparity`, `TestDASHGridIntegration` bei 23,976/25/29,97 fps:
  kalt, Sprung, Umschaltgruppe mit zwei Stufen, getrenntes Audio innerhalb eines
  AAC-Frames).

URLs pro Sitzung werden neu gebacken (`transcode_vod_manifest_rebake.go`), um das
Stream-Token zu tragen, für HLS und DASH gleichermaßen, auch für den Chromecast
ohne Cookies.

## Genutzte Entwurfsmuster

- **Ports und Adapter (hexagonal)** - die Anwendungsschicht spricht mit
  Schnittstellen (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`,
  `StreamProducer`); ffmpeg und mkvgo implementieren sie. Eine neue Maschine ist
  ein neuer Adapter.
- **Strategie** - `StreamProducer` (Software / Hardware / Kopie), von einem
  `ProducerSelector` gewählt.
- **Zuständigkeitskette mit Rückfall** - die Verteiler für Prüfung und Keyframes
  (mkvgo → Autorität ffprobe).
- **Inhaltsadressierter Cache** - der Hash `DecisionFingerprint` ist der Schlüssel
  von Cache und ffmpeg-Sitzung.
- **Reiner Kern / imperative Hülle** - `Derive`, `MakePlaybackDecision`, das
  Kostenmodell sind rein und ohne ffmpeg oder Datenbank unit-getestet.
- **Unveränderliches Wertobjekt** - `StreamPlan`: die Zulassungsentscheidung,
  einmal ausgegeben, nie verändert, sodass die Invariante URLs↔Bytes durch
  Konstruktion hält.
- **Sanftes Zurückfallen** - ein sicherer Rückfall bei jedem Schritt: 720p als
  Standardfähigkeit, Ausweichen von Hardware auf Software, Kopie→Transkodierung,
  mkvgo→ffprobe.

## Das Entscheidungsgehirn: der PlanResolver der Anwendungsschicht

Das Gehirn der Anfragebearbeitung - Festlegungen der Entscheidung, Formung der
Leiter, Hardwareweg, Zusammensetzen des Fingerabdrucks, Weg der Kopierstufe,
Wahl des Produzenten - liegt in der Anwendungsschicht: `PlanResolver`
(`app/transcoding/playback_plan_resolver.go`), der konkrete
`PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Jede
Anfrage wird **einmal** aufgelöst: Der http-Adapter übersetzt den
`*http.Request` in einen transportunabhängigen `PlaybackRequest`
(`http/transcode_vod_plan.go`) und liefert aus dem zurückgegebenen
unveränderlichen `PlaybackPlan` aus. Jeder Verbraucher - Eingabe des Manifests,
Abwurfprüfung, Versiegelung veralteter URLs, Wahl des Produzenten,
Segmentschlüssel - liest diese eine Auflösung, sodass die Invariante des
Cache-Hashs *innerhalb einer Anfrage* durch Konstruktion hält (und der heiße Pfad
der Segmente von ~5 Abrufen der Profilvorlieben auf 1 fiel). Die Hilfen für
Quelleigenschaften (Codec, HDR, SAR, 10 Bit…) liegen daneben in
`app/transcoding/source_facts.go`; der Adapter behält dünne Aliase. Über Anfragen
hinweg beruht die Invariante weiterhin auf dem Einbacken in URLs: **Jede Eingabe
der Entscheidung muss in die Kind-URLs eingebacken werden**, sonst kann eine
Abfrage mitten in der Wiedergabe den Plan unter dem Player kippen (`querySuffix`
in `manifest_generator.go`).
