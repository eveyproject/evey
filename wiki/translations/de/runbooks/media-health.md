---
source_sha: 62df894506621ac2b76e3b103c93c675fdf1c4c85203b1ce7599afdc157bbca8
reviewed: true
---
# Medienzustand { #media-health }

Bei einem Titel dauert das Springen lange, die Wiedergabe bleibt an einer
Stelle hängen, oder der Ton verschiebt sich: **Verwaltung → Medienzustand** liest jede MKV/WebM-Datei
Ihrer Bibliotheken und gibt jeder eine einzige Antwort. Ziel ist eine Seite,
die sich leert: Was Evey beheben kann, behebt es (**Alle reparieren**); der
Rest wird einmal erwähnt und kann als gesehen markiert werden.

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Medienzustand nach einem Scan: eine Datei zu reparieren, ein Hinweis](../images/admin-media-health.webp){ loading=lazy }
  <figcaption>Ein Scan mit einer Datei, die Evey reparieren kann (ihr Seek-Index fehlt), und einem Hinweis (überschüssige Bytes nach dem Ende der Datei, harmlos) (Oberfläche auf Englisch).</figcaption>
</figure>

## Zwei Urteile { #two-verdicts }

Eine Datei, die nicht **Gesund** ist (Index, Container, Größen und das Timing
des Tons stimmen), bekommt eine von zwei Antworten, und mit Absicht nur zwei:

| Urteil | Was es bedeutet | Was Evey tut |
| --- | --- | --- |
| **Reparierbar** | Ein Fehler, den Evey auf diesem Server repariert. | **Alle reparieren** erledigt sie im Hintergrund; die Zahl auf der Schaltfläche ist genau ihre Zahl. |
| **Info** | Etwas, das an der Datei gemessen wurde und das Evey nicht von selbst umschreibt. | Nichts. Markieren Sie es als gesehen. |

Keines der beiden Urteile verlangt manuelle Arbeit von Ihnen. Eine Messung,
mit der Evey nichts anfangen kann, ist nur ein Hinweis, und eine Messung kann
falsch sein: Läuft die Datei korrekt, ist nichts zu tun.

### Reparierbar { #repairable }

- **Ohne Seek-Index**, oder mit einem, der nicht nutzbar ist. Man merkt es an
  langsamem Springen oder langsamem Fortsetzen. Der Index wird in einer
  vorläufigen Kopie neu aufgebaut. Die Kopie wird vollständig geprüft (Spuren,
  Dauer, Index) und ersetzt dann die Datei.
- **Falscher Container**: Die `.mkv` ist in Wahrheit eine MP4- oder MOV-Datei.
  Sie wird in `.mp4` umbenannt, und die Bibliothek übernimmt die Änderung; nichts wird
  umgeschrieben.
- **Beschädigter Abschnitt**: eine beschädigte Stelle mitten in der Datei. Die
  Reparatur überspringt sie und synchronisiert, was folgt, neu. Diese Bytes
  sind also für immer verloren. Der Reparaturdialog fragt ausdrücklich danach
  (**Auch N beschädigte Dateien neu synchronisieren**); bleibt das Kästchen
  leer, werden beschädigte Dateien nicht angefasst. Dasselbe gilt für eine
  Datei, deren Beschädigung erst beim Umschreiben auftaucht (**Beschädigung
  bei der Reparatur gefunden**): Der Scan liest nur die Kopfdaten und konnte
  sie nicht sehen.
- **Spuren blockweise abgelegt**: minutenlang Video, dann minutenlang Ton,
  statt beides nebeneinander. Kein Player kommt über den ersten Block hinaus.
  **Spuren neu verschachteln** schreibt sie ohne Neukodierung nebeneinander
  und prüft, dass jedes Paket angekommen ist, bevor die Datei ersetzt wird.

### Info { #info }

- **Unvollständiger Download**: Das Ende der Datei ist nie angekommen. Sie
  läuft bis zu der Stelle, an der sie abbricht. Kein Werkzeug holt den
  fehlenden Teil zurück; das kann nur die Quelle.
