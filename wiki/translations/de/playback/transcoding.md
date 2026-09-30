---
source_sha: c8d03e544227da6967bbbab98675cdb62fcd32e683e410544a6fffc79d40c1d5
reviewed: false
---
# Transkodierung und Serverkapazität

Die Transkodierung ist das teure Ergebnis der
[Wiedergabeentscheidung](how-playback-is-decided.md): Direktwiedergabe und
Remux kosten den Server fast nichts und belegen nie einen
Transkodierungsplatz. Diese Seite erklärt, woher der Server weiß, was er
kodieren kann, wie er diese Kapazität unter den Zuschauern aufteilt und was
passiert, wenn er voll ist.

## Der Server vermisst sich selbst

Die Kapazität ist keine Einstellung, die Sie schätzen. Der Server misst sie:

- **Wann.** Nicht beim Start: Der Server startet mit dem, was er vorher wusste
  (oder beim ersten Mal mit einer vorsichtigen Standardvorgabe von 720p in
  Software), und führt die Messung im ersten ruhigen Moment aus, wenn niemand
  schaut und der Prozessor weitgehend frei ist. Sie dauert einige Minuten. Eine
  unter Last durchgeführte Messung wird als vorläufig markiert und wiederholt,
  sobald die Maschine frei ist.
- **Was.** Echte Kodierungen eines 4K-Clips, in SDR und in HDR, auf dem
  Prozessor und auf jedem erkannten Beschleuniger, in derselben Form wie eine
  echte Wiedergabe. Das Ergebnis ist die höchste Auflösung, die jeder in
  Echtzeit hält, und die Kosten jeder Art von Stream.
- **Erneut.** Nachdem sich die Hardware geändert hat, oder mit **Neu
  kalibrieren** unter **Admin → Transkodierung → Encode-Kalibrierung**.

Während die Messung läuft, zeigt eine Wiedergabe, die eine Transkodierung
braucht, **Der Server kalibriert sich** und wird gebeten, es kurz darauf erneut
zu versuchen. Die Direktwiedergabe ist nicht betroffen.

Auf einem Prozessor, der keine der Auflösungen hält, bietet der Server trotzdem
720p (SDR) und 480p (HDR) an, jeweils einen Stream.

Die Messung verwendet, in dieser Reihenfolge, einen von Ihnen bereitgestellten
Clip, einen einmal heruntergeladenen und per SHA256 geprüften Referenzclip oder
einen vor Ort erzeugten synthetischen Clip. Ein echter Clip misst ehrlicher als
der synthetische, der die Kosten überschätzt und eine Maschine niedriger
begrenzen kann, als sie verdient:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## Die Kapazität teilen

**Kapazität für laufende Wiedergabe** (**Admin → Transkodierung**) ist die Zahl
der Videostreams, die gleichzeitig kodiert werden dürfen. Sie wird aus der
Messung abgeleitet und passt sich von selbst an, wenn sich die Hardware ändert.
**Gleichzeitige Videostreams begrenzen** kann sie senken, nie erhöhen.

- Eine Transkodierung, die nur das Audio neu kodiert (Video kopiert), belegt
  keinen Platz.
- Ein Zuschauer, der die Qualität wechselt, behält einen einzigen Platz.
- Nur die Qualität, die ein Zuschauer tatsächlich schaut, wird kodiert, nicht
  die ganze Qualitätsleiter.

Jeder Stream geht an den günstigsten Weg, der ihn bedienen kann: einen
Beschleuniger, der bewiesen hat, was der Stream braucht (zum Beispiel HDR-Tone-
Mapping oder HEVC-Ausgabe), oder den Prozessor. Prozessor und Beschleuniger
sind zwei getrennte Pools. Sind die Beschleuniger voll, kann ein neuer Stream
auf den Prozessor wechseln, wenn dieser Platz hat; hat keiner Platz, zeigt der
Player **Der Server transkodiert gerade andere Streams** und kann es erneut
versuchen.

Während jemand schaut, pausieren die eigenen Hintergrundaufgaben des Servers
(Vorschaubilder, Intro-Erkennung, Medienanalyse), damit die Wiedergabe die
Maschine behält: siehe [Hintergrundaufgaben und Serverlast](../operations/background-work.md).

Wenn eine bestimmte Art von Quelle Zuschauer trotz allem immer wieder warten
lässt, senkt der Server eine Zeit lang ihre Höchstqualität und hebt die Grenze
dann von selbst wieder auf. Siehe
[Überwachung und Logs](../operations/monitoring.md#how-playback-actually-went).

## Was kodiert wird

| | Standard |
| --- | --- |
| Videocodec | H.264. HEVC, wenn der Player es dekodiert und die Grafikkarte es kodiert; AV1, wenn aktiviert (**AV1-Ausgabe**, nur Hardware) und der Player es dekodiert. |
| Auflösung | Die der Quelle, sofern keine Grenze greift: das Maximum des Players, die Bitrate-Grenze oder das, was der Server in Echtzeit kodieren kann. |
| Bitrate | Folgt der Quelle, skaliert auf die Auflösung. **Obergrenze Videobitrate** begrenzt sie. |
| Audio | AAC. Siehe [Audio](audio.md). |
| Auslieferung | HLS, in fragmentierten MP4-Segmenten. |

<figure class="evey-shot" markdown="span">
  ![Die zwei Videobitrate-Obergrenzen unter Admin → Transkodierung](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Die zwei Obergrenzen unter <strong>Admin → Transkodierung</strong>: für jeden Zuschauer und für Zuschauer außerhalb der lokalen Netzwerke (Oberfläche auf Englisch).</figcaption>
</figure>

## Der Transkodierungs-Cache

Kodierte Segmente werden auf der Festplatte im Volume `evey-cache` aufbewahrt
und wiederverwendet, wenn ein anderer Zuschauer, oder derselbe später, denselben
Titel mit derselben Entscheidung abspielt. **Admin → Transkodierungs-Cache**
zeigt seine Größe und leert ihn; das **Budget des Transcode-Caches** (**Admin →
Serverkonfiguration → Servereinstellungen**) begrenzt ihn. Leeren Sie ihn nach
einem Update, das ändert, wie Videos kodiert werden, damit niemand Segmente der
vorherigen Version erhält.

## Prüfen, was passiert

- **Admin → Aktuelle Wiedergabe**: jeder Stream, seine Methode Spur für Spur
  und das Backend, das ihn kodiert.
- **Admin → Transkodierung → Encode-Kalibrierung**: die gemessenen Obergrenzen
  und die genutzten Beschleuniger.
- **Admin → Streaming-Qualität**: wie die Wiedergabe für echte Zuschauer lief.
- **Lasttest** (**Admin → Transkodierung**): simuliert Zuschauer, um zu messen,
  was der Server hält. Nur ausführen, wenn niemand schaut.
