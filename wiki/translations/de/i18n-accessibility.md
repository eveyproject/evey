---
source_sha: 6ad73949864d3c9b0dea20d620fb0f5e4a2b176cba0f54c8e15090f66f2b929c
---
# Sprachen und Barrierefreiheit

Eine ehrliche Bestandsaufnahme dessen, wo Evey bei der Internationalisierung
(i18n) und der Barrierefreiheit (a11y) steht: was funktioniert, wie es
funktioniert und was noch fehlt. Das Projekt hostet man selbst, gebaut und
gepflegt wird es von einer einzigen Person - nehmen Sie gerade die
Barrierefreiheit deshalb als *nach bestem Bemühen, nicht als zertifiziert*.

---

## Internationalisierung (i18n)

### Was übersetzt wird und was nicht

- **Übersetzt:** der Rahmen der Oberfläche - Schaltflächen, Menüs,
  Einstellungen, Fehlermeldungen sowie die Texte der Benachrichtigungen per
  E-Mail und Push.
- **Von Evey nicht übersetzt:** die Metadaten Ihrer Medien (Titel,
  Inhaltsangaben, Genres) kommen von Ihrem Metadatenanbieter (TMDB oder OMDb),
  und zwar in der **Metadatensprache**, die Sie einstellen
  (`EVEY_METADATA_LANGUAGE`) - sie ist von der Sprache der Oberfläche
  unabhängig. Ihre eigenen Bibliotheken, die Dateinamen und alles, was Sie
  selbst eintippen, werden unverändert gespeichert.

Die Oberfläche darf also auf Französisch stehen, während die Inhaltsangabe eines
Films auf Englisch erscheint (oder umgekehrt) - das sind zwei getrennte
Einstellungen.

### Unterstützte Sprachen der Oberfläche

Sechs: **Englisch (en)**, **Französisch (fr)**, **Deutsch (de)**, **Spanisch
(es)**, **Chinesisch (zh)** und **Hindi (hi)**. Englisch ist die Ausgangssprache
und zugleich der Rückfall: Fehlt eine Zeichenkette in einer anderen Sprache,
erscheint die englische Fassung statt einer Lücke oder eines Schlüssels.

### Wie das funktioniert

- **Eine einzige maßgebliche Quelle:** [`/i18n/<locale>/*.yaml`](../i18n) im
  Wurzelverzeichnis des Repositorys, aufgeteilt in Namensräume (`common`,
  `auth`, `media`, `player`, `admin`, `errors`, `notifications`, `mobile`,
  `downloads`, `requests`). Derselbe Katalog speist jede Oberfläche.
- Der **Server** lädt ihn, liefert ihn unter `GET /api/v1/i18n/{locale}` aus und
  greift für übersetzte Fehlermeldungen der API und für die Texte der
  Benachrichtigungen unmittelbar darauf zurück.
- Die **Anwendungen für Web, Mobilgeräte und Fernseher** laden die Übersetzungen
  von diesem Endpunkt (i18next mit http-backend), legen sie lokal im Cache ab
  und halten `<html lang>` auf dem Stand der aktiven Sprache, damit
  Hilfstechnologien die richtige Sprache ansagen.
- Welche Sprache es wird, entscheidet zuerst die Einstellung Ihres Kontos, dann
  die Sprache des Browsers beziehungsweise des Geräts (`Accept-Language` wird
  ausgewertet, die Region dabei verworfen: `fr-FR` → `fr`) und zuletzt Englisch.
- Für Platzhalter und Pluralformen ist i18next zuständig (`{{count}}` und so
  weiter).

### Was ehrlicherweise fehlt

- **Der Übersetzungsstand schwankt.** Englisch ist maßgeblich und immer
  vollständig; die übrigen fünf werden von Hand und mit einem Prüfskript auf
  gleichem Stand gehalten, doch eine brandneue Funktion erscheint womöglich
  zuerst auf Englisch und fällt bis zur Übersetzung darauf zurück.
- **Noch keine von rechts nach links geschriebenen Sprachen.** Arabisch,
  Hebräisch und Persisch gibt es nicht, und das Layout ist für die Laufrichtung
  von rechts nach links weder gebaut noch getestet - eine solche Sprache
  aufzunehmen ist echte Arbeit (logische CSS-Eigenschaften, Umgang mit `dir`)
  und nicht bloß eine weitere Übersetzungsdatei.
- **Die Übersetzungen sind nicht fachlich geprüft.** Sie stammen aus der
  Gemeinschaft und vom Maintainer und haben entsprechende Qualität.
  Korrekturen sind willkommen.
- **Die Formatierung folgt der Locale nur teilweise.** Datums- und
  Zahlenangaben verlassen sich auf `Intl` der Plattform beziehungsweise die
  Locale des Browsers und sind nicht auf jeder Ansicht einheitlich verdrahtet.

