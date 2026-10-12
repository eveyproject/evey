---
source_sha: aacd60e164be25536f5b7ca3fe0a88655d153900e00d2521c8d0795dd2769d24
reviewed: true
---
# Sprachen und Barrierefreiheit

Wo Evey bei der Internationalisierung (i18n) und der Barrierefreiheit (a11y)
steht: was übersetzt ist und wie der Katalog zu jedem Client kommt, dann was
für Hilfstechnologien vorhanden ist und was nicht. Der Katalog liegt in
`i18n/<locale>/*.yaml` im Wurzelverzeichnis des Repositorys, wird von
`internal/pkg/i18n` ausgeliefert und vom i18n-Modul jeder App geladen. Dies
ist ein selbst gehostetes Projekt, das eine einzelne Person pflegt: Sehen Sie
die Barrierefreiheit besonders als Bemühen nach bestem Können, nicht als
zertifiziert.

## Internationalisierung (i18n)

### Was übersetzt ist und was nicht

| Von Evey übersetzt | Nicht von Evey übersetzt |
| --- | --- |
| die Oberfläche selbst: Schaltflächen, Menüs, Einstellungen, Fehlermeldungen, Texte der Push-Benachrichtigungen, Benachrichtigungs-E-Mails (in der Sprache des Empfängers) | Metadaten der Medien (Titel, Inhaltsangaben, Genres). Sie kommen vom Metadatenanbieter, in der **Metadatensprache**, die im Einrichtungsassistenten oder unter **Verwaltung → Serverkonfiguration → Sprache & Region** eingestellt ist (`EVEY_METADATA_LANGUAGE` ist nur der Standardwert) |
| | Ihre eigenen Bibliotheken, Dateinamen und alles, was Sie eintippen; es wird so gespeichert, wie es ist |

E-Mails zum Zurücksetzen des Passworts werden auf Englisch verschickt. Die
Sprache der Oberfläche und die Sprache der Metadaten sind zwei getrennte
Einstellungen: Die Oberfläche kann auf Französisch sein und die
Inhaltsangabe eines Films auf Englisch, oder umgekehrt.

### Unterstützte Sprachen der Oberfläche

Sechs: **Englisch (en)**, **Französisch (fr)**, **Deutsch (de)**,
**Spanisch (es)**, **Chinesisch (zh)**, **Hindi (hi)**. Englisch ist die
Ausgangssprache und der Rückfall: Fehlt ein Text in einer anderen Sprache,
erscheint er auf Englisch statt als leeres Feld oder als Schlüssel.

### Wie ein Text auf den Bildschirm kommt

- **Eine einzige Quelle:** [`/i18n/<locale>/*.yaml`](../i18n) im
  Wurzelverzeichnis des Repositorys, aufgeteilt in Namensräume (`common`,
  `auth`, `media`, `player`, `admin`, `errors`, `notifications`, `mobile`,
  `downloads`, `requests`). Derselbe Katalog speist jede Oberfläche.
- Der **Server** lädt ihn, liefert ihn unter `GET /api/v1/i18n/{locale}` aus
  und nutzt ihn direkt für übersetzte Fehlermeldungen der API und für die
  Texte der Benachrichtigungen.
- Die **Web-App** lädt die Übersetzungen von diesem Endpunkt (i18next mit dem
  HTTP-Backend), behält eine Kopie im lokalen Speicher des Browsers für den
  nächsten Start und hält `<html lang>` mit der aktiven Sprache synchron,
  damit Hilfstechnologien die richtige Sprache ansagen. Die **Mobil- und
  TV-Apps** laden vom selben Endpunkt über das gemeinsame native i18n-Modul
  und bringen eine eingebaute englische Grundausstattung mit
  (`fallback.en.json`, `offlineStrings.ts`) für die Bildschirme, die vor der
  ersten Antwort eines Servers erscheinen.
- Die Sprache kommt aus Ihrer Kontoeinstellung. Fehlt sie, gilt die Sprache
  des Browsers oder Geräts (`Accept-Language` wird ausgewertet, die Region
  entfernt: `fr-FR` wird zu `fr`), dann die **Serversprache** unter
  **Verwaltung → Serverkonfiguration → Sprache & Region** (die auch die
  Sprache neuer Konten festlegt), dann Englisch.
- Platzhalter und Pluralformen laufen über i18next (`{{count}}` usw.).

### Was noch fehlt

- **Die Vollständigkeit schwankt.** Englisch ist maßgeblich und immer
  vollständig. Die anderen fünf werden von Hand auf demselben Stand gehalten,
  mit einem Test (`internal/pkg/i18n/parity_test.go`), der fehlschlägt, wenn
  die Schlüssel auseinanderlaufen. Eine ganz neue Funktion kann aber zuerst
  auf Englisch kommen und bis zur Übersetzung auf Englisch zurückfallen.
- **Keine Sprachen mit Schreibrichtung von rechts nach links.** Es gibt kein
  Arabisch, Hebräisch oder Persisch, und das Layout ist nicht für RTL gebaut
  oder getestet: Eine solche Sprache hinzuzufügen ist echte Arbeit (logische
  CSS-Eigenschaften, Umgang mit `dir`), nicht nur eine Übersetzungsdatei.
