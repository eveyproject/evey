---
source_sha: a631c73d13e67b6253b0aa9e7c5c362475b1e0fcb6153effecea88bd092feca5
reviewed: true
---
# HDR und Dolby Vision

**Kurz gesagt:**

- **HDR** ist ein Bild mit helleren Spitzlichtern und satteren Farben als das
  übliche Bild (genannt **SDR**). Dolby Vision ist eine Art von HDR. HDR
  braucht einen Bildschirm, der es anzeigen kann.
- Auf einem HDR-Bildschirm mit der App für Android oder Android TV (und einem
  Gerät, das HEVC dekodiert) bekommen Sie das originale HDR-Bild, solange
  nichts anderes eine Umwandlung erzwingt.
- In einem Webbrowser und auf jedem Bildschirm ohne HDR wandelt der Server das
  Bild in SDR um. Das ist Absicht: HDR auf einem SDR-Bildschirm sieht grau und
  ausgewaschen aus.
- HDR in SDR umzuwandeln ist die aufwendigste Arbeit, die der Prozessor leisten
  kann. Eine Grafikkarte hilft: siehe
  [Hardware-Beschleunigung](../administration/hardware-acceleration.md).

Der Rest dieser Seite erklärt jeden Fall: das Originalbild, eine
Neukodierung, die das HDR behält, oder eine Umwandlung in SDR. Die allgemeinen
Regeln der Wiedergabe stehen unter
[Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md).

## Was Sie sehen, je nach Gerät

| Ihr Bildschirm | Was ankommt |
| --- | --- |
| Ein HDR-Bildschirm mit Android TV oder einem Android-Handy, mit einem HEVC-Decoder | Das HDR10- oder HLG-Original, unverändert, außer etwas anderes (Bitrate, Auflösung, ein eingebrannter Untertitel) erzwingt eine Neukodierung. |
| Ein Browser | Immer eine Umwandlung in SDR: Der Web-Player meldet keinen HDR-Bildschirm, also rechnet der Server das Bild per Tone-Mapping für ihn um. |
| Ein Cast-Empfänger | Was der Empfänger über seinen eigenen Bildschirm gemeldet hat: siehe [Streamen auf Chromecast](casting.md). |
| Ein SDR-Bildschirm, egal wo | Eine Umwandlung in SDR. |

HDR ohne Umwandlung auf einem SDR-Bildschirm ergibt ausgewaschene, graue
Farben. Deshalb sendet der Server es nie. Die Android-Apps prüfen die Anzeige
bei jedem Start einer Wiedergabe, nicht einmal bei der Installation: Schließen
Sie einen HDR-Fernseher an eine Box an, wirkt das beim nächsten Titel, beim
nächsten Spurwechsel oder bei der nächsten Änderung der Qualität.

## Wann das Original erhalten bleibt

Das HDR10- oder HLG-Video wird so gesendet, wie es ist, wenn drei Dinge
zutreffen: Der Player meldet, dass sein Bildschirm HDR anzeigt, er dekodiert
den Codec, und kein anderer Grund (Bitrate, Auflösung, Container) verlangt
eine Neukodierung.

## Wenn das Video trotzdem neu kodiert werden muss

Eine Grenze für die Bitrate oder die Auflösung oder ein Untertitel, der ins
Bild eingebrannt werden soll, kann eine HDR-Quelle zur Neukodierung zwingen.
Das HDR bleibt dann erhalten, wenn all dies zutrifft:

- Der Player meldet einen HDR-Bildschirm und dekodiert HEVC;
- der Server hat eine Grafikkarte, die bei seiner Messung nachgewiesen hat, dass
  sie HEVC 10 Bit mit HDR kodiert;
- diese Karte ist für diesen Stream frei.

Das Ergebnis ist HEVC 10 Bit, und die HDR-Kennzeichnung der Quelle wird
übernommen (Farbraum BT.2020, Transferfunktion PQ oder HLG). Die dynamischen
Metadaten von HDR10+ werden nicht übernommen: Ein HDR10+-Titel läuft auf
diesem Weg als HDR10.

Ist eine der Bedingungen nicht erfüllt, rechnet der Prozessor oder die Karte
das Bild vor dem Kodieren per Tone-Mapping auf SDR um. Stream-Info führt dann
**HDR nach SDR konvertiert (Tone-Mapping)** auf.

