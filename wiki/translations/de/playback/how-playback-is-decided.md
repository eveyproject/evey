---
source_sha: 1ba4c65cfb14b1f65376a6dcc31dd7a325cfffb9e3f20082f430ba214a292009
reviewed: false
---
# Wie Evey die Wiedergabeart wählt

Evey verlangt nie, dass Sie vor dem Abspielen eine Qualität wählen. Jedes Mal,
wenn ein Player einen Titel startet, ermittelt der Server aus drei Dingen, wie
er ihn ausliefert: was die Datei enthält, was dieser Player dekodieren kann und
was dieser Server gerade kodieren kann. Derselbe Film kann daher unverändert an
ein Gerät gehen und für ein anderes neu kodiert werden, oder auf demselben Gerät
anders laufen, wenn der Server ausgelastet ist.

## Die drei Ergebnisse

| In Stream-Info | Was der Player erhält |
| --- | --- |
| **Direktwiedergabe** | Die Originaldatei, Byte für Byte. |
| **Remux** | Das Originalvideo und -audio in einem anderen Container. Nichts wird neu kodiert. |
| **Transkodierung** | Mindestens eine Spur neu kodiert. Oft nur das Audio, während das Video das Original bleibt. |

Der Server bevorzugt sie in dieser Reihenfolge, und innerhalb einer
Transkodierung kodiert er nur die Spuren neu, die es brauchen.

## Was jede Seite beiträgt

**Die Datei.** Codec, Auflösung, Farbtiefe, HDR-Format, Bitrate sowie die Audio-
und Untertitelspuren, gelesen beim Scannen der Datei.

**Der Player.** Beim Start der Wiedergabe meldet die App, was ihr Gerät
dekodieren kann: die unterstützten Codec-Familien, die Container, die es öffnet,
seine maximale Auflösung, ob sein Bildschirm HDR darstellt, wie viele
Audiokanäle es ausgibt und bei manchen Geräten eine niedrigere Obergrenze für
H.264. Die Web-App fragt den Browser; die Apps für Android und Android TV
fragen die Decoder und die Anzeige des Geräts. Evey führt keine Liste
unterstützter Geräte: Es nutzt, was der Player meldet. Eine Cast-Sitzung wird
vom Empfänger selbst beschrieben, wenn er seine Fähigkeiten melden kann, und
sonst durch ein vorsichtiges Profil: siehe [Streamen auf Chromecast](casting.md).

**Die Sitzung.** Die Vorlieben des Profils (maximale Bitrate, maximale
Audiokanäle, Umgang mit Untertiteln) und die Grenzen, die für diesen Zuschauer
gelten, siehe unten.

**Der Server.** Ob er das in Echtzeit kodieren kann, auf seiner Grafikkarte
oder seinem Prozessor, und ob ein Transkodierungsplatz frei ist. Das wird
gemessen, nicht angenommen: siehe [Transkodierung und Serverkapazität](transcoding.md).

## Eine Datei, drei Ergebnisse

Ein 4K-HEVC-Film in HDR10 mit 70 Mb/s und einer Dolby-TrueHD-7.1-Spur,
abgespielt auf einem Android TV, dessen Decoder HEVC beherrscht und dessen
Bildschirm HDR darstellt:

- **Keine Bitrate-Grenze.** Der Fernseher dekodiert das Video und zeigt das HDR
  an, also bleibt das Video, wie es ist. TrueHD dekodiert er nicht, also wird
  nur das Audio neu kodiert. Stream-Info zeigt **Transkodierung**, mit dem Video
  *kopiert*.
- **Eine Grenze von 20 Mb/s und eine Grafikkarte, die HEVC 10 Bit mit HDR
  kodiert.** Das Video muss neu kodiert werden, um hineinzupassen. Der Server
  kodiert es in HEVC und behält das HDR, weil sowohl der Fernseher als auch die
  Karte es können.
- **Dieselbe Grenze, bei ausgelasteter oder für diesen Titel nicht verfügbarer
  Grafikkarte.** Der Prozessor übernimmt. Er kodiert H.264, das hier kein HDR
  trägt, also wird das Bild per Tone Mapping auf SDR umgerechnet.

Die Datei und der Fernseher haben sich nicht geändert. Die Grenze der Sitzung
und der Zustand des Servers schon.

## Einschränkungen, die der Server selbst anwendet

Einige Grenzen melden die Player heute nicht, und der Server wendet sie als
vorsichtige Kompatibilitätsregel an, gleich für jeden Player: H.264 mit 10 Bit,
4:2:2- und 12-Bit-Video werden neu kodiert, während HEVC, VP9 und AV1 bis
10 Bit 4:2:0 der Codec-Unterstützung des Players überlassen bleiben. Dolby
Vision wird nur in einer Form beibehalten, die der Player korrekt darstellen
kann: siehe [HDR und Dolby Vision](hdr.md).