- **Grobes Springen**: Der neu aufgebaute Index hat noch eine Lücke, meist im
  Abspann. Ein Sprung in die Lücke endet an ihrem Rand; überall sonst sind
  Sprünge genau.
- **Ton bei +N s**: Eine Tonspur beginnt nach dem Bild. So wird eine
  Synchronfassung an eine Schnittfassung angepasst, und jeder Player folgt
  dem. Evey schreibt das nie um.
- **Tonspuren N s auseinander**: Die Tonspuren der Datei passen nicht
  zueinander, eine davon sitzt also an der falschen Stelle. Keine Messung kann
  sagen, welche.
- **Ton N s neben dem Bild**: Eine Tonspur liegt nicht dort, wo die
  Untertitel, die auf das Bild abgestimmt sind, das Gesprochene verorten.
- **Tonspur auf 0 gezogen**: das Zeichen, das eine frühere Reparatur
  hinterlässt, wenn sie eine Spur um ihre angegebene Verzögerung vorgezogen
  hat. Läuft die Synchronfassung jetzt zu früh, ist der entfernte Betrag nicht
  mehr in der Datei.
- **Audio -N s**: Eine Tonspur endet vor dem Bild. Die Wiedergabe füllt die
  Lücke mit Stille.
- **Überschüssige Bytes**: zusätzliche Bytes nach dem Ende des Inhalts der
  Datei. Sie läuft problemlos.
- **Unlesbar**: Der Scan konnte die Datei nicht lesen (sie fehlt, ist nicht
  lesbar, oder ihr Container wurde vom Parser abgelehnt). Prüfen Sie die
  Einbindung des Laufwerks oder die Quelle.

## Die Synchronität mit dem Auge prüfen { #checking-the-sync-by-eye }

Für die drei Hinweise zum Ton baut **Synchronität prüfen** in der Zeile der
Datei einen kurzen Clip auf dem Server: Wählen Sie einen Moment, eine Spur und
einen Versatz zum Testen, dann schauen Sie ihn an, bei Bedarf verlangsamt.
Vergleicht man Spuren, erfährt man, ob sie zueinander passen, nie, welche zum
Bild passt; das entscheidet nur das Hinschauen.

**N s auf diese Spur anwenden** verschiebt diese Spur um den gewählten
Versatz. Die Änderung wird als Ihr Urteil gespeichert, nicht als Messung, und
**Verlauf** kann sie rückgängig machen.

## Als gesehen markieren { #marking-as-seen }

**Gesehen, nicht mehr melden** (eine Datei) und **Alle als gesehen
markieren** (eine Gruppe) nehmen einen Info-Hinweis aus der Ansicht
**Wartend**. Markieren lassen sich nur Hinweise: Ein reparierbarer Fehler wird
repariert. Die Markierung gilt für die Datei, wie sie gerade ist: Ändern sich
ihre Größe oder ihr Datum, stellt der nächste Scan die Diagnose neu. **Erneut
melden** hebt die Markierung auf.

Mit den Ansichten über der Tabelle (**Wartend**, **Reparierbar**, **Info**,
**Gesehen**, **Sicherungen**, **Alle**) und dem Suchfeld arbeiten Sie sich
durch eine große Bibliothek: **Wartend** ist die Liste der offenen Punkte.

## Scans { #scans }

Ein Scan liest drei kleine Bereiche jeder Datei (den Anfang, den Index nahe dem Ende und das Ende). Für eine Datei, die
sich nicht geändert hat, übernimmt er die vorherige Antwort.
**Festplattennutzung** legt das Tempo fest:

- **Zurückhaltend**: eine Datei nach der anderen, dann eine Pause, so lang wie
  das Lesen gedauert hat. Die Festplatten bleiben etwa die Hälfte der Zeit
  frei, so dass jemand weiterschauen kann.
- **Schnell**: mehrere Dateien gleichzeitig, ohne Pause. Spürbar
  schneller, wenn die Bibliothek auf mehreren Festplatten liegt, aber die
  Wiedergabe kann währenddessen ruckeln.