### Eine Sprache ergänzen oder ausbessern

1. Kopieren Sie `i18n/en/` nach `i18n/<your-locale>/` und übersetzen Sie die
   Werte im YAML (die Schlüssel bleiben stehen).
2. Tragen Sie die Locale in die Positivliste in
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) ein
   (`supportedLocales`).
3. Halten Sie den Satz der Schlüssel über alle Locales hinweg identisch - sonst
   schlägt die Gleichstandsprüfung fehl.

---

## Barrierefreiheit (a11y)

### Wo das Projekt steht

Evey ist mit semantischem HTML, ARIA-Beschriftungen und den
Barrierefreiheits-Eigenschaften von React Native gebaut, und einiges von dem,
was unten steht, ist bewusst so gemacht. Aber: Eine förmliche Prüfung gegen WCAG
hat es **nicht** gegeben, in der Testsuite steckt **keine** automatisierte
Prüfung der Barrierefreiheit, und mit Screenreadern ist die Anwendung **nicht**
planmäßig getestet worden. Teile davon lassen sich mit Hilfstechnologien
vermutlich bedienen, andere vermutlich nicht. Wenn Sie auf Hilfstechnologien
angewiesen sind, rechnen Sie mit rauen Kanten - und melden Sie uns bitte
Konkretes.

### Was da ist

- **Weniger Bewegung:** `prefers-reduced-motion: reduce` wird überall beachtet -
  Animationen und Übergänge sind faktisch abgeschaltet, sobald Ihr
  Betriebssystem darum bittet.
- **Sprache für Hilfstechnologien:** `<html lang>` folgt der aktiven Sprache der
  Oberfläche.
- **Tastatur (Player):** vollständige Steuerung über die Tastatur samt einer
  eingeblendeten Übersicht der Tastenkürzel (`?`); während Sie in ein Feld
  tippen, sind die Kürzel stillgelegt.
- **Beschriftete Bedienelemente:** Schaltflächen, die nur aus einem Symbol
  bestehen, und Schieberegler tragen im Web ein `aria-label` und auf
  Mobilgeräten `accessibilityLabel` beziehungsweise `accessibilityRole` (der
  größere Teil der Bedienelemente, nicht alle).
- **Designs:** fünf Themes sind eingebaut (dunkel als Standard, AMOLED,
  dark-red, light-default, light-blue); weitere kommen über
  Theme-Erweiterungen hinzu.
- **Android TV:** Fokussteuerung über das Steuerkreuz für die Navigation mit der
  Fernbedienung.

### Was ehrlicherweise noch aussteht

- **Kein Durchgang mit einem Screenreader.** Die Abläufe unter NVDA, VoiceOver
  und TalkBack sind ungeprüft; einige eigene Bedienelemente (Menüs, Dialoge, die
  Fortschrittsleiste, die Auswahl der Spuren) sagen ihren Zustand oder ihre Rolle
  womöglich nicht richtig an.
- **Die Fokusführung ist ungeprüft.** Die Darstellung des sichtbaren Fokus und
  die Fokusreihenfolge in Dialogen und Overlays sind stellenweise uneinheitlich.
- **Der Kontrast ist nur für die eingebauten Themes gemessen.** Ein Unit-Test
  (`frontend/packages/theme/src/contrast.test.ts`) hält jedes mitgelieferte
  Theme bei der Akzentfarbe, den drei Textabstufungen und den Gefahrenfarben gegen
  den Hintergrund und beide Oberflächen auf WCAG AA 4,5:1; die Anwendung für den
  Fernseher prüft ihre abgeleiteten Tokens noch einmal nach. Theme-Erweiterungen
  und Farben, die eine Erweiterung zur Laufzeit einschleust, werden nicht
  geprüft - ein eigenes Theme kann also stellenweise durchfallen. AAA ist nicht
  das Ziel.
- **Keine Textskalierung in der Anwendung** über den Zoom von Browser und
  Betriebssystem hinaus, und auch keinen eigenen Modus mit hohem Kontrast.
- **Die Gestaltung der Untertitel auf Mobilgeräten ist eingeschränkt** (eine
  Grenze des zugrundeliegenden Players `expo-video`), abgespielt werden die
  Untertitelspuren selbst aber sehr wohl.

### Ein Problem melden

Nennen Sie uns die Seite beziehungsweise die Ansicht, das konkrete
Bedienelement, was Sie erwartet haben und welche Hilfstechnologie in welcher
Version Sie eingesetzt haben. Konkrete Meldungen werden behoben, mit „das ist
nicht barrierefrei“ lässt sich nichts anfangen. Ausbesserungen an der
Barrierefreiheit erfolgen nach bestem Bemühen.