## Grenzen der Sitzung

Ein Titel, dessen Bitrate (Video plus die abgespielte Audiospur) über der
niedrigsten dieser Grenzen liegt, wird darauf heruntertranskodiert:

- das Maximum, das der Player meldet;
- die **Maximale Bitrate** des Kontos, von einem Administrator festgelegt;
- die **Maximale Streaming-Bitrate** des Profils, und seine **Maximale
  Mobilfunk-Bitrate**, wenn ein Telefon eine Mobilfunkverbindung meldet;
- **Max. Remote-Bitrate** (**Admin → Fernzugriff**) und die Obergrenze für
  entfernte Zuschauer unter **Admin → Transkodierung**, die niedrigere der
  beiden, wenn die Adresse des Zuschauers öffentlich ist und außerhalb der
  **LAN-Netzwerke** liegt. Eine private Adresse wird nie begrenzt.

## Stream-Info lesen

**Stream-Info** (die Informationsschaltfläche (i) in den Bedienelementen des
Players; auf dem Fernseher im Optionsmenü des Players) zeigt die Methode, dann
jede Spur mit ihrer Entscheidung (*kopiert*, *transkodiert* oder *clientseitig
dekodiert* für Audio, das der Web-Player selbst dekodiert) und unter **Warum
keine Direktwiedergabe** die Gründe.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream-Info im Web-Player, bei einem Remux](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>Ein Remux: Video und Audio kopiert, und der einzige Grund, warum die Datei nicht so abgespielt wird, wie sie ist (Oberfläche auf Englisch).</figcaption>
</figure>

Die Gründe fallen in vier Gruppen:

| Woher der Grund kommt | Beispiele, wie angezeigt |
| --- | --- |
| Der Player kann die Spur nicht so nutzen, wie sie ist | Videocodec, den dieser Player nicht dekodieren kann · Audiocodec, den dieser Player nicht dekodieren kann · Auflösung über dem Limit des Players · Mehr Audiokanäle, als dieser Player unterstützt · Container nicht direkt abspielbar |
| Eine Grenze dieser Sitzung | Bitrate über dem erlaubten Limit · Die vom Client gemessene Leitung trägt die Kopie nicht |
| Eine Folge für Bild oder Ton | HDR nach SDR konvertiert (Tone-Mapping) · Untertitel ins Bild eingebrannt · Audio-Resynchronisation angefordert |
| Eine Einstellung oder eine bereits getroffene Entscheidung | Direktwiedergabe im Profil deaktiviert · Remux im Profil deaktiviert · Bevorzugter Video-Codec (Profileinstellung) · Auswahl für diese Sitzung fixiert |

Beurteilt wird nur die abgespielte Audiospur. Eine Datei, deren nicht gewählte
Kommentarspur in DTS vorliegt, wird deswegen nicht transkodiert.

Stream-Info wird beim Start der Wiedergabe berechnet. Der einzige Fall, den sie
nicht vorhersehen kann, ist eine Grafikkarte, die im Moment des Stream-Starts
voll ist: Der Stream wechselt dann auf den Prozessor, und **Admin → Aktuelle
Wiedergabe** zeigt das tatsächlich genutzte Backend.

## Warum sich die Methode mitten im Film selten ändert

Die Entscheidung fällt beim Start der Wiedergabe und bleibt für die Sitzung
bestehen, sodass der Player nie Segmente aus zwei verschiedenen Kodierungen
erhält. Ein Wechsel der Audio- oder Untertitelspur, der das erfordert, oder ein
Neustart des Titels führt zu einer neuen Entscheidung.

## Eine Methode erzwingen

**Einstellungen → Wiedergabe** bietet pro Profil **Direktwiedergabe
erlauben**, **Remux erlauben** und **Transkodierung erlauben**. Ist eine davon
aus, überspringt der Server diese Methode, und Stream-Info zeigt den Grund. Das
ist ein Diagnosewerkzeug, keine Qualitätseinstellung: Eine erzwungene
Transkodierung ist nie besser als das Original.

**Qualität** in den Einstellungen des Players ist etwas anderes: Sie wählt
zwischen den Qualitäten eines transkodierten Streams. **Auto** folgt der
Verbindung; eine Höhe (1080p, 720p...) hält diese Qualität; **Original** hebt
die Höhenbegrenzung auf, die der Player sonst anfordern würde.

## Weiterführend

- Die Kriterien im Detail: [HDR und Dolby Vision](hdr.md) · [Audio](audio.md) ·
  [Untertitel](subtitles.md) · [Streamen auf Chromecast](casting.md)
- Wie der Server seine Kapazität teilt: [Transkodierung und Serverkapazität](transcoding.md)
- Die Umsetzung: [Wiedergabekette](../playback-serving.md)
