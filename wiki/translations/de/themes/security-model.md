---
source_sha: 2ec48afdaf5dd44ee661048d21aa84c7ac2b22e7b6454b4270a203bd473922b4
---
# Sicherheitsmodell für Themes

## Theme oder Erweiterung: der entscheidende Unterschied

Ein **Theme** lebt ausschließlich im Frontend. Es hat keinen Backend-Prozess,
keine Sprachlaufzeit, keinen IPC-Socket und keinerlei Berechtigungen auf dem
Server. Es besteht aus:

- `theme.css` - einem Stylesheet, das auf jeder Seite in den `<head>` eingefügt wird
- `theme.js` (optional) - JavaScript, das in einem abgeschotteten Kontext mit der eng gefassten API `evey.theme.*` ausgeführt wird

Eine **Erweiterung** hat immer einen Backend-Prozess (`kind: plugin`, dazu
`language` und `entrypoint` als Pflichtangaben). Ein Theme, das `language`,
`entrypoint` oder `permissions` angibt, wird bei der Installation abgewiesen.

Sowohl das CSS als auch das JavaScript werden bei der Installation statisch
analysiert, bevor das Theme angenommen wird.

## Regeln der CSS-Bereinigung

`theme.css` läuft bei der Installation durch `SanitizeCSS`. Die Bereinigung
schreibt gefährliche Konstrukte um oder entfernt sie und gibt eine Liste von
Warnungen zurück. Sie weist die Datei **nicht** als Ganzes ab - sauberes CSS
kommt unverändert hindurch.

### Blockierte Konstrukte

**`@import`** - wird vollständig entfernt.

Jede `@import`-Anweisung wird herausgeschnitten. Der Grund: Ein Theme muss für
sich stehen, und der Import eines externen Stylesheets ist ein Weg, Daten nach
außen abfließen zu lassen.

**Externe `url()`** - wird durch `none` ersetzt.

Werte in `url()`, die auf `http://`, `https://` oder ein protokollrelatives `//`
zeigen, werden ersetzt. `data:`-URIs (`data:image/*`, `data:font/*`) sind
erlaubt und kommen unverändert hindurch.

**`javascript:` in `url()`** - wird durch `none` ersetzt.

**`expression()`** (Altlast aus dem IE) - wird durch `none` ersetzt.

**`-moz-binding` und `behavior`** - werden entfernt.

Alte Wege, über XBL beziehungsweise HTC Code auszuführen; sie werden vollständig
entfernt.

**Regeln mit heiklen Selektoren** - der gesamte Regelblock wird entfernt.

Regelblöcke, deren Selektor auf eines der folgenden Muster passt, werden
restlos gestrichen:

- `[type="password"]`, `[type='password']` oder `[type=password]`
- `[type="hidden"]`
- `.admin-<beliebig>`
- `.token-<beliebig>`
- `.secret-<beliebig>`

Der Grund: Ein Theme darf Passwortfelder und privilegierte Bedienelemente nicht
so gestalten können, dass sich damit eine Oberfläche zu Täuschungszwecken
umbauen lässt (UI-Redressing).

**Regeln für bildschirmfüllende Overlays** - der gesamte Regelblock wird
entfernt.

Ein Regelblock fliegt heraus, wenn er `position: fixed` oder `position: sticky`
enthält und dazu mindestens zwei von `top: 0`, `left: 0`, `width: 100vw` und
`height: 100vh`. Genau so sieht die klassische Phishing-Überlagerung per CSS
aus.

**`pointer-events: none` auf Bedienelementen** - die Deklaration wird aus dem
Regelblock entfernt.

Zielt der Selektor einer Regel auf `button`, `[role="button"]`,
`a[href*=logout]` oder `[data-critical]`, wird jede Deklaration
`pointer-events: none` aus dieser Regel entfernt. Der Rest der Regel bleibt
stehen. Der Grund: Die Schaltfläche zum Abmelden oder eine andere kritische
Schaltfläche unanklickbar zu machen, ist ein Täuschungsangriff auf die
Oberfläche.

**`z-index` über 999** - wird auf 999 gedeckelt.

Jeder `z-index` größer als 999 wird auf 999 umgeschrieben und eine Warnung
vermerkt. Ein Theme darf sich nicht über die Ebene legen, auf der die Anwendung
ihre eigenen Dialoge und Overlays zeichnet.

### Was ein Theme in CSS DARF

