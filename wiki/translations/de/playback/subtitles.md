---
source_sha: 960cbfbbd3741cf6a15885a1e19f04e9e96e53efbe2f9861376cc3a92778dfef
reviewed: true
---
# Untertitel { #subtitles }

**Um Untertitel einzuschalten**, öffnen Sie im Player das Einstellungsmenü und
wählen **Untertitel**. Damit sie jedes Mal in Ihrer Sprache erscheinen, stellen
Sie **Einstellungen → Wiedergabe → Standard-Untertitelsprache** ein.

Die meisten Untertitel erscheinen sofort. Zwei Fälle sind anders:

- **Wenn jemand zum ersten Mal einen Untertitel wählt, der in der Datei
  steckt**, muss Evey ihn aus der Datei extrahieren. Der Player zeigt **Untertitel
  werden vorbereitet…**, das Video läuft weiter, und die Untertitel erscheinen
  von selbst. Bei einem sehr großen Film kann das einige Minuten dauern,
  einmalig: Danach erscheinen sie für alle sofort. Siehe
  [Wenn ein Untertitel zum ersten Mal gezeigt wird](#the-first-time-a-subtitle-is-shown).
- **Untertitel von Blu-rays, DVDs und aus dem Fernsehen** sind Bilder, kein
  Text. Der Server wandelt sie einmal in Bilder um, und der Player stellt sie
  dar wie Text. Nur das seltene Format XSUB brennt der Server noch ins Video
  ein, was für ihn deutlich mehr Rechenaufwand bedeutet. Siehe
  [Untertitel einbrennen](#subtitle-burn-in).

Der Rest dieser Seite erklärt jedes Untertitelformat und was es für den
Server ändert.

## Was jedes Format mit dem Video macht { #what-each-format-does-to-the-video }

| Format | Beispiele | Wie es angezeigt wird | Wirkung auf das Video |
| --- | --- | --- | --- |
| Text | SRT, ASS/SSA, WebVTT, Untertitel in der Datei | Neben dem Video geschickt und vom Player dargestellt, mit Ihrem **Untertitel-Stil** | Keine |
| Bildformate | PGS (Blu-ray), VobSub (DVD), DVB (Fernsehen), in der Datei oder als `.sup`- oder `.idx`/`.sub`-Datei daneben | Einmal vom Server in Bilder umgewandelt, zwischengespeichert und vom Player unverändert dargestellt | Keine |
| XSUB | XSUB (manche DivX-Dateien) | Vom Server ins Bild eingebrannt | Das Video wird neu kodiert |

Jeder Player (Web, Android, Android TV) stellt Text- und Bild-Untertitel
selbst dar. Einen davon zu wählen, ändert also nie, wie das Video geliefert wird.
Nur die letzte Zeile tut das: Untertitel ins Bild einzubrennen heißt, das
Video zu dekodieren und neu zu kodieren. Ein Titel, der direkt abgespielt
wurde, wird so zu einer Transkodierung. Die Stream-Info zeigt dann
**Untertitel ins Bild eingebrannt**.

Ein XSUB-Untertitel wird nur eingebrannt, wenn Sie ihn wirklich auswählen.
Eine Datei, die nur einen enthält oder ihn als Standard markiert, bleibt bei
der Direktwiedergabe.

## Untertitel einbrennen { #subtitle-burn-in }

Die Profileinstellung **Untertitel einbrennen** (**Einstellungen →
Wiedergabe**) entscheidet, welche Bild-Untertitel der Server ins Video
einbrennt:

| Einstellung | Eine ausgewählte XSUB-Spur | Eine ausgewählte PGS-, VobSub- oder DVB-Spur | Text |
| --- | --- | --- | --- |
| **Auto** | Eingebrannt, das Video wird neu kodiert. | Vom Player dargestellt. | Vom Player dargestellt. |
| **Immer einbrennen** | Eingebrannt, das Video wird neu kodiert. | Eingebrannt, das Video wird neu kodiert. | Vom Player dargestellt: Text wird nie eingebrannt. |
| **Nie einbrennen** | Kann nicht angezeigt werden: Das Video wird für einen Untertitel nie neu kodiert. | Vom Player dargestellt. | Vom Player dargestellt. |

## Wenn ein Untertitel zum ersten Mal gezeigt wird { #the-first-time-a-subtitle-is-shown }

Ein Untertitel in der Datei (Text oder Bild) wird vom Server extrahiert, wenn
ihn jemand zum ersten Mal wählt, und dann aufbewahrt: Bei jeder späteren
Wiedergabe, egal durch wen, erscheint er sofort. Deckt der eigene Index der Datei eine
Text- oder PGS-Spur ab, liest der Server nur die Blöcke des Untertitels, und das dauert
Sekunden. Ohne diesen Index liest er die ganze Datei: einige Minuten bei
einem großen Film (einige Minuten bei einer sehr großen Datei, länger, wenn derselbe Server gleichzeitig streamt).

Währenddessen zeigt der Player **Untertitel werden vorbereitet…** dort, wo
die Untertitel erscheinen, prüft von selbst erneut und zeigt sie an, sobald
sie bereit sind; das Video läuft weiter. Eine andere Spur zu wählen, stoppt
die Arbeit nicht: Der Server schließt sie für das nächste Mal ab.

Eine Spur, die der Server nicht extrahieren kann, bleibt leer. Der Fehler
wird gespeichert, sodass eine erneute Auswahl nicht noch einmal die ganze Datei
liest; ein Update von MKVGo, der Bibliothek, die die Spur extrahiert,
versucht es erneut. Siehe [MKVGo in Evey](mkvgo.md#what-mkvgo-does), was
wann gelesen wird.

## Auswählen und anpassen { #choosing-and-adjusting }

- Die Player wählen den Untertitel der **Standard-Untertitelsprache** des
  Profils, danach die Spur, die die Datei als Standard markiert (zum Beispiel
  eine erzwungene Spur). Eine von Hand gewählte Spur wird nie überschrieben,
  und bei einem neuen Titel beginnt die Wahl von vorn.
- **Untertitel-Synchronisation** im Player verschiebt die Untertitel für den
  laufenden Titel (positive Werte zeigen sie später). Das wirkt sofort.
- **Untertitel-Stil** stellt Größe, Textfarbe, Hintergrund, Schrift, Schatten
  und Position von Text-Untertiteln ein. Bild-Untertitel werden unverändert
  dargestellt.
- **Untertitel hochladen…** fügt dem Titel eine Datei hinzu.
- Mit einem OpenSubtitles-Schlüssel (siehe [Bibliotheken und Metadaten](../administration/libraries.md#metadata-keys))
  kann die Server-API nach einem Untertitel suchen, den die Datei nicht
  enthält, und ihn herunterladen (`GET /api/v1/media/{id}/subtitles/search`,
  dann `POST /api/v1/media/{id}/subtitles/download`). Die Datei wird in einem
  Ordner `.subtitles` neben dem Film gespeichert. Die Apps bieten diese Suche
  nicht an.
