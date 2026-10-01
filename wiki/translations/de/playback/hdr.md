---
source_sha: 5c15d0f69d445cc33f1a7038b71f5435492c5280093f9b5d8ce87e76a2707d53
reviewed: false
---
# HDR und Dolby Vision

Ein HDR-Bild kann auf drei Arten auf den Bildschirm kommen: das Original,
unverändert; eine Neukodierung, die das HDR behält; oder eine Neukodierung mit
Tone Mapping auf SDR. Welche greift, hängt vom Bildschirm des Players ab, vom
Format des HDR und davon, was die Hardware des Servers nachweislich kann.

## Wann das Original erhalten bleibt

Das HDR10- oder HLG-Video wird so gesendet, wie es ist, wenn der Player meldet,
dass sein Bildschirm HDR darstellt und er den Codec dekodiert, und kein anderer
Grund (Bitrate, Auflösung, Container) eine Neukodierung verlangt. Die
allgemeinen Regeln stehen unter [Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md).

## Wenn das Video ohnehin neu kodiert werden muss

Eine Bitrate-Grenze, eine Auflösungsgrenze oder ein einzubrennender Untertitel
können die Neukodierung einer HDR-Quelle erzwingen. Das HDR bleibt dann
erhalten, wenn all dies zutrifft:

- der Player meldet einen HDR-Bildschirm und dekodiert HEVC;
- der Server hat eine Grafikkarte, die bei ihrer Messung bewiesen hat, dass sie
  HEVC 10 Bit mit HDR kodiert;
- diese Karte ist für diesen Stream verfügbar.

Das Ergebnis ist HEVC 10 Bit mit übernommenen statischen HDR10-Informationen
(Farbraum, Transferfunktion, Mastering-Display). Dynamische HDR10+-Metadaten
werden nicht übernommen: Ein HDR10+-Titel läuft auf diesem Weg als HDR10.

Ist eine der Bedingungen nicht erfüllt, rechnet der Prozessor oder die Karte das
Bild vor dem Kodieren per Tone Mapping auf SDR um. Stream-Info führt dann
**HDR nach SDR konvertiert (Tone-Mapping)** auf.

Hätte das Original gesendet werden können, kann der Server es aber noch nicht
als Kopie ausliefern (zum Beispiel, weil sein Seek-Index noch aufgebaut
wird), wird das Video auf einer solchen Karte mit erhaltenem HDR neu kodiert,
und Stream-Info zeigt **Kopie nicht verfügbar, HDR beim Neukodieren erhalten**.

## Wenn der Bildschirm SDR ist

Ein Player, der keinen HDR-Bildschirm meldet, erhält ein per Tone Mapping
umgerechnetes SDR-Bild. HDR ohne Umwandlung auf einem SDR-Bildschirm
anzuzeigen, ergibt ausgewaschene, graue Farben, deshalb sendet der Server es
nie.

Das Tone Mapping läuft auf der Grafikkarte, wenn sie es nachweislich kann
(zum Beispiel VAAPI mit OpenCL), sonst auf dem Prozessor mit einem
SIMD-optimierten Filter (AVX2 auf x86-64, NEON auf ARM). Auf dem Prozessor ist
es die teuerste Transkodierung überhaupt: Der Server misst für HDR eine eigene,
meist niedrigere Auflösungsobergrenze, sodass ein HDR-Titel auf derselben
Maschine in einer niedrigeren Auflösung umgewandelt werden kann als ein
SDR-Titel.

## Dolby Vision

Die Meldung eines Players sagt, ob sein Bildschirm HDR darstellt, nicht, ob er
Dolby Vision unterstützt, und Evey leitet die Unterstützung von Dolby Vision nie
aus der HDR-Unterstützung ab: Dolby Vision an einen Bildschirm zu senden, der
nur HDR10 beherrscht, ergibt falsche Farben. Der Stream wird nur kopiert, wenn
er eine Basisschicht trägt, die ohne jede Dolby-Vision-Dekodierung korrekt
angezeigt wird:

| Dolby-Vision-Profil | HDR-Player | SDR-Player |
| --- | --- | --- |
| **8.1** (HDR10-kompatible Basisschicht) | Wie HDR10 behandelt: Die Regeln oben gelten, das Original kann erhalten bleiben. | Per Tone Mapping auf SDR umgerechnet. |
| **7** (UHD Blu-ray) | Die HDR10-Basisschicht wird als HEVC 10 Bit mit erhaltenem HDR neu kodiert. Stream-Info: **Dolby-Vision-Basisschicht neu codiert (HDR bleibt)**. | Per Tone Mapping auf SDR umgerechnet. |
| **5** (keine HDR10-Basisschicht) | Per Tone Mapping auf SDR umgerechnet. | Per Tone Mapping auf SDR umgerechnet. |

## Prüfen, was passiert ist

**Stream-Info** zeigt die Videoentscheidung und unter den Gründen das Tone
Mapping oder das erhaltene HDR. Jeder Weg mit Tone Mapping meldet es dort, mit
einer Ausnahme: Ist die Grafikkarte im Moment des Stream-Starts voll, wechselt
der Stream auf den Prozessor, nachdem Stream-Info berechnet wurde. **Admin →
Aktuelle Wiedergabe** zeigt das tatsächlich genutzte Backend.
