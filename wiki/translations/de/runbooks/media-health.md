---
source_sha: 4abf3c80428cd67c34ec53516788e91cc6019eab33ecc17b138c08d22a4bde30
---
# Medienzustand (Admin → Medienzustand)

Die Seite Medienzustand nimmt sich jede MKV- und WebM-Datei einer Bibliothek vor
und gibt jeder Datei EIN Urteil. Der Zweck: reparieren, was sich reparieren
lässt, und aufhören, ewig dieselben Warnungen zu lesen - nicht eine Liste mit
600 Befunden pflegen, auf die niemand reagiert.

## Die vier Urteile

| Urteil | Was es bedeutet | Was der Server dagegen tut |
|---|---|---|
| **Gesund** | Index, Ton-Timing, Container und Größen sind in Ordnung. | Nichts. |
| **Reparierbar** | Ein Defekt, den der Server selbst und gefahrlos beheben kann. | **Alle reparieren** behebt sie im Hintergrund allesamt (siehe unten). |
| **Ihre Entscheidung** | Ein Defekt, den nur die Quelle beheben kann, oder eine Reparatur mit echtem Preis. | Nichts von allein. Haken Sie ihn ab oder beschaffen Sie die Datei neu. |
| **Info** | Eine Merkwürdigkeit ohne Auswirkung auf die Wiedergabe. | Nichts. Haken Sie sie ab, um die Ansicht zu leeren. |

Jede Zeile trägt zusätzlich die Kennzeichen, die hinter ihrem Urteil stehen.
Die Kennzeichen, nach Urteil gruppiert:

**Reparierbar**

- Ein *fehlender* Suchindex oder ein Index, der zwar da, aber unbrauchbar ist
  (auf die Tonspur bezogen, auf Spuren verweisend, die die Datei nicht mehr hat,
  oder zu grob, um einen Sprung nahe am Ziel landen zu lassen). Die Reparatur
  baut die Cues neu auf: Die Datei wird in eine temporäre Kopie geschrieben,
  vollständig neu geprüft (Spuren, Dauer, Index) und dann eingetauscht. Sie
  kuriert langsames Springen und langsames Fortsetzen.
- *Ton bei +N s* - eine Tonspur beginnt später als das Bild (ein Fehler beim
  Muxen, die klassischen „Lippen vor dem Ton“). Die Reparatur richtet die Spur
  an Ort und Stelle neu aus.
- *Falscher Container* - die `.mkv` ist in Wahrheit eine MP4- oder MOV-Datei.
  Einen Matroska-Index gibt es hier nicht aufzubauen; die Reparatur **benennt sie
  in `.mp4` um** und aktualisiert den Bibliothekseintrag. Neu geschrieben wird
  nichts.
- *Beschädigt ~Größe* - ein beschädigter Abschnitt mitten in der Datei, an dem
  ein striktes Neuschreiben scheitert. Die Reparatur kann **neu
  synchronisieren**: Sie überspringt den beschädigten Bereich und setzt danach
  wieder auf. Diese Bytes sind endgültig verloren, deshalb fragt der Dialog
  ausdrücklich danach (*Auch N beschädigte Dateien neu synchronisieren*) - ohne
  Häkchen bleiben beschädigte Dateien unangetastet.

**Ihre Entscheidung**

- *Unvollständiger Download - fehlendes Ende ~Größe* - das deklarierte Ende der
  Datei liegt jenseits ihrer tatsächlichen Größe, und der Durchlauf hat den
  Abriss bestätigt. Das fehlende Ende bekommt kein Werkzeug zurück: Die Größe
  sagt Ihnen, ob es um einen Rumpf von 9 KB geht (bei der Wiedergabe unsichtbar)
  oder um eine fehlende Schlussszene. Wirklich beheben lässt sich das nur, indem
  Sie die Quelle neu beschaffen.
- *Audio -N s* - eine Tonspur endet N Sekunden **vor dem Bild** (gemessen wird
  Inhalt gegen Inhalt, nie gegen die deklarierte Dauer des Containers, die
  lediglich die Behauptung eines Muxers ist). Die Wiedergabe füllt das fehlende
  Ende mit Stille; der Ton steht schlicht nicht in der Datei.
- *Grobe Suche* - der Index wurde neu aufgebaut, und eine Lücke bleibt. Woran es
  liegt, sagt das Detail: Keyframes, die niemand in die Cues eingetragen hat
  (ein zweiter Neuaufbau schließt die Lücke), ein Abschnitt ganz ohne Keyframe
  (nur eine Neukodierung macht ihn anspringbar) oder **ein Abschnitt ganz ohne
  Bild** - dort friert das Bild ein, was der Index auch tut, und Abhilfe schafft
  allein die Quelle.