- Den gesamten Aufbau der Startseite, den Hero-Bereich, das Kartenraster und die Seitenleiste umgestalten
- Die Bedienelemente des Videoplayers, die Fortschrittsleiste und die Overlays neu gestalten
- Eigene Schriften einbinden (über `data:` oder ein eingebettetes `@font-face` mit `src: url(data:...)`)
- Animationen und Übergänge (`@keyframes`, `transition`, `animation`)
- Eigene Eigenschaften (`--my-color: #...`) und Übersteuerungen auf `:root`
- Media-Queries und Container-Queries
- Jeden Selektor, der weder auf Passwortfelder noch auf `.admin-*`, `.token-*` oder `.secret-*` zielt

## Regeln des JavaScript-Scanners für Themes

`theme.js` wird bei der Installation von `internal/pkg/themejs/scan.go`
durchsucht. Ein einziger Treffer genügt, und die Installation wird mit einer
sprechenden Fehlermeldung abgewiesen. Warnungen gibt es nicht - der Scanner ist
mit Absicht vorsichtig (Fehlalarme sind hinnehmbar, denn ein Theme hat keinen
legitimen Grund, diese APIs überhaupt zu erwähnen).

### Blockierte Zeichenketten

Taucht eine der folgenden Zeichenfolgen irgendwo im Quelltext auf, wird die
Installation abgewiesen:

- `document.cookie`
- `localStorage`
- `sessionStorage`
- `indexedDB`
- `XMLHttpRequest`
- `navigator.sendBeacon`
- `navigator.serviceWorker`
- `WebSocket`
- `EventSource`
- `importScripts`
- `document.write`
- `Element.prototype`
- `Object.defineProperty`
- `globalThis`
- `window.parent`
- `window.top`
- `window.opener`
- `Reflect.get`
- `Reflect.apply`
- `Reflect.construct`
- `window.frames`
- `frameElement`
- `postMessage`

### Blockierte reguläre Ausdrücke

| Muster | Was er abfängt |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Unmittelbare Aufrufe verbotener globaler Objekte |
| `\bimport\s*\(` | Dynamischer Import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Zugriff auf globale Objekte des Hosts über eckige Klammern (fängt `window["fetch"]` ab) |
| `\.cookie\b` | jeder Zugriff auf `.cookie`, ob über einen Property-Deskriptor oder berechnet |
| `\.constructor\s*\.\s*constructor` | Der Ausbruch über `[].constructor.constructor` |
| `new\s+Function\b` | Code bauen über `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lesender Zugriff auf verbotene globale Objekte |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Zuweisung an eine Eigenschaft der globalen Objekte des Hosts |
| `Object\s*\.\s*prototype` | Prototype Pollution |
| `\.register\s*\(\s*['"]\S+\.js` | Registrierung eines Service Workers |

### Abschottung des Theme-JavaScripts zur Laufzeit

Zur Laufzeit läuft `theme.js` in einem iframe mit `sandbox="allow-scripts"` und
einem `srcdoc` unter verriegelter CSP. Selbst wenn jemand den Scanner bei der
Installation überlisten würde, käme das iframe zur Laufzeit weder
gleichursprünglich an das Host-Dokument noch an dessen `window`,
`localStorage` oder Cookies heran, und nach außen telefonieren kann es auch
nicht (CSP `connect-src 'none'`).

Der einzige Weg der Verständigung ist die API `evey.theme.*`, die die Brücke des
Hosts bereitstellt. Welche Methoden es gibt, steht in
[`api-reference.md`](api-reference.md).

## Was Sie in theme.js DÜRFEN

- `evey.theme.css(rules)` aufrufen, um CSS in die Stil-Schicht des Hosts einzufügen
- `evey.theme.log(level, msg)` aufrufen, um Debug-Meldungen in die Konsole des Browsers zu schreiben
- Render-Funktionen für Komponenten über `evey.theme.registerComponent(slot, render)` registrieren (V2 - siehe unten)
- Den Zustand von Player und Anwendung über `evey.theme.observe(stateKey, cb)` lesend beobachten (V2)

## Was Sie NICHT dürfen

- Daten von irgendeiner URL holen (`fetch`, `XMLHttpRequest`, `WebSocket`, `navigator.sendBeacon`)
- Zugangsdaten oder Tokens der Benutzer lesen (`document.cookie`, `localStorage`, `sessionStorage`)
- Aus dem Kontext des iframe ausbrechen (`window.parent`, `window.top`, `frameElement`)
- Ein Overlay zeichnen, das den gesamten sichtbaren Bereich verdeckt
- Mit CSS auf Passwortfelder oder auf `.admin-*`, `.token-*` und `.secret-*` zielen
- Die Schaltfläche zum Abmelden oder andere kritische Schaltflächen unanklickbar machen (`pointer-events: none`)
- Sich über die Dialoge der Anwendung legen (`z-index > 999`)
