---
source_sha: f3462cd1e79cf16db4a38f3ecb82da3588ca9b03301e7d32db33118bb80ea428
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
| **Musik** | Künstler, Alben, Titel | Die Tags der Dateien; Cover aus den Dateien, dem Albumordner, Cover Art Archive (MusicBrainz), dann iTunes |
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

## Musikbibliotheken

Der Scanner liest MP3-, FLAC-, M4A-, OGG-, Opus-, WAV- und AAC-Dateien und
entnimmt jeden Titel seinen Tags: Titel (der Dateiname, wenn er fehlt),
Künstler, Albumkünstler, Album, Jahr, Titel- und CD-Nummer, Genre. Künstler
werden nach Albumkünstler gruppiert, oder nach Künstler, wenn dieses Tag leer
ist.

**Ein Album ist ein Ordner.** Titel werden nach dem Ordner gruppiert, in dem sie
liegen, nicht nach ihrem Album-Tag: Zwei Ordner mit demselben Album-Tag sind
zwei Alben, und ein Album, das auf Unterordner `CD1` und `CD2` verteilt ist,
erscheint als zwei. Behalten Sie jedes Album in einem Ordner; seine Titel werden
nach CD-Nummer und dann nach Titelnummer sortiert, und das Album trägt den
Namen, den die meisten seiner Titel tragen.

Das Cover ist das in die Dateien eingebettete Bild, sonst ein Bild im
Albumordner (`cover.jpg`, `folder.jpg` oder `front.jpg`), sonst Cover Art
Archive (über MusicBrainz gefunden), sonst iTunes.

## Fotobibliotheken

Der Scanner liest JPEG-, PNG-, HEIC-, WebP-, GIF-, TIFF- und BMP-Dateien. Das
Album eines Fotos ist sein Ordner, relativ zum Bibliotheksordner; ein Foto auf
der obersten Ebene hat keines, und wer eine Datei in einen anderen Ordner
verschiebt, verschiebt sie beim nächsten Scan in dieses Album. Aufnahmedatum
und Kameramodell stammen aus den EXIF-Daten von JPEG-Dateien; die anderen
Formate werden ohne sie aufgeführt. Vorschaubilder werden auf dem Server
verkleinert und im Transkodierungs-Cache gehalten.

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

## Sammlungen

**Sammlungen** fasst Titel unter einem Namen zusammen, für alle Konten. Es gibt
drei Arten:

- **TMDB-Sammlungen**: die Filmreihen, die sich von selbst füllen, wenn TMDB
  einen Film einer zuordnet. **Reihen reparieren** (oben) holt Filme nach, die
  ihre verpasst haben.
- **Neue Sammlung**: eine Liste, die ein Administrator von Hand füllt, mit
  **Zur Sammlung hinzufügen** auf einem Titel oder einer Auswahl, und ordnet.
- **Neue Smart-Sammlung**: Regeln zu **Genre**, **Jahr**, **Typ**,
  **Bewertung ≥** und **Bibliothek**, die ein Titel alle erfüllen muss. Ihr
  Inhalt wird bei jedem Öffnen ermittelt, sodass neue Titel ohne Zutun
  erscheinen.

Nur Administratoren legen Sammlungen an und bearbeiten sie. Jeder kann sie
durchsuchen, und **Sammlung herunterladen** im Web holt die Originaldateien als
ein ZIP (bis zu 1000 Titel), ohne das, was der Bibliothekszugriff oder die
Altersgrenze des Kontos ausblendet.

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

Die Player zeigen standardmäßig die Schaltflächen **Intro überspringen**,
**Rückblick überspringen** und **Abspann überspringen**.
**Intros automatisch überspringen** (**Admin → Serverkonfiguration →
Servereinstellungen**, standardmäßig aus) lässt jeden Player sie von selbst
überspringen.
