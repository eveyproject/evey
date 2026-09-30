---
source_sha: 305f13e3ff6f705b51384ec1d553349c48887a6ee0bd367b61e102a000be8911
reviewed: false
---
# Themes

Ein Theme gestaltet die Oberfläche von Evey neu. Es ist ein Paket vom `kind: theme` **ohne
Backend-Prozess** und enthält:

- `theme.css`, ein Stylesheet, das nach der Bereinigung in jede Seite eingefügt wird;
- optional `theme.js`, ein Skript, das in einem isolierten Rahmen mit einer kleinen
  API `evey.theme` läuft.

Ein Theme darf `language`, `entrypoint` oder `permissions` nicht angeben: Sonst
wird die Installation abgelehnt. Beide Dateien werden bei der Installation
geprüft. Themes werden wie Plugins installiert, unter **Admin → Plugins**.

## Manifest-Schema

```yaml
name: aurora               # erforderlich; lesbarer Name
version: 1.0.0             # erforderlich; semver
kind: theme                # erforderlich; muss "theme" sein

description: "..."         # optional
author: "..."              # optional
min_evey_version: "1.0.0"  # optional

# KEIN language, entrypoint, permissions - für Themes ungültig
```

Das ZIP muss `theme.css` und/oder `theme.js` in seinem Wurzelverzeichnis enthalten. Ein Paket ohne beide wird angenommen, das Theme hat dann aber keine sichtbare Wirkung.

## theme.css

Ausgeliefert unter `/api/v1/themes/<id>/theme.css`. Die Datei durchläuft bei der Installation den CSS-Bereiniger. Gespeichert und ausgeliefert wird die bereinigte Ausgabe - was Sie schreiben und was eingefügt wird, kann sich unterscheiden, wenn Regeln gegriffen haben.

Das Stylesheet wird als `<link rel="stylesheet">` in `<head>` eingefügt. Es gilt global für das Host-Dokument. Normales CSS gilt: benutzerdefinierte Eigenschaften, `@keyframes`, `@media`, `@font-face` (mit `data:`-Quellen), `@layer`, Container-Queries.

Der Bereiniger schreibt um oder entfernt, was ein Theme nicht darf, und
protokolliert für jede Änderung eine Warnung; er lehnt die Datei nicht ab.
Gespeichert und ausgeliefert wird die bereinigte Ausgabe, sie kann also von dem
abweichen, was Sie geschrieben haben.

### Was der Bereiniger entfernt

**`@import`** - vollständig entfernt.

Jede Direktive `@import` wird entfernt. Begründung: Themes müssen in sich geschlossen sein; das Importieren externer Stylesheets ist ein Weg für Netzwerklecks.

**Externes `url()`** - durch `none` ersetzt.

Werte `url()`, die auf `http://`, `https://` oder protokollrelative URLs `//` zeigen, werden ersetzt. URIs `data:` (`data:image/*`, `data:font/*`) sind erlaubt und bleiben unverändert.

**`javascript:` in `url()`** - durch `none` ersetzt.

**`expression()`** (Altlast aus dem IE) - durch `none` ersetzt.

**`-moz-binding` und `behavior`** - entfernt.

Alte Ausführungswege über XBL/HTC, vollständig entfernt.

**Regeln mit sensiblen Selektoren** - der ganze Regelblock wird entfernt.

Regelblöcke, deren Selektor auf einen der folgenden passt, werden vollständig entfernt:

- `[type="password"]` oder `[type='password']` oder `[type=password]`
- `[type="hidden"]`
- `.admin-<anything>`
- `.token-<anything>`
- `.secret-<anything>`

Begründung: Themes dürfen Passwortfelder oder privilegierte Elemente der Oberfläche nicht so gestalten können, dass sie zum Verfälschen der Oberfläche (UI-Redressing) taugen.

**Regeln für Vollbild-Überlagerungen** - der ganze Regelblock wird entfernt.

Ein Regelblock wird entfernt, wenn er `position: fixed` oder `position: sticky` mit einer Box verbindet, die den Bildschirm bedeckt: `inset: 0`, die vier Seiten auf `0`, oder mindestens zwei von `top: 0`, `right: 0`, `bottom: 0`, `left: 0`, `width: 100vw`, `height: 100vh`, darunter eine der beiden Bildschirmgrößen. Eine mit `top: 0; left: 0` fixierte, durch ihren Inhalt bemessene Kopfzeile bleibt erhalten. Die Deklarationen eines Selektors werden über Regeln hinweg addiert, das Aufteilen des Musters auf mehrere Blöcke entgeht ihm also nicht. Dieses Muster ist die klassische CSS-Überlagerung für Phishing.

**`pointer-events: none` auf interaktiven Elementen** - die Deklaration wird aus dem Regelblock entfernt.

Zielt der Selektor einer Regel auf `button`, `[role="button"]`, `a[href*=logout]` oder `[data-critical]`, wird jede Deklaration `pointer-events: none` dieser Regel entfernt. Der Rest der Regel bleibt. Begründung: Die Klickbarkeit von Abmelde- oder kritischen Schaltflächen zu verstecken, ist ein Angriff durch Verfälschen der Oberfläche.

**z-index über 999** - auf 999 begrenzt.

Jeder Wert `z-index` über 999 wird auf 999 umgeschrieben. Eine Warnung wird protokolliert. Themes dürfen sich nicht über die eigene Modal- und Überlagerungsebene der Anwendung legen.

### Was ein Theme in CSS kann