- **Die Übersetzungen sind nicht professionell geprüft.** Sie haben die
  Qualität, die der Betreuer liefern kann. Korrekturen sind willkommen.
- **Formatierung nach Region nur teilweise.** Datum und Zahlen hängen an
  `Intl` der Plattform bzw. an der Spracheinstellung des Browsers und sind
  nicht auf jedem Bildschirm einheitlich angebunden.

### Eine Sprache hinzufügen oder korrigieren

1. Kopieren Sie `i18n/en/` nach `i18n/<your-locale>/` und übersetzen Sie die
   YAML-Werte (die Schlüssel bleiben).
2. Tragen Sie die Sprache in die Liste der erlaubten Sprachen in
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) ein
   (`supportedLocales`).
3. Halten Sie die Schlüssel in allen Sprachen identisch: Sonst schlägt der
   Paritätstest fehl, und `TestI18nPlaceholdersAgreeAcrossLocales` prüft, dass
   auch die `{{placeholders}}` jedes Textes übereinstimmen.

## Barrierefreiheit (a11y)

### Wo das steht

Evey ist mit semantischem HTML, ARIA-Beschriftungen und den
Barrierefreiheits-Eigenschaften von React Native gebaut, und einiges unten
ist bewusst so gemacht. Es wurde aber **nicht** formal nach WCAG geprüft, es
gibt **keine automatischen Barrierefreiheitstests** in der Testsuite, und es
wurde **nicht** systematisch mit Screenreadern getestet. Teile davon sind
mit Hilfstechnologien wahrscheinlich nutzbar, andere wahrscheinlich nicht.
Wenn Sie auf Hilfstechnologien angewiesen sind, rechnen Sie mit Ecken und
Kanten und melden Sie bitte Einzelheiten.

### Was vorhanden ist

| Bereich | Was umgesetzt ist |
| --- | --- |
| Weniger Bewegung | `prefers-reduced-motion: reduce` wird überall beachtet (`frontend/apps/web/src/styles/main.css`): Animationen und Übergänge sind praktisch abgeschaltet, wenn Ihr Betriebssystem das verlangt |
| Sprache für Hilfstechnologien | `<html lang>` folgt der aktiven Sprache der Oberfläche |
| Tastatur (Player) | volle Tastatursteuerung mit einer Übersicht der Tastenkürzel auf dem Bildschirm (`?`, `KeyboardShortcutsOverlay.tsx`); die Kürzel sind ausgeschaltet, solange Sie in ein Feld tippen |
| Beschriftete Bedienelemente | Schaltflächen nur mit Symbol und Schieberegler tragen im Web `aria-label` und auf dem Handy `accessibilityLabel`/`accessibilityRole` (der größere Teil der Bedienelemente, nicht alle) |
| Designs | fünf Designs sind dabei (Standard dunkel, AMOLED, dunkelrot, Standard hell, hellblau); weitere über Theme-Plugins |
| Android TV | Fokusführung mit dem Steuerkreuz für die Bedienung per Fernbedienung |

### Was noch fehlt

- **Kein Durchgang mit Screenreadern.** Abläufe mit NVDA, VoiceOver und
  TalkBack sind ungeprüft. Manche eigenen Bedienelemente (Menüs, Dialoge, die
  Zeitleiste, die Spurauswahl) sagen ihren Zustand oder ihre Rolle vielleicht
  nicht richtig an.
- **Die Fokusführung ist nicht geprüft.** Die Sichtbarkeit des Fokus und die
  Fokusreihenfolge in Dialogen und Überlagerungen sind stellenweise
  uneinheitlich.
- **Der Kontrast wird nur für die eingebauten Designs gemessen.** Ein
  Unit-Test (`frontend/packages/theme/src/contrast.test.ts`) verlangt von
  jedem mitgelieferten Design WCAG AA 4.5:1 für die Akzentfarbe, die drei
  Textabstufungen und die Warnfarben, gegen den Hintergrund und beide
  Flächen. Die TV-App prüft ihre abgeleiteten Werte noch einmal
  (`ThemeProvider.contrast.test.ts`). Theme-Plugins und die Farben, die ein
  Plugin zur Laufzeit einfügt, werden nicht geprüft: Ein eigenes Theme kann
  stellenweise durchfallen. AAA ist nicht das Ziel.
- **Keine Textvergrößerung in der App** über den Zoom von Browser oder
  Betriebssystem hinaus, und kein eigener Modus mit hohem Kontrast.
- **Die Gestaltung von Untertiteln auf dem Handy ist begrenzt** (eine
  Einschränkung des zugrunde liegenden Players `expo-video`). Untertitel und
  Untertitel für Hörgeschädigte selbst werden aber abgespielt.

### Ein Problem melden

Nennen Sie uns die Seite oder den Bildschirm, das genaue Bedienelement, was
Sie erwartet haben und welche Hilfstechnologie Sie in welcher Version
benutzt haben. Konkrete Meldungen werden behoben; mit „es ist nicht
barrierefrei“ lässt sich nichts anfangen. Korrekturen zur Barrierefreiheit
werden nach bestem Können bearbeitet.

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
