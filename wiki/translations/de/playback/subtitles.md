---
source_sha: 3fced19f436765baa59845968676caa83737f9a681cd41288d6e854fcde880ae
reviewed: false
---
# Untertitel

Was mit einem Untertitel geschieht, hängt von seinem Format ab: Text oder
Bilder.

| Format | Beispiele | Wie er angezeigt wird | Wirkung auf das Video |
| --- | --- | --- | --- |
| Text | SRT, ASS/SSA, WebVTT, Untertitel in der Datei | Neben dem Video gesendet und vom Player gezeichnet | Keine |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | Vom Server einmal in Bilder dekodiert, zwischengespeichert und vom Player gezeichnet | Keine |
| Andere Bildformate | VobSub (DVD), DVB, XSUB | Vom Server ins Bild eingebrannt | Das Video wird neu kodiert |

Jeder Player (Web, Android, Android TV) zeichnet Text- und PGS-Untertitel
selbst, sodass die Wahl eines Untertitels nie ändert, wie das Video
ausgeliefert wird. Nur die letzte Zeile tut das: Untertitel ins Bild
einzubrennen verlangt, das Video zu dekodieren und neu zu kodieren, sodass ein
Titel, der direkt lief, zu einer Transkodierung wird. Stream-Info führt dann
**Untertitel ins Bild eingebrannt** auf.

Ein Bilduntertitel wird nur eingebrannt, wenn Sie ihn tatsächlich wählen. Eine
Datei, die lediglich einen mitbringt oder ihn als Standard markiert, behält ihre
Direktwiedergabe.

## Untertitel einbrennen

Die Profileinstellung **Untertitel einbrennen** (**Einstellungen →
Wiedergabe**) regelt die Formate der letzten Zeile:

- **Auto** und **Immer einbrennen**: Eine gewählte VobSub-, DVB- oder
  XSUB-Spur wird eingebrannt. Text wird nie eingebrannt, egal bei welcher
  Einstellung.
- **Nie einbrennen**: Das Video wird für einen Untertitel nie neu kodiert. Eine
  VobSub-, DVB- oder XSUB-Spur kann dann nicht angezeigt werden.

## Wählen und anpassen

- Die Player wählen den Untertitel der **Standard-Untertitelsprache** des
  Profils, dann die Spur, die die Datei als Standard markiert (zum Beispiel eine
  erzwungene Spur).
- **Untertitel-Synchronisation** im Player verschiebt die Untertitel für den
  aktuellen Titel (positive Werte zeigen sie später). Das gilt sofort.
- **Untertitel-Stil** legt Größe, Farbe, Hintergrund, Schrift und Position von
  Textuntertiteln fest.
- **Untertitel hochladen…** fügt dem Titel eine Datei hinzu.
- Mit einem OpenSubtitles-Schlüssel (siehe
  [Bibliotheken und Metadaten](../administration/libraries.md#metadata-keys))
  kann die Server-API einen Untertitel suchen und herunterladen, den die Datei
  nicht mitbringt (`GET /api/v1/media/{id}/subtitles/search`, dann
  `POST /api/v1/media/{id}/subtitles/download`). Die Datei wird in einem Ordner
  `.subtitles` neben dem Film gespeichert. Die Apps bieten diese Suche noch
  nicht an.
