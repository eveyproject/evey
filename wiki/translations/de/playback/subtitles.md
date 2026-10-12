---
source_sha: 0df87f23e8ea13dc773d22a1984be991c45751df70cc5c7cfc5cad5238cb7d7b
reviewed: true
---
# Untertitel

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
- **Untertitel von DVDs und aus dem Fernsehen** sind Bilder, kein Text. Der
  Server brennt sie direkt ins Video ein, was für ihn deutlich mehr Rechenaufwand bedeutet. Die
  Profileinstellung **Untertitel einbrennen** (**Einstellungen → Wiedergabe**)
  kann das ablehnen, aber dann können diese Untertitel nicht angezeigt
  werden. Siehe [Untertitel einbrennen](#subtitle-burn-in).

Der Rest dieser Seite erklärt jedes Untertitelformat und was es für den
Server ändert.

## Was jedes Format mit dem Video macht

| Format | Beispiele | Wie es angezeigt wird | Wirkung auf das Video |
| --- | --- | --- | --- |
| Text | SRT, ASS/SSA, WebVTT, Untertitel in der Datei | Neben dem Video geschickt und vom Player dargestellt, mit Ihrem **Untertitel-Stil** | Keine |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | Einmal vom Server in Bilder umgewandelt, zwischengespeichert und vom Player unverändert dargestellt | Keine |
| Andere Bildformate | VobSub (DVD), DVB, XSUB | Vom Server ins Bild eingebrannt | Das Video wird neu kodiert |

Jeder Player (Web, Android, Android TV) stellt Text- und PGS-Untertitel
selbst dar. Einen davon zu wählen, ändert also nie, wie das Video geliefert wird.
Nur die letzte Zeile tut das: Untertitel ins Bild einzubrennen heißt, das
Video zu dekodieren und neu zu kodieren. Ein Titel, der direkt abgespielt
wurde, wird so zu einer Transkodierung. Die Stream-Info zeigt dann
**Untertitel ins Bild eingebrannt**.

Ein Bild-Untertitel wird nur eingebrannt, wenn Sie ihn wirklich auswählen.
Eine Datei, die nur einen enthält oder ihn als Standard markiert, bleibt bei
der Direktwiedergabe.

## Untertitel einbrennen

Die Profileinstellung **Untertitel einbrennen** (**Einstellungen →
Wiedergabe**) gilt für die Formate der letzten Zeile:

| Einstellung | Eine ausgewählte VobSub-, DVB- oder XSUB-Spur | Text und PGS |
| --- | --- | --- |
| **Auto** | Eingebrannt, das Video wird neu kodiert. | Vom Player dargestellt. |
| **Immer einbrennen** | Wie Auto. | Vom Player dargestellt: Text wird nie eingebrannt, egal bei welcher Einstellung. |
| **Nie einbrennen** | Kann nicht angezeigt werden: Das Video wird für einen Untertitel nie neu kodiert. | Vom Player dargestellt. |

## Wenn ein Untertitel zum ersten Mal gezeigt wird

Ein Untertitel in der Datei (Text oder PGS) wird vom Server extrahiert, wenn
ihn jemand zum ersten Mal wählt, und dann aufbewahrt: Bei jeder späteren
Wiedergabe, egal durch wen, erscheint er sofort. Deckt der eigene Index der Datei die
Spur ab, liest der Server nur die Blöcke des Untertitels, und das dauert
Sekunden. Ohne diesen Index liest er die ganze Datei: einige Minuten bei
einem großen Film (etwa fünf bei einem UHD-Remux von 85 GB auf einer lokalen
Festplatte, länger, wenn derselbe Server gleichzeitig streamt).

Währenddessen zeigt der Player **Untertitel werden vorbereitet…** dort, wo
die Untertitel erscheinen, prüft von selbst erneut und zeigt sie an, sobald
sie bereit sind; das Video läuft weiter. Eine andere Spur zu wählen, stoppt
die Arbeit nicht: Der Server schließt sie für das nächste Mal ab.

Eine Spur, die der Server nicht extrahieren kann, bleibt leer. Der Fehler
wird gespeichert, sodass eine erneute Auswahl nicht noch einmal die ganze Datei
liest; ein Update von MKVGo, der Bibliothek, die die Spur extrahiert,
versucht es erneut. Siehe [MKVGo in Evey](mkvgo.md#what-mkvgo-does), was
wann gelesen wird.

## Auswählen und anpassen

- Die Player wählen den Untertitel der **Standard-Untertitelsprache** des
  Profils, danach die Spur, die die Datei als Standard markiert (zum Beispiel
  eine erzwungene Spur). Eine von Hand gewählte Spur wird nie überschrieben,
  und bei einem neuen Titel beginnt die Wahl von vorn.
- **Untertitel-Synchronisation** im Player verschiebt die Untertitel für den
  laufenden Titel (positive Werte zeigen sie später). Das wirkt sofort.
- **Untertitel-Stil** stellt Größe, Textfarbe, Hintergrund, Schrift, Schatten
  und Position von Text-Untertiteln ein. PGS-Bilder werden unverändert
  dargestellt.
- **Untertitel hochladen…** fügt dem Titel eine Datei hinzu.
- Mit einem OpenSubtitles-Schlüssel (siehe [Bibliotheken und Metadaten](../administration/libraries.md#metadata-keys))
  kann die Server-API nach einem Untertitel suchen, den die Datei nicht
  enthält, und ihn herunterladen (`GET /api/v1/media/{id}/subtitles/search`,
  dann `POST /api/v1/media/{id}/subtitles/download`). Die Datei wird in einem
  Ordner `.subtitles` neben dem Film gespeichert. Die Apps bieten diese Suche
  nicht an.
