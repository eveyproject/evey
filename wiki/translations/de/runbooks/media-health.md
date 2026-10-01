---
source_sha: 974b056c376498e40c4211ccbde7fd2d3f94c52f343c81353f4cc6f083b62a40
reviewed: false
---
# Medienzustand

**Admin → Medienzustand** liest jede MKV/WebM-Datei Ihrer Bibliotheken und gibt
jeder eine einzige Antwort. Ziel ist eine Seite, die sich leert: Was Evey
beheben kann, behebt es; der Rest wird einmal erwähnt und kann als gesehen
markiert werden.

<figure class="evey-shot" markdown="span">
  ![Admin → Medienzustand nach einem Scan der Demo-Bibliotheken](../images/admin-media-health.webp){ loading=lazy }
  <figcaption>Ein Scan, bei dem nichts mehr zu tun ist: Jede Datei ist gesund (Oberfläche auf Englisch).</figcaption>
</figure>

## Drei Urteile

| Urteil | Was es bedeutet | Was Evey tut |
| --- | --- | --- |
| **Gesund** | Index, Container, Größen und Audio-Timing stimmen. | Nichts. |
| **Hier reparierbar** | Ein Fehler, den Evey auf diesem Server repariert. | **Alle reparieren** erledigt sie im Hintergrund. |
| **Information** | Etwas an der Datei Gemessenes, das Evey nicht von selbst umschreibt. | Nichts. Markieren Sie es als gesehen. |

Keines davon schiebt Ihnen eine Aufgabe zu. Eine Messung, auf
die Evey nicht reagieren kann, ist ein Hinweis, und eine Messung kann falsch
sein: Wenn die Datei gut läuft, ist sie in Ordnung.

### Hier reparierbar

- **Ohne Seek-Index**, oder mit einem, der nicht nutzbar ist. Das Symptom ist
  ein langsamer Sprung oder ein langsames Fortsetzen. Der Index wird in einer
  temporären Kopie neu aufgebaut, die Kopie wird vollständig geprüft (Spuren,
  Dauer, Index) und ersetzt dann die Datei.
- **Falscher Container**: Die `.mkv` ist in Wahrheit eine MP4- oder MOV-Datei.
  Sie wird in `.mp4` umbenannt und die Bibliothek folgt; nichts wird
  umgeschrieben.
- **Beschädigter Abschnitt**: ein beschädigter Bereich mitten in der Datei. Die
  Reparatur überspringt ihn und synchronisiert, was folgt, neu, diese Bytes
  gehen also endgültig verloren. Der Reparaturdialog fragt ausdrücklich danach
  (**Auch N beschädigte Dateien neu synchronisieren**); bleibt das Kästchen
  leer, werden beschädigte Dateien nicht angefasst. Dasselbe gilt für eine
  Datei, deren Beschädigung erst beim Umschreiben auftaucht (**Beschädigung bei
  der Reparatur gefunden**): Der Scan liest nur die Köpfe und konnte sie nicht
  sehen.
- **Spuren blockweise abgelegt**: minutenlang Video, dann minutenlang Audio,
  statt nebeneinander. Kein Player kommt über den ersten Block hinaus.
  **Spuren neu verschachteln** schreibt sie ohne Neukodierung nebeneinander und
  prüft, dass jedes Paket angekommen ist, bevor die Datei ersetzt wird.

### Information

- **Unvollständiger Download**: Das Ende der Datei ist nie angekommen. Sie läuft
  bis zum Schnitt. Kein Werkzeug holt den fehlenden Teil zurück; nur die Quelle
  kann das.
- **Grobe Suche**: Der neu aufgebaute Index hat noch eine Lücke, meist im
  Abspann. Ein Sprung in die Lücke landet an ihrem Rand; überall sonst sind
  Sprünge exakt.
- **Ton bei +N s**: Eine Audiospur beginnt nach dem Bild. So wird eine
  Synchronfassung an einen Schnitt angeglichen, und jeder Player folgt dem. Evey
  schreibt das nie um.
- **Tonspuren N s auseinander**: Die eigenen Audiospuren der Datei stimmen nicht
  überein, eine davon sitzt also an der falschen Stelle. Keine Messung kann
  sagen, welche.
- **Ton N s neben dem Bild**: Eine Audiospur liegt entfernt von der Stelle, an
  der die Untertitel, die auf das Bild getimt sind, die Sprache verorten.
- **Tonspur auf 0 gezogen**: das Zeichen, das eine frühere Reparatur hinterlässt,
  wenn sie eine Spur um ihre angegebene Verzögerung zurückgezogen hat. Läuft
  die Synchronfassung jetzt zu früh, ist der entfernte Betrag nicht mehr in der
  Datei.
- **Audio -N s**: Eine Audiospur endet vor dem Bild. Die Wiedergabe füllt die
  Lücke mit Stille.
- **Überschüssige Bytes**: überzählige Bytes nach dem Ende des Dateiinhalts. Sie
  läuft gut.
