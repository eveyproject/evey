---
source_sha: 3d48e5430ddf686fb21cfa258b7c737dcc71adf37151f1ae4b3b09e68fd5849e
reviewed: false
---
# Sprachen und Barrierefreiheit

Eine ehrliche Bestandsaufnahme, wo Evey bei der Internationalisierung (i18n) und
der Barrierefreiheit (a11y) steht: was funktioniert, wie es funktioniert und was
noch nicht. Dies ist ein selbst gehostetes Projekt, das von einer Person gebaut
und gepflegt wird - betrachten Sie insbesondere die Barrierefreiheit als *nach
bestem Bemühen, nicht zertifiziert*.

---

## Internationalisierung (i18n)

### Was übersetzt ist und was nicht

- **Übersetzt:** die Bedienoberfläche - Schaltflächen, Menüs, Einstellungen,
  Fehlermeldungen, Texte von Push-Benachrichtigungen, Benachrichtigungs-E-Mails
  (in der Sprache des Empfängers). E-Mails zum Zurücksetzen des Passworts werden
  auf Englisch versendet.
- **Nicht von Evey übersetzt:** Die Metadaten der Medien (Titel,
  Zusammenfassungen, Genres) kommen von Ihrer Metadatenquelle (TMDB/OMDb) in der
  **Metadatensprache**, die im Einrichtungsassistenten oder unter **Admin →
  Serverkonfiguration → Sprache & Region** gewählt ist (`EVEY_METADATA_LANGUAGE`
  ist nur der Standard), unabhängig von der Sprache der Oberfläche. Ihre eigenen
  Bibliotheken, Dateinamen und alles, was Sie eingeben, werden unverändert
  gespeichert.

Die Oberfläche kann also auf Französisch sein, während die Zusammenfassung eines
Films auf Englisch ist (oder umgekehrt) - es sind zwei getrennte Einstellungen.

### Unterstützte Sprachen der Oberfläche

Sechs: **Englisch (en)**, **Französisch (fr)**, **Deutsch (de)**, **Spanisch
(es)**, **Chinesisch (zh)**, **Hindi (hi)**. Englisch ist die Ausgangssprache
und der Rückfall: Jede in einer anderen Sprache fehlende Zeichenkette fällt auf
Englisch zurück, statt leer oder als Schlüssel zu erscheinen.

### Wie es funktioniert

- **Eine einzige Quelle der Wahrheit:** [`/i18n/<locale>/*.yaml`](../i18n) im
  Wurzelverzeichnis des Repositorys, in Namensräume aufgeteilt (`common`,
  `auth`, `media`, `player`, `admin`, `errors`, `notifications`, `mobile`,
  `downloads`, `requests`). Derselbe Katalog speist jede Oberfläche.
- Der **Server** lädt ihn und liefert ihn unter `GET /api/v1/i18n/{locale}` aus
  und nutzt ihn direkt für übersetzte Fehlermeldungen der API und Texte von
  Benachrichtigungen.
- Die **Apps für Web, Mobil und TV** laden die Übersetzungen von diesem Endpunkt
  (i18next + http-backend), speichern sie lokal zwischen und halten
  `<html lang>` mit der aktiven Sprache synchron, damit Hilfstechnologien die
  richtige Sprache ansagen.
- Die Sprache kommt aus der Vorliebe Ihres Kontos, sonst aus der Sprache des
  Browsers oder Geräts (`Accept-Language` wird gelesen, ohne Region:
  `fr-FR` → `fr`), sonst aus der **Serversprache** unter **Admin →
  Serverkonfiguration → Sprache & Region** (die auch die Sprache neuer Konten
  festlegt), sonst Englisch.
- Interpolation und Pluralformen laufen über i18next (`{{count}}` usw.).

### Ehrliche Grenzen

- **Die Vollständigkeit schwankt.** Englisch ist maßgeblich und immer
  vollständig; die anderen fünf werden von Hand mit einem Prüfskript auf
  gleichem Stand gehalten, aber eine ganz neue Funktion kann zuerst auf Englisch
  erscheinen und zurückfallen, bis sie übersetzt ist.
- **Noch keine Sprachen mit Schreibrichtung von rechts nach links.** Es gibt
  kein Arabisch, Hebräisch oder Persisch, und das Layout wurde nicht für RTL
  gebaut oder getestet - eine solche Sprache hinzuzufügen ist echte Arbeit
  (logische CSS-Eigenschaften, Umgang mit `dir`), nicht nur eine
  Übersetzungsdatei.
- **Die Übersetzungen sind nicht professionell geprüft.** Sie haben die Qualität
  der Community und des Maintainers. Korrekturen sind willkommen.
