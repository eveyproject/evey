---
source_sha: e039606b33ab2207d3774f9bacb71ee3d18f0d12f52d9dc02bec1e9817e1c91e
reviewed: true
---
# Transkodierung und Serverkapazität { #transcoding-and-server-capacity }

**Kurz gesagt:**

- **Transkodieren** heißt, ein Video in eine Form umzuwandeln, die das Gerät
  abspielen kann. Das ist die einzige aufwendige Arbeit des Servers: Eine Datei
  so abzuspielen, wie sie ist (Direktwiedergabe), oder sie umzuverpacken
  (Remux), kostet fast nichts. Siehe
  [Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md).
- Sie legen nicht fest, wie viele Personen der Server bedienen kann. Er misst
  sich selbst in seinem ersten ruhigen Moment, in wenigen Minuten, und nimmt
  nie mehr an.
- Wird es knapp, bekommt eine neue Wiedergabe eine niedrigere Qualität, oder
  sie wechselt von einer ausgelasteten Grafikkarte auf den Prozessor. Hat
  keiner von beiden Platz, zeigt der Player **Der Server transkodiert gerade
  andere Streams**.
- Was Sie tun können: dem Server eine Grafikkarte geben
  ([Hardware-Beschleunigung](../administration/hardware-acceleration.md)) und
  auf der Festplatte mit dem Cache Platz frei halten
  ([Der Transkodierungs-Cache](#the-transcoding-cache)).

Der Rest dieser Seite ist für Administratoren, die es genau wissen wollen: wie
der Server misst, was er kodieren kann, wie er diese Kapazität aufteilt und
was ein Zuschauer sieht, wenn sie erschöpft ist.

## Der Server misst sich selbst { #the-server-measures-itself }

Die Kapazität ist keine Einstellung, die Sie schätzen. Der Server misst sie:

- **Wann.** Nicht beim Start. Der Server startet mit dem, was er vorher wusste
  (beim ersten Mal mit einer vorsichtigen Vorgabe: 720p, nur Prozessor). Die
  Messung führt er in seinem ersten ruhigen Moment aus, wenn niemand schaut und
  der Prozessor fast nichts zu tun hat. Sie dauert einige Minuten. Eine
  Messung unter Last gilt als vorläufig und wird wiederholt, sobald die
  Maschine nicht mehr ausgelastet ist.
- **Was.** Echte Kodierungen eines 4K-Clips, in SDR und in HDR, auf dem
  Prozessor und auf jedem Beschleuniger, den der Server erkannt hat. Sie laufen
  genauso ab wie bei einer echten Wiedergabe. Das Ergebnis: die höchste
  Auflösung, die jeder in Echtzeit schafft, und was jede Art von Stream kostet.
- **Erneut.** Nachdem sich die Hardware geändert hat, oder mit **Neu
  kalibrieren** unter **Verwaltung → Transkodierung → Encode-Kalibrierung**.
  Die Schaltfläche lehnt ab, solange jemand schaut; nur die automatische Neumessung wartet, bis der Server im Leerlauf ist.

Während die Messung läuft, zeigt eine Wiedergabe, die eine Transkodierung
braucht, **Der Server kalibriert sich**. Der Player erhält die Anweisung, es kurz
darauf erneut zu versuchen. Direktwiedergabe und Remux sind nicht betroffen.

Schafft ein Prozessor keine der Auflösungen, bietet der Server trotzdem 720p
(SDR) und 480p (HDR) an, wie viele Streams das gleichzeitig sind, ergibt sich aus dem Budget des Limiters für Videositzungen.

Für die Messung nimmt der Server, in dieser Reihenfolge: einen Clip, den Sie
bereitstellen, einen Referenzclip, der einmal heruntergeladen und über seine
SHA256 geprüft wird, oder einen künstlichen Clip, der vor Ort erzeugt wird.
Ein echter Clip liefert genauere Messwerte als der künstliche. Der künstliche
überschätzt die Kosten und kann eine Maschine niedriger einstufen, als es ihrer
tatsächlichen Leistung entspricht:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## Die Kapazität teilen { #sharing-the-capacity }

**Kapazität für laufende Wiedergabe** (**Verwaltung → Transkodierung**) ist
die Zahl der Videostreams, die gleichzeitig kodiert werden dürfen. Sie ergibt
sich aus der Messung und passt sich von selbst an, wenn sich die Hardware
ändert. **Gleichzeitige Videostreams begrenzen** kann sie senken, nie erhöhen.

- Eine Transkodierung, die nur den Ton neu kodiert (Video kopiert), belegt
  keinen Platz.
- Ein Zuschauer, der die Qualität wechselt, behält einen einzigen Platz.
- Kodiert wird nur die Qualität, die ein Zuschauer wirklich schaut, nicht alle
  Qualitätsstufen.

Jeder Stream nimmt den günstigsten Weg, der nachgewiesen hat, dass er ihn
bedienen kann: einen Beschleuniger, der genau auf das gemessen wurde, was der
Stream braucht (zum Beispiel HDR-Tone-Mapping oder Ausgabe in HEVC), oder den
Prozessor. Prozessor und Beschleuniger sind zwei getrennte Pools. Ein Stream
auf der Grafikkarte braucht trotzdem ein wenig Prozessor, für seinen Ton und
seine Verpackung.

Wird der Platz knapp, reagiert der Server in dieser Reihenfolge:

1. **Eine niedrigere Qualität.** Passt die höchste Qualität eines neuen
   Streams nicht, wird er mit der höchsten zugelassen, die passt. Diese
   Obergrenze gilt dann für den ganzen Stream.
2. **Der Prozessor.** Sind die Beschleuniger voll, wechselt ein neuer Stream
   auf den Prozessor, wenn dieser Platz hat.
3. **Eine Ablehnung.** Hat keiner von beiden Platz, zeigt der Player **Der
   Server transkodiert gerade andere Streams** und kann es erneut versuchen.

Scheitert eine Kodierung auf der Grafikkarte bei einem Titel, geht dieser
Titel eine Zeit lang an den Prozessor, für alle Zuschauer. Die Karte selbst
wird erst dann nicht mehr verwendet, wenn auch eine Testkodierung scheitert, die nichts
mit dem Medium zu tun hat. Später wird sie erneut versucht: siehe
[Hardware-Beschleunigung](../administration/hardware-acceleration.md).

Während jemand schaut, pausieren die eigenen Hintergrundarbeiten des Servers
(Vorschaubilder, Intro-Erkennung, Medienanalyse). So steht der Wiedergabe die
gesamte Leistung der Maschine zur Verfügung: siehe
[Hintergrundarbeiten und Serverlast](../operations/background-work.md).

Lässt eine bestimmte Art von Quelle die Zuschauer trotz allem immer wieder
warten, senkt der Server eine Zeit lang ihre höchste Qualität. Danach hebt er
die Grenze von selbst wieder auf. Siehe
[Überwachung und Protokolle](../operations/monitoring.md#how-playback-actually-went).

## Was kodiert wird { #what-is-encoded }

| | Standard |
| --- | --- |
| Videocodec | H.264. HEVC-Ausgabe, wenn eine gemessene Route sie belegt und der Player HEVC dekodiert (die HDR-Passthrough-Route, oder wenn der **Bevorzugte Video-Codec** des Profils es verlangt); AV1, wenn eingeschaltet (**AV1-Ausgabe**, nur Hardware) und der Player es dekodiert. |
| Auflösung | Die der Quelle, außer eine Grenze greift: das Maximum des Players, die Bitratengrenze oder das, was der Server in Echtzeit kodieren kann. |
| Bitrate | Folgt der Quelle, angepasst an die Auflösung. **Obergrenze Videobitrate** begrenzt sie; **Video-Bitratenobergrenze, entfernte Zuschauer** begrenzt sie außerhalb der lokalen Netzwerke stärker. |
| Ton | AAC. Siehe [Audio](audio.md). |
| Auslieferung | HLS, in fragmentierten MP4-Segmenten. |

<figure class="evey-shot" markdown="span">
  ![Die zwei Obergrenzen der Videobitrate unter Verwaltung → Transkodierung](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Die zwei Obergrenzen unter <strong>Verwaltung → Transkodierung</strong>: für jeden Zuschauer und für Zuschauer außerhalb der lokalen Netzwerke (Oberfläche auf Englisch).</figcaption>
</figure>

## Der Transkodierungs-Cache { #the-transcoding-cache }

Kodierte Segmente werden auf der Festplatte im Volume `evey-cache`
aufbewahrt. Sie werden wiederverwendet, wenn ein anderer Zuschauer, oder
derselbe später, denselben Titel mit derselben Entscheidung abspielt.
**Verwaltung → Transkodierungs-Cache** zeigt seine Größe und leert ihn; das
**Budget des Transkodierungs-Caches (GB)** (**Verwaltung → Transkodierung**) begrenzt ihn. **Alle leeren** entfernt auch die
Untertitel, die aus Ihren Dateien extrahiert wurden und in
`./evey-data/subtitles` liegen: Sie werden bei Bedarf erneut extrahiert.

**Wenn die Festplatte voll wird.** Der Server hält auf der Festplatte mit dem
Cache eine Reserve an freiem Platz frei: standardmäßig 2 GiB, einstellbar mit
`EVEY_TRANSCODE_CACHE_MIN_FREE_MB` (`0` schaltet diese Reserve ab). Wird sie
unterschritten, werden zuerst die ältesten Segmente gelöscht, die niemand
schaut. Reicht das nicht, wird ein Segment, das es noch nicht gibt,
abgelehnt, mit einer Wartezeit, nach der der Player erneut fragen darf. So
startet kein Encoder, der mangels Platz scheitern würde. Segmente, die schon
auf der Festplatte liegen, werden weiter ausgeliefert. Für die
extrahierten Untertitel in `./evey-data` gilt dieselbe Reserve: Wird sie
unterschritten, werden neue nicht mehr aufbewahrt, bei jeder Anfrage werden sie
neu extrahiert, und die Administratoren erhalten **Untertitel-Cache voll** (höchstens
einmal pro Stunde, auch an Webhooks gesendet als
`transcoding.derived_cache_full`).

!!! tip "Tipp"
    Leeren Sie den Cache nach einem Update, das ändert, wie Videos kodiert
    werden. So bekommt niemand Segmente der vorherigen Version. Leeren Sie ihn,
    während jemand schaut, puffert dessen Stream einmal neu und läuft dann
    weiter.

## Prüfen, was passiert { #checking-what-happens }

- **Verwaltung → Aktuelle Wiedergabe**: jeder Stream, seine Methode Spur für
  Spur und womit er kodiert wird.
- **Verwaltung → Transkodierung → Encode-Kalibrierung**: die gemessenen
  Obergrenzen und die genutzten Beschleuniger.
- **Verwaltung → Streaming-Qualität**: wie die Wiedergabe für echte Zuschauer
  lief.
- **Lasttest** (**Verwaltung → Transkodierung**): simuliert Zuschauer, um zu
  messen, was der Server schafft. Er startet nicht, solange jemand schaut.

<!-- sources
disk floor: cmd/evey/wire.go (EVEY_TRANSCODE_CACHE_MIN_FREE_MB, DefaultMinFreeBytes, SetWriteFloor, SetSubtitleCacheWriteFloor, OnSubtitleCacheFloorHit) ; internal/app/transcoding/segment_cache_admit.go - AdmitWrite, tryOnDemandEvict, ErrCacheFull ; internal/adapters/driving/http/transcode_vod_admission.go (503 + Retry-After) ; internal/domain/transcoding/events.go - DerivedCacheFull ; i18n notifications.cache_full
clear all empties extracted subtitles: internal/app/transcoding/service.go - WithDerivedCachePurge ; internal/adapters/driving/http/subtitle_cache.go - SubtitleCachePurger (/data/subtitles)
calibration: internal/app/transcoding/calibration_service.go - EnsureCalibrated, Recalibrate, idleProbes, idleCPUPercent ; internal/domain/transcoding/capability.go - SafeFallback ; internal/adapters/driven/ffmpeg/benchmark.go - swBenchLadderSpecs (floors 720/480), sample order (path > url+sha256 > synthetic) ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md ; i18n player.calibrating, admin.calibration.contendedLine, transcodesActiveToast
capacity: i18n admin.transcoding.livePlaybackTitle, maxVideoSessionsDesc, videoSessionsAutoAdjusts ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper, StreamPlan ; internal/app/transcoding/video_session_limiter.go - VideoSessionLimiter (PoolCPU, PoolGPU) ; internal/app/transcoding/resolved_route.go - SpillPoolFull, SpillTitleBlocked ; internal/app/transcoding/hw_route_blocklist.go ; CalibrationService.DemoteCapability
busy: i18n errors max_concurrent_reached
output codec: internal/app/transcoding/resolver_route.go ; internal/app/transcoding/decision.go ; internal/app/transcoding/testdata/route_golden.txt ; i18n admin.calibration.av1Label, av1Hint, maxBitrateLabel, maxBitrateRemoteLabel
single variant: internal/config/config.go (EVEY_TRANSCODING_SINGLE_VARIANT, default on)
bench samples: internal/config/config.go (EVEY_BENCH_{SDR,HDR}_SAMPLE{,_URL,_SHA256})
cache budget, clear while live: i18n admin.serverSettings.transcodeCacheMaxGB, admin.transcodeCache.clearConfirmLiveSessions ; automatic limits: admin.qoe.overrides.*
load test: i18n admin.loadtest.idleWarning, errorActiveStreams
plain opening, busy message: i18n errors max_concurrent_reached ; internal/app/transcoding/admission_shaper.go - VideoAdmissionShaper.GrantRoute
-->
