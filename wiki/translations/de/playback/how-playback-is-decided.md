---
source_sha: af1b54b0c0e4fe2e86f3015114ce2667b287b7a2ec114f06ab77d2dc7f0de872
reviewed: true
---
# Wie Evey die Wiedergabeart wählt

Warum läuft ein Film auf dem Fernseher unverändert, wird auf dem Handy aber
umgewandelt?

**Kurz gesagt:**

- Sie wählen nie eine Qualität. Wenn Sie auf Abspielen drücken, prüft der
  Server die Datei, was Ihr Gerät abspielen kann und wie ausgelastet er
  selbst ist. Dann wählt er den leichtesten Weg, der
  funktioniert.
- Es gibt drei Wege: **Direktwiedergabe** (die Datei, wie sie ist), **Remux**
  (dasselbe Bild und derselbe Ton, in ein anderes Dateiformat umverpackt) und
  **Transkodierung** (ein Teil des Films wird umgewandelt, oft nur der Ton).
- Derselbe Film kann also unverändert an ein Gerät gehen und für ein anderes
  umgewandelt werden. Oder er läuft auf demselben Gerät anders, wenn der
  Server ausgelastet ist.
- Meist müssen Sie nichts tun. Welchen Weg Sie bekommen haben und warum,
  zeigt **Stream-Info** im Player: siehe [Stream-Info lesen](#reading-stream-info).

Zuerst kommen die drei Wege und wie man **Stream-Info** liest. Danach folgen
die genauen Regeln des Servers, für alle, die sie verstehen oder anpassen
möchten.

## Die drei Ergebnisse

| In Stream-Info | Was der Player erhält | Was es den Server kostet |
| --- | --- | --- |
| **Direktwiedergabe** | Die Originaldatei, Byte für Byte. | Das Lesen einer Datei. |
| **Remux** | Das Originalvideo und der Originalton in einem anderen Container. Nichts wird neu kodiert. | Das Umverpacken. Das erledigt [MKVGo](mkvgo.md) im Server, wo es geht, sonst eine Kopie mit FFmpeg. |
| **Transkodierung** | Mindestens eine Spur neu kodiert. Oft nur der Ton, das Video bleibt das Original. | Eine Kodierung. Nur eine Neukodierung des Videos belegt einen [Transkodierungsplatz](transcoding.md#sharing-the-capacity). |

Der Server versucht sie in dieser Reihenfolge. Bei einer Transkodierung
kodiert er nur die Spuren neu, bei denen es nötig ist.

## Stream-Info lesen

**Stream-Info** (die Informationstaste (i) in den Bedienelementen des Players;
auf dem Fernseher im Optionsmenü des Players) zeigt die **Methode**. Darunter
steht jede Spur mit ihrer Entscheidung (*kopiert*, *transkodiert* oder
*clientseitig dekodiert* für Ton, den der Web-Player selbst dekodiert). Unter
**Warum keine Direktwiedergabe** stehen die Gründe.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Stream-Info im Web-Player, bei einem Remux](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux: Die Verpackung ändert sich, die Video- und Tonspuren bleiben unverändert, und der Bereich zeigt den einzigen Grund, warum die Datei nicht so abgespielt wird, wie sie ist. „Kopiert“ beschreibt die Spuren, nicht die Wiedergabemethode (Oberfläche auf Englisch).</figcaption>
</figure>

Die Gründe fallen in vier Gruppen:

| Woher der Grund kommt | Beispiele, wie angezeigt |
| --- | --- |
| Der Player kann die Spur nicht so nutzen, wie sie ist | Videocodec, den dieser Player nicht dekodieren kann · Audiocodec, den dieser Player nicht dekodieren kann · Auflösung über dem Limit des Players · Mehr Audiokanäle, als dieser Player unterstützt · Pixelformat, das der Player nicht decodiert (10-Bit-H.264, 4:2:2) · Container nicht direkt abspielbar (zu HLS umverpackt) |
| Eine Grenze dieser Sitzung | Bitrate über dem erlaubten Limit · Die vom Client gemessene Leitung trägt die Kopie nicht |
| Eine Folge für Bild oder Ton | HDR nach SDR konvertiert (Tone-Mapping) · Untertitel ins Bild eingebrannt · Audio-Resynchronisation angefordert |
| Eine Einstellung oder eine bereits getroffene Entscheidung | Direktwiedergabe im Profil deaktiviert · Remux im Profil deaktiviert · Bevorzugter Video-Codec (Profileinstellung) · Auswahl für diese Sitzung fixiert |

Beurteilt wird nur die Tonspur, die gerade läuft. Hat eine Datei eine nicht
gewählte Kommentarspur in DTS, wird sie deswegen nicht transkodiert.

Stream-Info wird beim Start der Wiedergabe berechnet. Nur einen Fall kann sie
nicht vorhersehen: eine Grafikkarte, die genau beim Start des Streams voll
ist. Der Stream wechselt dann auf den Prozessor, und **Verwaltung → Aktuelle
Wiedergabe** zeigt, womit tatsächlich kodiert wird.

## Was jede Seite beiträgt

**Die Datei.** Codec, Auflösung, Farbtiefe, HDR-Format, Bitrate sowie die
Ton- und Untertitelspuren. Das alles wurde beim Scannen der Datei gelesen.

**Der Player.** Beim Start der Wiedergabe meldet die App, was ihr Gerät
dekodieren kann: die Codec-Familien, die es unterstützt, die Container, die es
öffnet, seine höchste Auflösung, ob sein Bildschirm HDR anzeigt, wie viele
Tonkanäle es ausgibt, und bei manchen Geräten eine niedrigere Obergrenze oder
ein niedrigeres Level für H.264. Die Web-App fragt den Browser; die Apps für
Android und Android TV fragen die Decoder und die Anzeige des Geräts. Evey
führt keine Liste unterstützter Geräte: Es nutzt, was der Player meldet. Eine
Cast-Sitzung beschreibt der Empfänger selbst, wenn er seine Fähigkeiten melden
kann, sonst ein vorsichtiges Profil: siehe [Streamen auf Chromecast](casting.md).

**Die Sitzung.** Die Vorlieben des Profils (höchste Bitrate, höchste Zahl der
Tonkanäle, Umgang mit Untertiteln) und die Grenzen, die für diesen Zuschauer
gelten (siehe unten).

**Der Server.** Ob er das in Echtzeit kodieren kann, auf seiner Grafikkarte
oder seinem Prozessor, und ob ein Transkodierungsplatz frei ist. Das wird
gemessen, nicht angenommen: siehe [Transkodierung und Serverkapazität](transcoding.md).

## Eine Datei, drei Ergebnisse

Ein 4K-Film in HEVC und HDR10 mit 70 Mb/s und einer Tonspur in Dolby TrueHD
7.1. Er läuft auf einem Android TV, dessen Decoder HEVC beherrscht und dessen
Bildschirm HDR anzeigt:

- **Keine Bitratengrenze.** Der Fernseher dekodiert das Video und zeigt das
  HDR an, also bleibt das Video, wie es ist. TrueHD dekodiert er nicht, also
  wird nur der Ton neu kodiert. Stream-Info zeigt **Transkodierung**, mit dem
  Video *kopiert*.
- **Eine Grenze von 20 Mb/s und eine Grafikkarte, die HEVC 10 Bit mit HDR
  kodiert.** Das Video muss neu kodiert werden, damit es unter die Grenze
  passt. Der Server kodiert es in HEVC und behält das HDR, weil sowohl der
  Fernseher als auch die Karte das können.
- **Dieselbe Grenze, aber die Grafikkarte ist ausgelastet oder für diesen
  Titel nicht verfügbar.** Der Prozessor übernimmt. Er kodiert H.264, das hier
  kein HDR trägt. Das Bild wird also per Tone-Mapping auf SDR umgerechnet, in
  der Qualität, die der Prozessor durchhält.

Die Datei und der Fernseher haben sich nicht geändert. Geändert haben sich die
Grenze der Sitzung und der Zustand des Servers.

## Grenzen der Sitzung

Liegt die Bitrate eines Titels (Video plus die Tonspur, die gerade läuft) über
der niedrigsten dieser Grenzen, wird er auf diese Grenze heruntertranskodiert:

- das Maximum, das der Player meldet;
- die **Maximale Bitrate** des Kontos, von einem Administrator festgelegt;
- die **Maximale Streaming-Bitrate** des Profils, und seine **Maximale
  Mobilfunk-Bitrate**, wenn ein Handy eine Mobilfunkverbindung meldet;
- **Max. Remote-Bitrate** (**Verwaltung → Fernzugriff**) und
  **Video-Bitratenobergrenze, entfernte Zuschauer** (**Verwaltung →
  Transkodierung**), die niedrigere der beiden, wenn die Adresse des
  Zuschauers öffentlich ist und außerhalb der **LAN-Netzwerke** liegt. Eine
  private Adresse wird nie begrenzt.

Die Tonspur, die gerade läuft, wird neu kodiert, wenn sie mehr Kanäle hat, als
der Player meldet oder als die **Maximale Audio-Kanäle** des Profils erlauben.
Eine neu kodierte Spur behält bis zu 5.1 Kanäle, nie mehr: siehe
[Audio](audio.md).

## Einschränkungen, die der Server selbst anwendet

Einige Grenzen melden die Player heute nicht. Der Server wendet sie als
vorsichtige Regel für die Kompatibilität an, gleich für jeden Player: H.264
mit 10 Bit und jedes Video in 4:2:2, 4:4:4 oder 12 Bit werden neu kodiert.
HEVC, VP9 und AV1 bis 10 Bit 4:2:0 hängen dagegen davon ab, welche Codecs der
Player unterstützt. Dolby Vision bleibt nur in einer Form erhalten, die der
Player richtig anzeigen kann: siehe [HDR und Dolby Vision](hdr.md).

## Warum sich die Methode mitten im Film selten ändert

Die Entscheidung fällt beim Start der Wiedergabe und gilt für die ganze
Sitzung. So erhält der Player nie Segmente aus zwei verschiedenen Kodierungen.
Eine neue Entscheidung gibt es, wenn Sie die Ton- oder Untertitelspur so
wechseln, dass es nötig ist, oder wenn Sie den Titel neu starten.

## Eine Methode erzwingen

**Einstellungen → Wiedergabe** bietet pro Profil **Direktwiedergabe
erlauben**, **Remux erlauben** und **Transkodierung erlauben**. Schalten Sie
eine davon aus, überspringt der Server diese Methode, und Stream-Info zeigt den
Grund.

!!! note "Hinweis"
    Das ist ein Werkzeug zur Diagnose, keine Qualitätseinstellung: Eine
    erzwungene Transkodierung ist nie besser als das Original.

**Qualität** in den Einstellungen des Players ist etwas anderes: Damit wählen
Sie zwischen den Qualitäten eines transkodierten Streams. **Auto** folgt der
Verbindung; eine Bildhöhe (1080p, 720p...) hält diese Qualität; **Original**
hebt die Begrenzung der Bildhöhe auf, die der Player sonst anfordern würde.

## Weiterführend

- Die Kriterien im Detail: [HDR und Dolby Vision](hdr.md) · [Audio](audio.md) ·
  [Untertitel](subtitles.md) · [Streamen auf Chromecast](casting.md)
- Wie der Server seine Kapazität teilt: [Transkodierung und Serverkapazität](transcoding.md)
- Was ohne FFmpeg im Server läuft: [MKVGo in Evey](mkvgo.md)
- Die Umsetzung: [Wiedergabekette](../playback-serving.md)

<!-- sources
decision: internal/app/transcoding/decision.go - MakePlaybackDecisionWithPrefs, evaluateVideo, pixelFormatPlayable, evaluateAudio, selectedAudioSpec, servedBitrateBps, resolveMaxBitrate, checkBitrate, applyAudioChannelTarget (aacMaxTargetChannels), applyDirectPlayPrefs
session: internal/app/transcoding/playback_plan_resolver.go - Decide, Reported, resolveDecided ; internal/app/transcoding/resolved_route.go - SpillReason (pool_full, title_blocked)
remote cap: internal/adapters/driving/grpcapi/playback_handler.go - resolvePlaybackPrefs ; internal/config/config.go - RemoteBitrateCapKbps
client caps: internal/domain/transcoding/playback_decision.go - ClientProfile ; frontend/packages/client/src/web/codec.ts ; frontend/packages/media-caps/src/castUrl.ts
example: internal/app/transcoding/testdata/route_golden.txt - "nvenc-pass | 2160p-hevc-hdr10-70m | hdr-tv{,-20m} | {plain,gpu-full,title-blocked}"
tone-map reported: internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.* ; player.quality.* ; auth.settings.playback.* ; admin.yaml maxBitrate (user limit) ; admin.remoteAccess.maxRemoteBitrate, lanNetworks ; admin.calibration.maxBitrateRemoteLabel
entry point: frontend/apps/web/src/features/playback/StreamInfoPanel.tsx ; frontend/apps/web/src/features/playback/PlayerSettingsPanel.tsx ; frontend/apps/mobile/src/components/player/PlayerControlsRow.tsx ; frontend/apps/tv/src/components/TrackOptionsMenu.tsx
adr: 0024, 0029, 0030
plain opening: docs/playback/transcoding.md ; i18n player.streamInfo.title
-->