**Info**

- *Überschüssige Bytes* - Bytes jenseits des deklarierten Endes des Segments
  (ein abgebrochenes Anhängen, das Journal eines Torrent-Clients). Das Medium
  ist vollständig und läuft einwandfrei; jedes künftige Neuschreiben wirft sie
  weg.

## Abhaken

*Gesehen, nicht mehr melden* (je Datei) und *Alle als gesehen markieren* (je
Gruppe) nehmen einen Befund aus der Ansicht *Wartend*. Der Haken hängt an der
Identität der Datei **und an ihren Bytes**: Ändert sich die Datei (Größe oder
mtime), stellt der nächste Scan die Diagnose neu, und der Befund kommt zurück.
*Erneut melden* macht den Haken rückgängig. Ein Haken, den Sie während eines
laufenden Scans setzen, bleibt nach dessen Ende erhalten (sofern die Datei
unverändert ist).

Mit den Ansichten über der Tabelle (*Wartend*, *Reparierbar*, *Ihre
Entscheidung*, *Info*, *Gesehen*, *Sicherungen*, *Alle*) und dem Suchfeld
arbeiten Sie eine große Bibliothek ab: *Wartend* ist die Aufgabenliste,
*Gesehen* ist das, worüber Sie schon entschieden haben.

## Scans, und warum der erste Scan nach einer Aktualisierung lange dauert

Ein Scan liest von jeder Datei nur den Kopf (ein paar KB) und übernimmt das
vorige Urteil, solange sich die Datei nicht geändert hat. Zwei Dinge erzwingen
eine frische Diagnose für alles:

- die Datei hat sich geändert (Größe oder mtime);
- **die Erkennung im Server hat sich geändert** - eine neue Regel, eine
  korrigierte Bezugsgröße, ein verschobener Schwellwert. Jede solche Änderung
  erhöht eine interne Diagnoseversion, und der nächste Scan stellt die Diagnose
  einmalig für die ganze Bibliothek neu, statt ein Urteil zu zeigen, das eine
  neuere Regel nicht mehr fällen würde. Auf einem NAS sind das bei einer großen
  Bibliothek Stunden; die Scans danach sind wieder schnell.

## Sicherungen und Zurückrollen

**Lesen Sie das, bevor Sie in einer Störung nach *Wiederherstellen* greifen: Ob
es überhaupt eine physische Kopie des Originals gibt, hängt an der
Reparaturstrategie, und eine der drei hebt keine auf.**

| Strategie | Physische Sicherung | Wie das Zurückrollen funktioniert |
|---|---|---|
| `rewrite` | `.mkvgobak` neben der Datei | Die Kopie zurückspielen |
| `auto` | `.mkvgobak` (fällt auf ein Neuschreiben zurück) | Die Kopie zurückspielen |
| `inplace` | **keine, niemals** | Die umgekehrte Verschiebung anwenden, die im Reparaturprotokoll steht |

`inplace` ist ein chirurgischer Eingriff - ein Schreibvorgang in der Datei, kein
Remux - und diesen Weg nimmt die Neuausrichtung des Tons (das Kennzeichen „Ton
bei +N s“). Kopiert wird dabei nichts: Der Weg zurück ist der Eintrag im
`audit_log`, der die angewandte Verschiebung festhält, und der Umstand, dass die
entgegengesetzte Verschiebung sie aufhebt. Ist dieser Eintrag weg, ist auch das
Zurückrollen weg. Maßgeblich für die drei Strategien ist
`admin_media.proto`; `inplace` ignoriert sowohl *Sicherungen behalten* als auch
die Zahl der aufzubewahrenden Sicherungen.

Für die beiden Strategien, die eine Kopie aufheben, gilt: *Wiederherstellen*
holt eine einzelne Datei zurück, *Alle wiederherstellen* den gewählten Bereich.
Würden die Sicherungen das Volume sprengen, sagt die Seite das und schlägt vor,
sie abzuschalten - dann bleiben während des Laufs nur die letzten paar erhalten
und werden am Ende entfernt, wobei jede Reparatur vor dem Tausch geprüft wurde.

Reparaturen brauchen die Medienordner **les- und schreibbar** im Container
eingehängt (kein `:ro` am Bind-Mount) - die Seite nennt Ihnen die Dateien, die
auf einem nur lesbaren Volume liegen.

## Die Entsprechungen auf der Kommandozeile

Dieselbe Maschinerie steht auf der Kommandozeile für Betreiber bereit (siehe
`cli.md`):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` - der Neuaufbau
  des Index, standardmäßig ein Trockenlauf;
- `evey admin validate-streaming --root <dir>` - der nur lesende Bericht zur
  Streaming-Tauglichkeit.