Zwei Dinge lassen einen Scan alles neu diagnostizieren: eine Datei, die sich
geändert hat, und eine neue Version von Evey, deren Prüfungen sich geändert
haben. Der erste Scan nach einem solchen Update dauert bei einer großen
Bibliothek daher lange; die folgenden sind wieder schnell.

## Sicherungen, Verlauf und Rückgängigmachen { #backups-history-and-undo }

**Sicherungen behalten** ist standardmäßig an: Jede umgeschriebene Datei
behält ihr Original als `.mkvgobak` daneben. **Wiederherstellen** stellt eine
Datei wieder her, **Alle wiederherstellen** die gewählte Auswahl, und
**Sicherungen löschen** entfernt die Sicherungen endgültig. Ist die Option
aus, gibt es während eines Laufs nur die letzten paar Kopien, und sie werden
an seinem Ende entfernt; jede Reparatur wird dann gründlicher geprüft, bevor
sie die Datei ersetzt. Würde eine Kopie jeder Datei das Volume füllen, sagt
die Seite das vor dem Lauf.

Das **Reparaturjournal** listet jede Änderung, die eine Reparatur an den
Bytes einer Datei gemacht hat, für die ganze Bibliothek; **Verlauf** zeigt sie
für eine einzelne Datei. Manche Änderungen lassen sich dort ohne jede Kopie
rückgängig machen:

| Änderung | Rückgängig machen |
| --- | --- |
| Tonspur verschoben | Wendet genau die entgegengesetzte Verschiebung an. |
| In .mp4 umbenannt | Benennt die Datei zurück. |
| Index neu aufgebaut | Nichts rückgängig zu machen: Bild und Ton bleiben unberührt. |
| Beschädigter Abschnitt übersprungen | Nur die Sicherung bringt die verlorenen Bytes zurück. Deshalb behält diese Reparatur immer eine. |

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Medienzustand nach Alle reparieren, mit geöffnetem Reparaturjournal](../images/admin-media-health-repaired.webp){ loading=lazy }
  <figcaption>Nach <strong>Alle reparieren</strong>: Der Index ist neu aufgebaut, das Journal hält es fest, und die Datei verlässt die Warteschlange; der Hinweis bleibt. Dieser Index wurde direkt in die Datei eingefügt, dabei bleibt keine Kopie: Nur eine Datei, die neu geschrieben werden muss, behält eine Sicherung (Oberfläche auf Englisch).</figcaption>
</figure>

Reparaturen schreiben in Ihre Medienordner. Diese müssen im Container mit
Schreibrecht eingebunden sein (kein `:ro` am Volume). Ist ein Lauf auf ein
schreibgeschütztes Volume gestoßen, sagt die Seite das. Scannen funktioniert
in beiden Fällen.

## Von der Kommandozeile { #from-the-command-line }

Dieselbe Technik gibt es in der Kommandozeile für Betreiber, siehe
[Kommandozeile](cli.md#media-maintenance):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` baut die
  fehlenden Indizes neu auf, als Probelauf, solange `--apply` fehlt;
- `evey admin validate-streaming --root <dir>` gibt den Bericht aus, ob die
  Dateien für das Streaming bereit sind, ohne etwas zu schreiben.

<!-- sources
verdicts (two non-healthy answers, the dropped third one): internal/app/maintenance/seek_index.go - Verdict, SeekStatus.Verdict, Ackable ; labels: i18n admin.seekIndex.verdictHealthy/verdictRepairable/verdictInformational
labels and hints: i18n admin.seekIndex.*, admin.repairJournal.*, admin.repairHistory.*, admin.syncPreview.*
what can be undone without a copy: internal/app/maintenance/repair_journal.go - RepairRecord.Reversible
apply an audio offset: internal/adapters/driving/grpcapi/admin_media.go - ApplyAudioShift
keep backups on by default: frontend/apps/web/src/features/admin/seekIndex/hooks.ts
-->