- Den ganzen Aufbau der Startseite, Bannerbereich, Kartenraster, Seitenleiste anpassen
- Bedienelemente, Fortschrittsleiste und Überlagerungen des Videoplayers neu gestalten
- Eigene Schriften (über `data:` oder inline `@font-face` mit `src: url(data:...)`)
- Animationen und Übergänge (`@keyframes`, `transition`, `animation`)
- Benutzerdefinierte Eigenschaften (`--my-color: #...`), Übersteuerungen in `:root`
- Media-Queries, Container-Queries
- Jeder Selektor, der nicht auf Passwortfelder, `.admin-*`, `.token-*`, `.secret-*` zielt

### Stabile Ansatzpunkte (Selektorvertrag)

Evey kennzeichnet eine kleine Auswahl kosmetischer, nicht sensibler Elemente mit
**stabilen Klassen**, die Sie ansprechen können, ohne von den (flüchtigen)
Hilfsklassen abzuhängen. Sie sind ein Vertrag: Bei Umbauten werden sie nicht
umbenannt.

| Selektor | Element | Gefahrlos neu zu gestalten |
|---|---|---|
| `.evey-app-header` | die obere Leiste | Farben, Höhe, Abstände, Unschärfe - die Bedienelemente darin **nie** verstecken oder verschieben |
| `.evey-media-grid` | die Kartenraster der Medien (Bibliothek, Reihen) | Spalten, `gap`, Ausrichtung |
| `.evey-media-card` | eine Medienkarte (Poster + Angaben) | Breite, Abstände, Hover |
| `.evey-poster` | das Poster- oder Bildmotiv (auch auf der Detailseite) | `aspect-ratio`, `border-radius`, Schatten, Einpassung |
| `.evey-media-detail` | die Wurzel der Detailseite eines Mediums | Aufbau des Detailbereichs |

Alles, was der Bereiniger verbietet, gilt weiterhin (kein `@import`, keine
Vollbild-Überlagerungen, kein `pointer-events:none` auf Bedienelementen, kein
`[type=password]` / `.admin-*` / `.token-*` / `.secret-*`, `z-index` begrenzt).
Ein Theme darf diese Ansatzpunkte frei gestalten; es darf die Kopfzeile **nicht**
umfunktionieren, um die vertrauenswürdigen Bedienelemente (Abmelden, Profil,
echte Benachrichtigungen) zu verstecken oder nachzuahmen.

### Beispiel: größere Poster, dichteres Raster, schärfere Ecken

```css
/* Größere Poster in einem engeren Raster */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Beispiel: eine andere Akzentfarbe

Die Oberfläche zeichnet Schaltflächen, Links, aktive Zustände und die
Fortschrittsleiste des Players aus einigen benutzerdefinierten Eigenschaften;
`examples/plugins/theme-dark-blue/theme.css` listet sie auf. Sie zu überschreiben
gestaltet jeden Bildschirm auf einmal neu:

```css
:root {
  --accent: #e8a33d;
  --accent-hover: #f0b95c;
}
```

## theme.js

Ausgeliefert unter `/api/v1/themes/<id>/theme.js` (Anmeldung erforderlich). In einem iframe mit `sandbox="allow-scripts"` und einer verriegelten Content Security Policy geladen - das Skript hat keinen Zugriff auf die Host-Seite, kann keine Netzwerkanfragen stellen und keinen Speicher lesen.

Die einzige verfügbare Schnittstelle ist `window.evey.theme`, von der Host-Brücke eingefügt, bevor `theme.js` läuft.

Selbst ein Skript, das den Installationsscanner passiert hat, erreicht weder die
Host-Seite noch ihren Speicher noch das Netzwerk: Der Rahmen hat keinen Zugriff
auf denselben Ursprung, und seine Richtlinie setzt `connect-src 'none'`.

### `evey.theme.css(rules)`

Fügt zur Laufzeit eine CSS-Zeichenkette in die Stilfläche des Hosts ein. Für dynamische Gestaltung, die von berechneten Werten oder Interaktionen im Theme abhängt.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Gibt eine Log-Meldung in der Browserkonsole aus, mit dem Präfix `[evey-theme]`. Erscheint nicht für Endbenutzer.

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: reiner Text

```js
evey.theme.log("info", "Aurora theme loaded.");
```

### Was der Scanner verweigert

Ein einziger Treffer lehnt die Installation ab, mit dem Grund. Der Scanner ist
bewusst vorsichtig: Ein Theme hat keinen berechtigten Grund, diese APIs zu nutzen.

#### Wörtliche Zeichenketten

Erscheint eine der folgenden Teilzeichenketten irgendwo im Quelltext, wird die Installation abgelehnt:

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

#### Muster

| Muster | Was es fängt |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Direkte Aufrufe verbotener Globale |
| `\bimport\s*\(` | Dynamischer Import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Zugriff in Klammernotation auf Host-Globale (fängt `window["fetch"]`) |
| `\.cookie\b` | jeder Zugriff auf `.cookie`, per Eigenschaftsdeskriptor oder berechnet |
| `\.constructor\s*\.\s*constructor` | Ausbruch über `[].constructor.constructor` |
| `new\s+Function\b` | Codeerzeugung mit `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lesen verbotener Globale als Eigenschaft |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Zuweisung an Eigenschaften von Host-Globalen |
| `Object\s*\.\s*prototype` | Prototype Pollution |
| `\.register\s*\(\s*['"]\S+\.js` | Registrierung eines Service Workers |

## Beispiele

Zwei Themes werden mit den Plugin-Beispielen ausgeliefert: `examples/plugins/theme-dark-blue` (eine kurze `theme.css`, die nur Farbvariablen überschreibt) und `examples/plugins/open-workshop` (eine vollständige Neugestaltung). Nehmen Sie eines davon als Ausgangspunkt - beide bestehen den Bereiniger bereits.
