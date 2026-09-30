---
source_sha: e59bbf6aa9d7e1a2cb3513ef0337a093ae293e253430eaa6e6d5dc087423cb60
reviewed: false
---
# Bibliotheken und Metadaten

Eine Bibliothek ist ein Typ und eine Reihe von Ordnern. Evey durchsucht die
Ordner, erkennt jede Datei an ihrem Namen und holt ihre Metadaten: Titel,
Zusammenfassungen, Poster, Besetzung, Bewertungen. Bibliotheken werden unter
**Admin → Bibliotheken** verwaltet.

<figure class="evey-shot" markdown="span">
  ![Admin → Bibliotheken auf dem Demo-Server](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Admin → Bibliotheken</strong>: eine Zeile pro Bibliothek, mit ihren Ordnern, der Anzahl der Einträge und ihrem letzten Scan (Oberfläche auf Englisch).</figcaption>
</figure>

## Typen

| Typ | Erkannt als | Metadaten von |
| --- | --- | --- |
| **Filme** | Ein Film pro Datei (oder pro Ordner) | TMDB, ergänzt durch OMDb, wenn ein Schlüssel gesetzt ist |
| **Serien** | Serien, Staffeln, Folgen | TMDB |
| **Musik** | Künstler, Alben, Titel | Die Tags der Dateien, MusicBrainz; Cover von Cover Art Archive, dann iTunes |
| **Fotos** | Alben, Bilder mit ihren Aufnahmedaten | Die Dateien |

Halten Sie Filme und Serien in getrennten Bibliotheken: Der Typ sagt dem
Scanner, wonach er suchen soll.

## Ordner sind Pfade im Container

Die Ordnerauswahl listet, was der Container sieht. Mit der Compose-Datei der
Installation ist Ihr Medienordner `/media`, ein Ordner `/srv/media/films` auf
dem Host ist also `/media/films` für Evey. Ein zweiter eingebundener Ordner
(`/media2`) wird genauso hinzugefügt.

## Dateien benennen

Der Scanner erkennt Namen wie diese:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

Bei einem Film zählt vor allem das Jahr. Bei einer Folge `S01E05` oder `1x05`.
Release-Tags (Auflösung, Quelle, Codec, Audio, Sprach-Tags wie `VOSTFR` oder
`MULTI`, Fassungen wie „Director's Cut") werden vor der Suche entfernt. Tragen
Ihre Dateien ein Tag, das der Parser nicht kennt, fügen Sie es unter
**Admin → Serverkonfiguration → Parser-Tokens** hinzu; es gilt ab dem nächsten
Scan.

## Wie neue Dateien erkannt werden

Jede Bibliothek hat drei Optionen:

| Option | Wirkung |
| --- | --- |
| **Echtzeit-Überwachung** | Neue, umbenannte und gelöschte Dateien werden erkannt, sobald sie erscheinen. Eine regelmäßige Prüfung (standardmäßig alle 2 Minuten) fängt ab, was das Dateisystem nicht gemeldet hat, wie es bei Netzwerkfreigaben vorkommt. |
| **Beim Start scannen** | Ein Scan bei jedem Serverstart. |
| **Zeitplan** | Ein vollständiger Scan alle 6 oder 12 Stunden, täglich, wöchentlich oder nach einem Cron-Ausdruck. |

Der Server prüft außerdem, dass seine Ordnerüberwachung noch Ereignisse
empfängt, indem er für einige Sekunden eine Prüfdatei in den
Bibliotheksordner schreibt. Eine Bibliothek, bei der keine Ereignisse mehr
ankommen, wird häufiger durchlaufen, bis sich die Überwachung erholt.

## Wenn ein Titel falsch ist oder fehlt

- **Nur Waisen abgleichen** (pro Bibliothek) versucht die Dateien erneut, die
  nie erkannt wurden. Das geht schnell.
- **Alle erneut mit TMDB abgleichen** wiederholt die Suche für jeden Titel der
  Bibliothek. Das ist langsam und korrigiert alte Fehler nach einer großen
  Umbenennung.
- **Metadaten bearbeiten** ändert bei einem Titel die Angaben von Hand oder
  fügt die richtige TMDB- oder IMDb-ID ein. Ein von Ihnen bearbeiteter Wert
  bleibt erhalten: Spätere Aktualisierungen überschreiben ihn nicht.
  **Metadaten neu zuordnen** beim Titel verwirft Ihre Änderungen und sucht
  erneut anhand des Dateinamens.
- **Reihen reparieren** fügt Filme, die zu keiner Sammlung gehören, wieder ihrer
  Filmreihe hinzu, wenn die anderen Filme der Reihe bereits in der Bibliothek
  sind.

## Metadaten-Schlüssel

Evey bringt einen TMDB-Schlüssel mit, sodass Metadaten ohne Konfiguration
funktionieren. Andere Quellen sind optional; ihre Schlüssel gehören unter
**Admin → Serverkonfiguration → API-Schlüssel**, und ein dort gesetzter
Schlüssel hat Vorrang vor demselben Schlüssel aus der Umgebung.

| Schlüssel | Bringt |
| --- | --- |
| TMDB (Ihr eigener) | Ihr eigenes Anfragekontingent statt des geteilten. |
| OMDb | Eine zweite Quelle für Filme, über die TMDB wenig weiß. |
| OpenSubtitles | Suche und Download von Untertiteln, die eine Datei nicht mitbringt, über die Server-API (die Apps bieten das noch nicht an). Siehe [Untertitel](../playback/subtitles.md). |
| TheIntroDB | Zeiten für Intro, Rückblick und Abspann aus einer Community-Datenbank: 500 Abfragen pro Tag mit Schlüssel statt 100 ohne. |

## Intro, Rückblick und Abspann

Evey markiert Intro, Rückblick und Abspann von Folgen und Filmen, damit die
Player anbieten können, sie zu überspringen. Die Zeiten kommen aus
Community-Datenbanken (TheIntroDB, dann IntroDB), wenn diese den Titel kennen,
und sonst aus der eigenen Analyse der Dateien durch den Server. Ein
Administrator kann die Segmente eines Titels von Hand anpassen oder die
Erkennung erneut ausführen.

Die Player zeigen standardmäßig eine Schaltfläche **Überspringen**.
**Intros automatisch überspringen** (**Admin → Serverkonfiguration →
Servereinstellungen**, standardmäßig aus) lässt jeden Player sie von selbst
überspringen.
