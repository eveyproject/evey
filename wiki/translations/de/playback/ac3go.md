---
source_sha: bb59efb19b44f714d153c63f29645a30745cd729e75bb9e92bae2127c34c3296
reviewed: true
---
# ac3go in Evey

**Kurz gesagt:**

- Chrome, Firefox und Edge können Ton in Dolby Digital (AC-3) oder Dolby
  Digital Plus (E-AC-3) nicht abspielen. Statt ihn vom Server umwandeln zu
  lassen, dekodiert der Web-Player von Evey ihn in der Seite selbst, mit
  ac3go, einem Decoder, der als WebAssembly im Browser läuft.
- Der Ton kommt so bei Ihnen an, wie er in der Datei vorliegt, und der Server
  muss dafür nichts am Ton umwandeln.
- Sie müssen nichts einstellen. **Stream-Info** im Player zeigt den Ton als
  *clientseitig dekodiert*, und **Verwaltung → Aktuelle Wiedergabe** zeigt
  für diese Spur *ac3go im Player*.
- Kann der Decoder nicht laufen, wechselt der Player von selbst zu Ton, den
  der Server umgewandelt hat (AAC).

Der Rest dieser Seite erklärt, wann dieser Weg genommen wird, wie der Ton
synchron zum Bild bleibt und was zu prüfen ist, wenn ein Film ohne Ton
läuft. Welche Tonspur läuft und wann sie allgemein kopiert oder umgewandelt
wird, steht unter [Audio](audio.md).

ac3go wird als eigene Bibliothek entwickelt, in Go geschrieben und ohne
Abhängigkeiten: [github.com/gravity-zero/ac3go](https://github.com/gravity-zero/ac3go).

## Wann der Browser den Ton selbst dekodiert

Alle diese Bedingungen müssen erfüllt sein:

| Bedingung | Sonst |
| --- | --- |
| Der Browser kann AC-3 oder E-AC-3 nicht selbst abspielen und führt WebAssembly aus. Der Player testet den Browser auf beides, statt ihn am Namen zu erkennen. | Ein Browser, der Dolby Digital selbst dekodiert, etwa Safari, erhält die Spur als gewöhnliche Spur. |
| Die gespielte Tonspur ist AC-3 oder E-AC-3. Andere Spuren des Films zählen nicht. | Ein anderer Codec folgt den Regeln unter [Audio](audio.md). |
| Die Spur hat höchstens sechs Kanäle (5.1) und nicht mehr als **Maximale Audio-Kanäle** im Profil erlaubt. Dieser Weg gibt die Spur so weiter, wie sie in der Datei steht, und kann die Kanäle deshalb nicht heruntermischen. | Der Server wandelt die Spur in AAC um, das die Kanäle für ein Stereo-Profil heruntermischen kann. Eine E-AC-3-Spur in 7.1 wird immer umgewandelt. |
| Der Film läuft über den Stream des Servers (Remux oder Transkodierung), nicht als direkt gelesene Datei. | Die Spur folgt der Entscheidung für diese Wiedergabe. |

## Wie die Wiedergabe abläuft

Das Bild läuft stumm über den üblichen Player des Browsers. Der Ton kommt aus
einem eigenen Audio-Stream, den der Server in der Playlist nennt: die Spur so,
wie sie in der Datei steht, verpackt von [MKVGo](mkvgo.md), wenn es geht,
sonst von FFmpeg kopiert. Keiner der beiden dekodiert sie.

In der Seite lädt ein Hintergrund-Thread (ein *Worker*) ac3go und dekodiert
jedes Audio-Segment in unkomprimierten Ton, getrennt vom Code, der das Bild zeichnet.
Der Player spielt diesen Ton dann über die Audio-Ausgabe des Browsers ab, im
Takt des Bildes: Das Bild gibt die Uhr vor, und der Ton wird danach geplant.
Eine Pause hält beides zusammen an. Ein Sprung im Film oder eine Lücke nach
einem hängenden Stream richtet den Ton an der neuen Position neu aus.

Eine 5.1-Spur wird dem Browser in der Kanalreihenfolge übergeben, die er
erwartet, damit er sie für Stereo-Lautsprecher oder Kopfhörer heruntermischen
kann.

Am Anfang eines Films braucht der Decoder einen Moment zum Laden. Bietet der
Server die Spur auch in AAC umgewandelt an, spielt dieser Ton während dieses
Moments, und ac3go übernimmt, sobald er bereit ist. Der Browser lädt den
Decoder nur einmal herunter und prüft ihn bei jeder Nutzung erneut. So mischt
ein Server-Update nie eine alte und eine neue Fassung.

## Wenn ac3go nicht laufen kann

Lädt der Decoder nicht, scheitert er an einem Segment oder hat der Server ihm
keinen Audio-Stream genannt, verwendet der Player ihn für den Rest des Titels
nicht mehr und fordert beim Server den normalen Stream an, mit dem Ton in AAC
umgewandelt. Der Film behält seinen Ton; **Stream-Info** zeigt dann die
Umwandlung statt *clientseitig dekodiert*.

!!! warning "Achtung"
    Ein Reverse Proxy, der seine eigene Content-Security-Policy setzt, muss
    `'wasm-unsafe-eval'` in `script-src` erlauben. Sonst kann der Decoder
    nicht starten. Außerdem muss er die Decoder-Dateien unter `/wasm/` wie den
    Rest der Web-App ausliefern. Siehe
    [Reverse-Proxy-Konfigurationen](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Auf einem Cast-Empfänger

Der Chromecast-Empfänger von Evey verwendet ac3go nicht. Auf TV-Boxen mit
wenig Speicher hörten Empfänger, die den Ton so dekodierten, nach einigen
Minuten Wiedergabe auf. Der Fernseher bekommt Dolby Digital unverändert, wenn
sein Empfänger meldet, dass er es durchreicht, und sonst das AAC des Servers:
siehe [Streamen auf Chromecast](casting.md).

## Die Bibliothek und ihre Lizenz

ac3go steht wie Evey unter der GNU AGPLv3. Es wird beim Bau des Evey-Images
aus den Quellen erstellt (mit den Go-Werkzeugen zu WebAssembly kompiliert),
und seine Lizenz wird neben dem Decoder unter `/wasm/ac3go.LICENSE.md`
ausgeliefert.
