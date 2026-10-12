---
source_sha: 53779971ebd8b5f41b11b97d89f9f1102e0d283c08d999b9cc028f3b92f630ca
reviewed: true
---
# Themes

Ein Theme gestaltet die Oberfläche von Evey neu: ein Paket mit `kind: theme`
ohne eigenen Serverprozess, bestehend aus einem Stylesheet und optional einem
Skript, das in einem unsichtbaren Frame eingesperrt bleibt. Diese Seite
richtet sich an Theme-Autoren. Sie sagt, was die beiden Dateien enthalten
dürfen und was der Installer umschreibt oder ablehnt. Themes werden wie
Plugins installiert, unter **Verwaltung → Plugins**. Jeder Benutzer wählt
eines unter **Einstellungen → Darstellung → Design**.

## Manifest

```yaml title="plugin.yaml"
name: aurora               # Pflicht; lesbarer Name
version: 1.0.0             # Pflicht; semver
kind: theme                # Pflicht

description: "..."         # optional
author: "..."              # optional

# kein language, entrypoint, permissions oder capabilities: sonst wird die Installation abgelehnt
```

Das ZIP enthält `theme.css` und/oder `theme.js` auf oberster Ebene, neben
dem Manifest. Ein Paket ohne beide wird angenommen und bewirkt sichtbar
nichts. `examples/plugins/theme-dark-blue` (eine kurze `theme.css`, die die
Farbvariablen überschreibt) und `examples/plugins/open-workshop` (eine
vollständige Neugestaltung) bestehen beide den Sanitizer und sind gute
Ausgangspunkte.

## theme.css

Das Stylesheet wird als `<link rel="stylesheet">` in den Kopf jeder Seite
eingefügt und gilt für das ganze Dokument: Custom Properties, `@keyframes`,
`@media`, `@font-face` mit `data:`-Quellen, `@layer` und Container Queries
funktionieren alle. Es wird ohne Authentifizierung unter
`/api/v1/themes/<id>/theme.css` ausgeliefert. Ein Theme, das sich der
Browser merkt, gilt also schon vor der Anmeldung.

### Mit den Custom Properties beginnen

Die Oberfläche nimmt ihre Flächen, Texte, Akzentfarbe, Rahmen und Radien aus
einer Reihe von Custom Properties auf `:root`.
`examples/plugins/theme-dark-blue/theme.css` listet sie auf, jede mit einem
Kommentar. Wer sie überschreibt, gestaltet jeden Bildschirm auf einmal um:

```css title="theme.css"
:root {
  --bg: #0b1120;
  --surface: #162032;
  --text: #e2e8f0;
  --accent: #3b82f6;
  --accent-hover: #60a5fa;
  --radius: 8px;
}
```

### Stabile Ansatzpunkte für Themes

Einige gestalterische Elemente tragen stabile Klassen. Sie können sie
ansprechen, ohne von den Utility-Klassen abzuhängen, die sich von Version zu
Version ändern. Diese Klassen sind ein Vertrag: Sie werden nicht umbenannt.

| Selektor | Element | Gefahrlos änderbar |
|---|---|---|
| `.evey-app-header` | die obere Leiste | Farben, Höhe, Abstände, Unschärfe; die Bedienelemente darin nie ausblenden oder verschieben |
| `.evey-media-grid` | die Raster der Medienkarten (Bibliothek, Reihen) | Spalten, `gap`, Ausrichtung |
| `.evey-media-card` | eine Medienkarte (Poster und Angaben) | Breite, Abstände, Hover |
| `.evey-poster` | das Poster oder Artwork (auch auf der Detailseite) | `aspect-ratio`, `border-radius`, Schatten, Einpassung |
| `.evey-media-detail` | das Wurzelelement der Detailseite eines Mediums | Layout des Detailbereichs |