- **Unlesbar**: Der Scan konnte die Datei nicht lesen (fehlt, nicht lesbar oder
  ein Container, den der Parser abgelehnt hat). Prüfen Sie die Einbindung oder
  die Quelle.

## Die Synchronität mit dem Auge prüfen

Für die drei Audio-Hinweise erstellt **Synchronität prüfen** in der Zeile der
Datei einen kurzen Ausschnitt auf dem Server: Wählen Sie einen Moment, eine Spur
und einen zu testenden Versatz, und schauen Sie ihn an, bei Bedarf verlangsamt.
Der Vergleich von Spuren sagt, ob sie übereinstimmen, nie, welche dem Bild folgt;
nur das Anschauen entscheidet.

**N s auf diese Spur anwenden** verschiebt diese Spur um den gewählten Versatz. Die Änderung wird
als Ihr Urteil gespeichert, nicht als Messung, und der **Verlauf** kann sie
rückgängig machen.

## Als gesehen markieren

**Gesehen, nicht mehr melden** (eine Datei) und **Alle als gesehen markieren**
(eine Gruppe) nehmen einen Informationshinweis aus der Ansicht **Wartend**.
Markieren lassen sich nur Hinweise: Ein reparierbarer Fehler wird repariert. Die
Markierung gehört zur Datei, wie sie ist: Ändern sich Größe oder Datum, stellt
der nächste Scan die Diagnose erneut. **Erneut melden** hebt die Markierung auf.

Die Ansichten über der Tabelle (**Wartend**, **Reparierbar**, **Info**,
**Gesehen**, **Sicherungen**, **Alle**) und das Suchfeld sind der Weg durch eine
große Bibliothek: **Wartend** ist die Aufgabenliste.

## Scans

Ein Scan liest den Anfang jeder Datei (einige Kilobyte) und verwendet die
vorherige Antwort für eine Datei weiter, die sich nicht geändert hat.
**Festplattennutzung** legt das Tempo fest:

- **Zurückhaltend**: eine Datei nach der anderen, dann eine Pause, so lang wie
  das Lesen gedauert hat. Die Festplatten bleiben etwa die Hälfte der Zeit frei,
  sodass jemand weiterschauen kann.
- **Schnell**: mehrere Dateien gleichzeitig, ohne Pause. Etwa dreimal schneller
  bei einer über mehrere Festplatten verteilten Bibliothek, aber die Wiedergabe
  kann dabei ruckeln.

Zwei Dinge lassen einen Scan alles neu diagnostizieren: eine geänderte Datei und
eine neue Version von Evey, deren Prüfungen sich geändert haben. Der erste Scan
nach einem solchen Update dauert bei einer großen Bibliothek daher lange; die
folgenden sind wieder schnell.

## Sicherungen, Verlauf und Rückgängigmachen

**Sicherungen behalten** ist standardmäßig an: Jede umgeschriebene Datei behält
ihr Original als `.mkvgobak` daneben. **Wiederherstellen** stellt eine Datei
wieder her, **Alle wiederherstellen** den gewählten Umfang, und **Sicherungen
löschen** entfernt sie endgültig. Ist die Option aus, existieren während eines
Laufs nur die letzten Kopien, und sie werden an dessen Ende entfernt; jede
Reparatur wird dann gründlicher geprüft, bevor sie die Datei ersetzt. Würde eine
Kopie jeder Datei das Volume füllen, sagt die Seite das vor dem Lauf.

Das **Reparaturjournal** listet jede Änderung, die eine Reparatur an den Bytes
einer Datei vorgenommen hat, über die ganze Bibliothek; der **Verlauf** zeigt sie
für eine Datei. Manche Änderungen lassen sich dort ohne Kopie rückgängig machen:

| Änderung | Rückgängig |
| --- | --- |
| Tonspur verschoben | Wendet genau die entgegengesetzte Verschiebung an. |
| In `.mp4` umbenannt | Benennt sie zurück. |
| Index neu aufgebaut | Nichts rückgängig zu machen: Bild und Ton bleiben unberührt. |
| Beschädigter Abschnitt übersprungen | Nur die Sicherung bringt die verlorenen Bytes zurück, deshalb behält diese Reparatur immer eine. |

Reparaturen schreiben in Ihre Medienordner. Diese müssen im Container mit
Schreibzugriff eingebunden sein (kein `:ro` am Volume); nach einem Lauf, der auf
ein schreibgeschütztes Volume gestoßen ist, sagt die Seite das. Scannen
funktioniert in beiden Fällen.

## Von der Kommandozeile

Dieselbe Maschinerie gibt es in der Betreiber-CLI (siehe `cli.md`):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` baut die
  fehlenden Indizes neu auf, als Probelauf, solange `--apply` fehlt;
- `evey admin validate-streaming --root <dir>` gibt den Bericht zur
  Streaming-Bereitschaft aus, ohne etwas zu schreiben.