Manchmal hätte das Original gesendet werden können, aber der Server kann es
noch nicht als Kopie ausliefern (zum Beispiel, weil sein Seek-Index noch
aufgebaut wird). Dann wird das Video auf einer solchen Karte neu kodiert, mit
erhaltenem HDR, und Stream-Info zeigt **Kopie nicht verfügbar, HDR beim
Neukodieren erhalten**.

## Wenn der Bildschirm SDR ist

Ein Player, der keinen HDR-Bildschirm meldet, erhält ein SDR-Bild, das per
Tone-Mapping umgerechnet wurde. Das Tone-Mapping läuft auf der Grafikkarte,
wenn sie nachgewiesen hat, dass sie es kann (zum Beispiel VAAPI mit OpenCL), sonst
auf dem Prozessor: mit einem SIMD-optimierten Filter (AVX2), wenn der
ffmpeg-Build ihn mitbringt, sonst mit einer portablen Kette von Filtern.

!!! note "Hinweis"
    Auf dem Prozessor ist Tone-Mapping die teuerste Transkodierung überhaupt.
    Der Server misst für HDR eine eigene, meist niedrigere Obergrenze der
    Auflösung. Ein HDR-Titel kann daher auf derselben Maschine in einer
    niedrigeren Auflösung umgewandelt werden als ein SDR-Titel. Siehe
    [Transkodierung](transcoding.md#the-server-measures-itself).

## Dolby Vision

Die Meldung eines Players sagt, ob sein Bildschirm HDR anzeigt, nicht, ob er
Dolby Vision unterstützt. Evey schließt nie von HDR auf Dolby Vision: Dolby
Vision an einen Bildschirm zu senden, der nur HDR10 beherrscht, ergibt falsche
Farben. Der Stream wird nur kopiert, wenn er eine Basisschicht trägt, die ohne
jede Dekodierung von Dolby Vision richtig angezeigt wird:

| Dolby-Vision-Profil | HDR-Player | SDR-Player |
| --- | --- | --- |
| **8.1** (Basisschicht, die mit HDR10 kompatibel ist) | Wie HDR10 behandelt: Die Regeln oben gelten, das Original kann erhalten bleiben. | Per Tone-Mapping auf SDR umgerechnet. |
| **7** (UHD Blu-ray) | Die HDR10-Basisschicht wird als HEVC 10 Bit neu kodiert, das HDR bleibt erhalten. Stream-Info: **Dolby-Vision-Basisschicht neu kodiert (HDR bleibt)**. | Per Tone-Mapping auf SDR umgerechnet. |
| **5** (keine HDR10-Basisschicht) | Per Tone-Mapping auf SDR umgerechnet. | Per Tone-Mapping auf SDR umgerechnet. |

## Prüfen, was passiert ist

**Stream-Info** zeigt die Entscheidung für das Video und unter den Gründen
das Tone-Mapping oder das erhaltene HDR. Jeder Weg mit Tone-Mapping meldet es
dort, mit einer Ausnahme: Ist die Grafikkarte genau beim Start eines Streams
voll, wechselt der Stream auf den Prozessor, nachdem Stream-Info schon
berechnet wurde. **Verwaltung → Aktuelle Wiedergabe** zeigt, womit
tatsächlich kodiert wird.

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateVideo (hdr_tonemapping_needed, dovi_base_layer, doViBaseLayerIsHDR10)
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable) ; internal/app/transcoding/resolver_route.go - naturalRoute, passthroughWanted, hwTonemapWanted
hdr signalling carried: internal/adapters/driven/ffmpeg/hls_args_hw.go - hwHDRPassthroughArgs (no HDR10+ metadata)
dovi copy rule: internal/app/transcoding/copy_eligibility.go (dovi_profile, DoViBLCompat)
who declares an HDR screen: frontend/packages/client/src/native/displayHdr.ts - probeDisplayHdr ; frontend/packages/client/src/native/playSession.ts (supportsHdr) ; frontend/apps/web/src/cast-receiver/receiverLogic.ts (hdr=1) ; the web player sends no hdr parameter
tonemapx, software HDR ceiling: internal/adapters/driven/ffmpeg/tonemapx.go ; internal/pkg/hdr/hdr.go ; docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.* ; admin.nowPlaying
plain opening: rules below ; tone mapping cost: internal/app/transcoding/calibration_service.go (separate HDR ceiling)
-->