```css title="theme.css - größere Poster, dichteres Raster, schärfere Ecken"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Was der Sanitizer umschreibt

Bei der Installation durchläuft `theme.css` einen Sanitizer. Er schreibt um
oder entfernt, was ein Theme nicht darf, hält für jede Änderung eine Warnung
fest und speichert das Ergebnis zurück: Ausgeliefert wird die bereinigte
Datei, und die kann von dem abweichen, was Sie geschrieben haben. Das Paket
lehnt er nie ab.

| Was Sie geschrieben haben | Was passiert | Warum |
|---|---|---|
| `@import ...` | Entfernt | Ein Theme ist in sich geschlossen; ein Import ist ein Datenleck ins Netz. |
| `url(http://...)`, `url(https://...)`, `url(//...)`, ein externer Kandidat in `image-set()`, eine Custom Property mit einer solchen URL | Durch `none` ersetzt oder unschädlich gemacht | Derselbe Grund. `data:`-URIs bleiben unverändert. |
| `url(javascript:...)`, `expression(...)`, `-moz-binding`, `behavior` | Entfernt | Alte Wege, Code auszuführen. |
| Eine Regel, deren Selektor `[type=password]`, `[type=hidden]`, `.admin-*`, `.token-*` oder `.secret-*` trifft | Der ganze Regelblock entfernt | Ein Theme darf Passwortfelder und privilegierte Elemente nicht umgestalten. |
| Eine Regel mit `position: fixed` oder `sticky`, die den Bildschirm bedeckt: `inset: 0`, die vier Seiten auf `0` oder mindestens zwei von `top`/`right`/`bottom`/`left: 0`, `width: 100vw`, `height: 100vh`, darunter eine Größe in Viewport-Einheiten | Der ganze Regelblock entfernt | Die klassische Phishing-Überlagerung. Eine Kopfzeile, die mit `top: 0; left: 0` angeheftet ist und sich nach ihrem Inhalt richtet, bleibt erhalten. Deklarationen für denselben Selektor werden über alle Regeln zusammengezählt. |
| `pointer-events: none` in einer Regel, die `button`, `[role=button]`, `a[href*=logout]` oder `[data-critical]` anspricht | Diese Deklaration entfernt, der Rest bleibt | Ein kritisches Bedienelement unklickbar zu machen, ist eine Täuschung der Oberfläche (UI Redressing). |
| `z-index` über 999 | Auf 999 begrenzt | Ein Theme bleibt unter der Ebene der Dialoge der Anwendung. |

Maskierte Zeichen werden vor diesen Prüfungen entschlüsselt. Eine
Escape-Sequenz umgeht sie also nicht.

## theme.js

Ein Skript ist optional. Es wird angemeldeten Benutzern unter
`/api/v1/themes/<id>/theme.js` ausgeliefert und läuft in einem unsichtbaren
`<iframe sandbox="allow-scripts">`, dessen Richtlinie `connect-src 'none'`
setzt: kein Zugriff auf die Hostseite, ihren Speicher oder das Netz, auch
nicht für ein Skript, das die Prüfung bei der Installation bestanden hat. Die
einzige Schnittstelle ist `window.evey.theme`, das vor dem Skript eingefügt
wird.

### `evey.theme.css(declarations)`

Schickt Deklarationen von Custom Properties an die Hostseite, die sie auf
`:root` anwendet. Nur diese Form wird angenommen: eine Liste `--name: value`,
höchstens 200 Deklarationen, jeder Wert höchstens 500 Zeichen, ohne `url()`,
`image-set`, `expression()`, `@import`, `javascript:`, `data:`, `http(s):`,
`//`, Kommentar oder geschweifte Klammer. Alles andere wird verworfen und in
der Konsole gemeldet. Eine Property wirkt nur dort, wo das bereinigte
Stylesheet sie verwendet.

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

Gibt eine Zeile an die Browserkonsole weiter. `level` ist `"debug"`,
`"info"`, `"warn"` oder `"error"`; die Nachricht wird nach 500 Zeichen
abgeschnitten. Dem Benutzer wird nichts angezeigt.

`evey.theme.registerComponent` und `evey.theme.observe` gibt es auf dem
Objekt, sie sind aber nicht umgesetzt: Der Host beantwortet sie mit einem
Fehler.

### Was der Scanner ablehnt

Bei der Installation wird `theme.js` als Text geprüft. Ein einziger Treffer
lehnt das Paket ab, mit Angabe des Grundes. Der Scanner ist absichtlich
streng: Ein Theme hat keinen legitimen Grund, diese Namen zu erwähnen, nicht
einmal in einem Kommentar.

Jede dieser Zeichenfolgen, egal wo im Quelltext:

`document.cookie`, `localStorage`, `sessionStorage`, `indexedDB`,
`XMLHttpRequest`, `navigator.sendBeacon`, `navigator.serviceWorker`,
`WebSocket`, `EventSource`, `importScripts`, `document.write`,
`Element.prototype`, `Object.defineProperty`, `globalThis`, `window.parent`,
`window.top`, `window.opener`, `Reflect.get`, `Reflect.apply`,
`Reflect.construct`, `window.frames`, `frameElement`, `postMessage`.

Jedes dieser Muster:

| Muster | Was es erkennt |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Direkte Aufrufe verbotener globaler Funktionen |
| `\bimport\s*\(` | Dynamischer Import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Zugriff mit eckigen Klammern auf globale Objekte des Hosts, etwa `window["fetch"]` |
| `\.cookie\b` | Jeder Zugriff auf `.cookie` |
| `\.constructor\s*\.\s*constructor` | Der Ausbruch über `[].constructor.constructor` |
| `new\s+Function\b` | Code zur Laufzeit erzeugen |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lesen einer verbotenen globalen Eigenschaft |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Zuweisung an ein globales Objekt des Hosts |
| `Object\s*\.\s*prototype` | Prototype Pollution |
| `\.register\s*\(\s*['"]\S+\.js` | Registrierung eines Service Workers |

<!-- sources
manifest rules: internal/adapters/driven/pluginrunner/manifest.go - validateManifest (KindTheme)
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme ; GET /api/v1/plugins/themes
routes, auth: internal/adapters/driving/http/theme_asset_handler.go (theme.css open, theme.js authMw)
link injection, hidden iframe, connect-src none: frontend/apps/web/src/features/themes/ThemeLoader.tsx
sanitizer: internal/adapters/driven/pluginrunner/css_sanitizer.go - SanitizeCSS, reImport, reURLExternal, reURLExternalUnterminated, reImageSetCandidateExternal, reCustomPropExternalURL, reURLJavascript, reExpression, reMozBinding, reBehavior, reSensitiveSelector, overlaySignals.isOverlay, stripPointerEventsOnInteractive, reZIndex ; written back at install: internal/app/plugin/install.go - validateStagedPlugin
custom properties: examples/plugins/theme-dark-blue/theme.css
stable hooks: frontend/apps/web/src (evey-app-header, evey-media-grid, evey-media-card, evey-poster, evey-media-detail)
theme.js bridge: frontend/apps/web/src/features/themes/theme-bridge.ts - THEME_BRIDGE_SCRIPT ; policy: theme-css-policy.ts - applyThemeCSSPolicy, MAX_DECLARATIONS, MAX_VALUE_LENGTH, FORBIDDEN_IN_VALUE
scanner: internal/pkg/themejs/scan.go - blockedLiterals, blockedRegex, Scan
-->
