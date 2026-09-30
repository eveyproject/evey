---
source_sha: a10d4c679e7aed09a2fa56ed708a6beb299e8c731fcea4016e7cbf7b9f1e30e5
reviewed: false
---
# Hintergrundaufgaben und Serverlast

Neben der Wiedergabe erledigt der Server eigene Arbeit: Bibliotheken scannen,
Bilder holen, die Vorschaubilder der Zeitleiste erzeugen, Intros und Abspann
erkennen, Dateien analysieren (Keyframes, Codec-Level, Medienzustand). Diese
Seite erklärt, wie sich diese Arbeit die Maschine mit den Zuschauern teilt.

## Die Wiedergabe hat Vorrang

Hintergrundaufgaben nutzen einen Anteil des Prozessors, standardmäßig **25 %**
(**Admin → Serverkonfiguration → Zugeteilte Rechenleistung**). **50 %**
beschleunigt sie, für Zeiten, in denen niemand den Server nutzt; eine schwere
Aufgabe (Medienanalyse, Vorschaubilder, Intro-Erkennung, Klassifizierung) kann
einen eigenen Anteil erhalten. Eine Änderung gilt ab dem nächsten verarbeiteten
Element.

Unabhängig vom Anteil pausiert eine Hintergrundaufgabe, während jemand schaut,
und setzt danach fort. Bei einer neuen Bibliothek kommen Vorschaubilder und
Intro-Markierungen daher über Stunden statt Minuten, wenn der Server genutzt
wird.

## Was im Hintergrund läuft

| Arbeit | Was sie bringt | Wann sie läuft |
| --- | --- | --- |
| Bibliotheksscan | Neue, geänderte und entfernte Dateien | Bei Dateiänderungen, nach Zeitplan, beim Start, falls aktiviert. Siehe [Bibliotheken und Metadaten](../administration/libraries.md). |
| Bilder herunterladen | Poster und Hintergründe auf dem Server, in WebP | Während des Scans; fehlende werden stündlich erneut versucht. Aus: Die Bilder lädt der Browser aus dem Internet, was manche Browser blockieren. |
| Vorschaubilder der Zeitleiste | Die Vorschauen beim Ziehen der Zeitleiste (ein Bild alle 7 Sekunden) | Wenn ein Titel hinzukommt, und beim Start für die ohne. Lässt sich unter **Serverkonfiguration** abschalten. |
| Intro- und Abspann-Erkennung | Die Schaltflächen **Überspringen** | Fortlaufend, für Titel, die Community-Datenbanken nicht abdecken |
| Keyframe-Index und Codec-Level | Direktwiedergabe und Kopie ohne Neukodierung, korrekte Farben | Fortlaufend, bei Dateien, die vor dem Erfassen dieser Informationen gescannt wurden |

## Speicher

**Speicherprofil** (**Admin → Serverkonfiguration**): **Niedrig** hält
untätige Plugins an, um Speicher zu sparen, und weckt sie bei Bedarf. **Auto
(empfohlen)** wählt Niedrig auf Maschinen mit weniger als 3 GB Speicher.

## Beim Start gelesene Einstellungen

Einige Einstellungen werden einmal gelesen, wenn der Server startet: Beim
Speichern wird Ihre Wahl abgelegt, ein Neustart wendet sie an. Der
Admin-Bereich sagt das neben ihnen. Es sind die Karte **Hintergrundarbeiten**
(Pausieren und Taktung der Keyframe- und Codec-Level-Durchläufe), der Schalter
**Intro-Skip-Anbieter** und die **Erweiterten Grenzwerte** der Transkodierung.
