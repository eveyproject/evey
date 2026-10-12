---
source_sha: 935a77e1c7b524e4df14c0a67f9ad57f4571098b770e8c3245fda5189aefc0c3
reviewed: true
---
# Hintergrundarbeiten und Serverlast

Warum der Server beschäftigt ist, wenn niemand schaut, und warum die
Vorschaubilder einer neuen Bibliothek Stunden brauchen: Neben der Wiedergabe
erledigt der Server eigene Arbeit (Bibliotheken scannen, Bilder holen,
Vorschaubilder für die Zeitleiste erzeugen, Intros und Abspann erkennen,
Dateien analysieren). Diese Seite sagt, wie sich diese Arbeit den Rechner mit
den Zuschauern teilt und wo Sie ihren Anteil einstellen.

## Die Wiedergabe hat Vorrang

Hintergrundaufgaben bekommen einen Anteil am Prozessor, den **Globaler
Anteil** unter **Verwaltung → Serverkonfiguration → Zugeteilte
Rechenleistung**: standardmäßig **25 %**, **50 %**, um sie zu beschleunigen,
wenn niemand den Server benutzt. Unter **Schwere Aufgaben** kann eine von
ihnen (Analyse neuer Medien, Trickplay-Vorschaubilder, Intro-Erkennung,
Einstufung der Komplexität) einen eigenen Anteil bekommen, der für diese
Aufgabe den globalen ersetzt; Anteile addieren sich nie. Eine Änderung gilt ab
dem nächsten bearbeiteten Eintrag, ohne Neustart.

Egal wie groß der Anteil ist: Eine Hintergrundaufgabe pausiert, solange
jemand schaut, und macht danach weiter. Bei einer neuen Bibliothek kommen
Vorschaubilder und Intro-Markierungen deshalb über Stunden statt Minuten,
wenn der Server benutzt wird.

## Was im Hintergrund läuft

| Arbeit | Was sie bringt | Wann sie läuft |
| --- | --- | --- |
| Bibliotheks-Scan | Neue, geänderte und entfernte Dateien | Bei Dateiänderungen, nach Zeitplan, beim Start, wenn eingeschaltet. Siehe [Bibliotheken und Metadaten](../administration/libraries.md). |
| Herunterladen der Bilder | Poster und Hintergrundbilder, auf dem Server aufbewahrt, in WebP | Während des Scans; fehlende werden stündlich erneut versucht. Ausgeschaltet: Der Browser lädt die Bilder aus dem Internet, was manche Browser blockieren. |
| Vorschaubilder der Zeitleiste | Die Vorschauen, die beim Ziehen der Zeitleiste erscheinen (ein Bild alle 7 Sekunden) | Wenn ein Titel hinzukommt, und beim Start für die Titel ohne. **Trickplay-Vorschaubilder aktivieren** unter **Serverkonfiguration** schaltet sie ab. |
| Erkennung von Intro und Abspann | Die Schaltflächen zum **Überspringen** | Laufend, für Titel, die Community-Datenbanken nicht abdecken |
| Keyframe-Index und Codec-Level | Direct Play und Kopieren ohne Neukodierung, richtige Farben | Laufend, für Dateien, die gescannt wurden, bevor diese Angaben gesammelt wurden |

## Arbeitsspeicher

**Speicherprofil** (**Verwaltung → Serverkonfiguration**): **Niedrig** legt
Plugins, die nichts tun, schlafen, um Arbeitsspeicher zu sparen, und weckt sie
bei Bedarf. **Auto (empfohlen)** wählt Niedrig auf Rechnern mit weniger als
3 GB Arbeitsspeicher.

## Einstellungen, die beim Start gelesen werden

Einige Einstellungen werden nur einmal gelesen, wenn der Server startet: Beim
Speichern wird Ihre Wahl festgehalten, ein Neustart wendet sie an. Der
Verwaltungsbereich sagt das neben ihnen. Es sind die Karte
**Hintergrundarbeiten** (Pausieren und Tempo der Durchgänge für Keyframes und
Codec-Level), der Schalter **Intro-Skip-Anbieter** und die **Erweiterte
Grenzwerte** der Transkodierung.

<!-- sources
processing power, heavy tasks, pause during playback: i18n admin.serverConfig.cpuBudget.* (globalLabel, tasksTitle, tasksDesc, applyHint) ; admin.activity.bgTaskHelp.*
thumbnails switch: i18n admin.serverConfig.trickplayEnabled
artwork: i18n admin.serverConfig.artworkInfo, artworkRetryDesc
thumbnails: i18n admin.activity.bgTaskHelp.trickplay_generation
memory: i18n admin.serverConfig.memoryProfileDesc
read at startup: i18n admin.serverConfig.backgroundJobs.restartWarning, introProvidersRestart ; docs/playback/transcoding.md
-->
