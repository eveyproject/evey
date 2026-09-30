---
source_sha: 671aed4ea8f4b0ef01dac83a6cadd4f8e569eb5206951057955cbf29a9e7b727
reviewed: false
---
# Audio

Die Audiospur wird getrennt vom Video beurteilt. Bei den meisten
Transkodierungen ist sie das Einzige, was neu kodiert wird: Das Video wird
kopiert und der Ton in ein Format umgewandelt, das der Player liest.

## Welche Spur läuft

Eine von Hand gewählte Spur, auf der Titelseite oder im Player, gewinnt immer.
Sonst folgen die Player der **Standard-Audiosprache** des Profils, dann der
Spur, die die Datei als Standard markiert, dann der ersten. Eine als Kommentar
markierte Spur wird zugunsten einer regulären übersprungen, wenn die Datei
beide hat.

Beurteilt wird nur die abgespielte Spur: Eine DTS-Spur, die Sie nicht hören,
löst nie eine Transkodierung aus.

## Kopiert oder neu kodiert

Die Spur wird kopiert, wenn der Player ihren Codec dekodiert und genug Kanäle
dafür hat, im Rahmen der **Maximalen Audio-Kanäle** des Profils (Stereo, 5.1
oder 7.1). Sonst wird sie nach AAC neu kodiert:

- mit so vielen Kanälen, wie Quelle, Player und Profil erlauben, bis 5.1 (eine
  7.1-Quelle wird auf 5.1 heruntergemischt, das jeder Player dekodiert;
  Stereo-Quellen bleiben Stereo);
- unabhängig von der Abtastrate, die allein nie ein Grund für eine
  Neukodierung ist.

Eine kopierte Spur in einem transkodierten oder neu verpackten Stream muss
außerdem ein Format sein, das der Player in den Segmenten des Streams
akzeptiert. AAC, AC-3 und E-AC-3 sind es immer; andere Codecs nur, wenn der
Player es angibt. Ist das nicht der Fall, wird die Spur neu kodiert, und
Stream-Info sagt es: Ein Player kann MP3 in einer Datei dekodieren und es in
einem Stream trotzdem ablehnen.

## Dolby Digital im Browser

Die meisten Browser können AC-3 oder E-AC-3 nicht dekodieren. Statt die Spur auf
dem Server neu zu kodieren, dekodiert der Web-Player sie selbst, mit einem
Decoder, der in der Seite läuft: Die Spur wird unverändert gesendet, und
Stream-Info zeigt sie als *clientseitig dekodiert*. Browser, die Dolby Digital
nativ dekodieren, wie Safari, erhalten sie als gewöhnliche Spur; die Wahl
erfolgt durch einen Test des Browsers, nicht durch sein Erkennen.

Ein Reverse Proxy, der seine eigene Content-Security-Policy setzt, muss
`'wasm-unsafe-eval'` in `script-src` erlauben, sonst kann der Decoder nicht
starten und der Film läuft ohne Ton. Siehe
[Reverse-Proxy-Konfigurationen](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Ton nicht synchron

**Audio-Synchronisation** im Player verschiebt den Ton für den aktuellen Titel.
Eine kopierte Spur lässt sich nicht verschieben, deshalb kodiert der Server sie
neu; Stream-Info zeigt **Audio-Resynchronisation angefordert**.

Liegt der Versatz in der Datei selbst (eine Audiospur, die nach dem Bild
beginnt, oder Spuren, die untereinander nicht übereinstimmen), meldet ihn
**Admin → Medienzustand**, schreibt ihn aber nie von selbst um: Ein spät
beginnender Versatz ist meist eine gewollte Ausrichtung einer Synchronfassung.
**Synchronität prüfen** in der Zeile der Datei lässt Sie ihn mit dem Auge
beurteilen und eine Verschiebung anwenden, die ihr Verlauf rückgängig machen
kann. Siehe [Medienzustand](../runbooks/media-health.md).