- **Die sprachabhängige Formatierung ist unvollständig.** Datumsangaben und
  Zahlen stützen sich auf `Intl` der Plattform bzw. die Sprache des Browsers und
  sind nicht auf jedem Bildschirm einheitlich verdrahtet.

### Eine Sprache hinzufügen oder korrigieren

1. Kopieren Sie `i18n/en/` nach `i18n/<your-locale>/` und übersetzen Sie die
   YAML-Werte (die Schlüssel bleiben).
2. Fügen Sie die Sprache der Erlaubnisliste in
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) (`supportedLocales`)
   hinzu.
3. Halten Sie die Schlüsselmenge in allen Sprachen gleich - sonst schlägt die
   Gleichstandsprüfung fehl.

---

## Barrierefreiheit (a11y)

### Wo sie steht

Evey ist mit semantischem HTML, ARIA-Beschriftungen und den
Barrierefreiheits-Eigenschaften von React Native gebaut, und einiges unten ist
bewusst so. Es wurde aber **nicht** formell nach WCAG geprüft, es gibt **keine
automatisierten Barrierefreiheitstests** in der Testsuite, und es wurde **nicht**
systematisch mit Screenreadern getestet. Teile davon sind wahrscheinlich mit
Hilfstechnologien nutzbar; Teile wahrscheinlich nicht. Wenn Sie auf
Hilfstechnologien angewiesen sind, rechnen Sie mit Ecken und Kanten und melden
Sie bitte Einzelheiten.

### Was vorhanden ist

- **Reduzierte Bewegung:** `prefers-reduced-motion: reduce` wird global beachtet -
  Animationen und Übergänge sind praktisch abgeschaltet, wenn Ihr System es
  verlangt.
- **Sprache für Hilfstechnologien:** `<html lang>` folgt der aktiven
  Oberflächensprache.
- **Tastatur (Player):** vollständige Tastatursteuerung mit einer Übersicht der
  Kürzel auf dem Bildschirm (`?`), und die Kürzel sind unterdrückt, während Sie
  in ein Feld tippen.
- **Beschriftete Bedienelemente:** Schaltflächen nur mit Symbol und Schieberegler
  tragen im Web `aria-label` und auf Mobilgeräten `accessibilityLabel`/
  `accessibilityRole` (der größere Teil der interaktiven Elemente, nicht alle).
- **Themes:** fünf Themes sind eingebaut (Standard dunkel, AMOLED, dunkelrot,
  hell-standard, hell-blau); mehr über Theme-Plugins.
- **Android TV:** Umgang mit dem D-Pad-Fokus für die Bedienung per Fernbedienung.

### Ehrliche Lücken (noch nicht erledigt)

- **Kein Durchgang mit Screenreadern.** Abläufe mit NVDA/VoiceOver/TalkBack sind
  ungeprüft; manche eigenen Bausteine (Menüs, Dialoge, die Zeitleiste,
  Spurauswahl) sagen ihren Zustand oder ihre Rolle womöglich nicht korrekt an.
- **Die Fokusführung ist nicht geprüft.** Die Darstellung des sichtbaren Fokus
  und die Fokusreihenfolge in Dialogen und Überlagerungen sind stellenweise
  uneinheitlich.
- **Der Kontrast wird nur für die eingebauten Themes gemessen.** Ein Unit-Test
  (`frontend/packages/theme/src/contrast.test.ts`) hält jedes mitgelieferte Theme
  bei WCAG AA 4,5:1 für den Akzent, die drei Textabstufungen und die
  Gefahrenfarben gegenüber dem Hintergrund und beiden Flächen; die TV-App prüft
  ihre abgeleiteten Werte erneut. Theme-Plugins und die Farben, die ein Plugin
  zur Laufzeit einfügt, werden nicht geprüft - ein eigenes Theme kann stellenweise
  durchfallen. AAA wird nicht angestrebt.
- **Keine Textskalierung in der App** über den Zoom von Browser oder System
  hinaus, und kein eigener Modus mit hohem Kontrast.
- **Die Gestaltung von Untertiteln ist auf Mobilgeräten eingeschränkt** (eine
  Beschränkung des zugrunde liegenden Players `expo-video`), auch wenn
  Untertitelspuren selbst abgespielt werden.

### Ein Problem melden

Nennen Sie Seite oder Bildschirm, das betroffene Bedienelement, was Sie erwartet
haben und die verwendete Hilfstechnologie mit Version. Konkrete Meldungen werden
behoben; „es ist nicht barrierefrei" lässt sich nicht umsetzen. Korrekturen der
Barrierefreiheit erfolgen nach bestem Bemühen.
