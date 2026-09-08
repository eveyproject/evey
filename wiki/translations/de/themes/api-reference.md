---
source_sha: d7589893f88623d5f9ecd03d54b2f38ec845cd7eb4a550e31a8ffb93d93353cb
---
# Referenz der Theme-API

Dieses Dokument richtet sich an externe Entwickler, die ein Theme für Evey
schreiben. Welche Sicherheitsvorgaben für Themes durchgesetzt werden, steht in
[`security-model.md`](security-model.md).

## Was ein Theme ist

Ein Theme ist ein Paket mit `kind: theme`. Es hat **keinen Backend-Prozess**.
Enthalten sein darf:

- `theme.css` - wird nach der Bereinigung auf jeder Seite in den `<head>` eingefügt
- `theme.js` - optionales JavaScript, das in einem abgeschotteten Kontext mit der eng gefassten API `evey.theme.*` läuft

Ein Theme darf weder `language` noch `entrypoint` oder `permissions` angeben.
Der Versuch führt dazu, dass die Installation abgewiesen wird.

## Schema des Manifests

```yaml
name: aurora               # Pflicht; sprechender Name
version: 1.0.0             # Pflicht; semver
kind: theme                # Pflicht; muss "theme" sein

description: "..."         # optional
author: "..."              # optional
min_evey_version: "1.0.0"  # optional

# KEIN language, entrypoint, permissions - für Themes sind diese Felder unzulässig
```

Im Wurzelverzeichnis des ZIP muss `theme.css`, `theme.js` oder beides liegen.
Ein Paket ohne beides wird zwar angenommen, das Theme bleibt dann aber ohne
sichtbare Wirkung.

## theme.css

Wird unter `/api/v1/themes/<id>/theme.css` ausgeliefert. Bei der Installation
läuft die Datei durch die CSS-Bereinigung (welche Konstrukte dabei blockiert
werden, steht in [`security-model.md`](security-model.md)). Gespeichert und
ausgeliefert wird das bereinigte Ergebnis - was Sie schreiben und was
tatsächlich eingefügt wird, kann also auseinandergehen, sobald eine Regel
gegriffen hat.

Eingefügt wird das Stylesheet als `<link rel="stylesheet">` im `<head>`. Es
wirkt im gesamten Host-Dokument. Es gilt gewöhnliches CSS: eigene
Eigenschaften, `@keyframes`, `@media`, `@font-face` (mit Quellen als `data:`),
`@layer` und Container-Queries.

### Stabile Ansatzpunkte für Themes (der Selektor-Vertrag)

Evey kennzeichnet eine kleine Auswahl rein kosmetischer, unkritischer Elemente
mit **stabilen Klassen**, auf die Sie zielen können, ohne sich auf die
(schwankenden) Utility-Klassen zu verlassen. Diese Klassen sind eine Zusage: Bei
Umbauten werden sie nicht umbenannt.

| Selektor | Element | Was gefahrlos umgestaltet werden darf |
|---|---|---|
| `.evey-app-header` | die Kopfleiste | Farben, Höhe, Abstände, Weichzeichnung - die Bedienelemente darin **niemals** ausblenden oder verschieben |
| `.evey-media-grid` | die Raster der Medienkarten (Bibliothek, Reihen) | Spalten, `gap`, Ausrichtung |
| `.evey-media-card` | eine einzelne Medienkarte (Poster und Angaben) | Breite, Abstände, Verhalten beim Überfahren |
| `.evey-poster` | das Poster beziehungsweise Artwork (auch auf der Detailseite) | `aspect-ratio`, `border-radius`, Schatten, Skalierung |
| `.evey-media-detail` | die Wurzel der Detailseite eines Mediums | Aufbau des Detailbereichs |

Was die Bereinigung verbietet, bleibt auch hier verboten (kein `@import`, keine
bildschirmfüllenden Overlays, kein `pointer-events:none` auf Bedienelementen,
kein `[type=password]`, `.admin-*`, `.token-*` oder `.secret-*`, `z-index`
gedeckelt). Ein Theme darf diese Ansatzpunkte frei umgestalten; es darf die
Kopfleiste aber **nicht** dazu zweckentfremden, die vertrauenswürdigen
Bedienelemente (Abmelden, Profil, echte Benachrichtigungen) zu verbergen oder
nachzuahmen.

### Beispiel: größere Poster, dichteres Raster, schärfere Ecken

```css
/* Größere Poster auf einem engeren Raster */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Beispiel: eigenes Theme für den Player

```css
.player-controls {
  background: linear-gradient(to top, rgba(0,0,0,.8), transparent);
  border-radius: 0 0 8px 8px;
}

.player-progress-bar {
  accent-color: var(--color-accent);
  height: 4px;
}
```

## theme.js

Wird unter `/api/v1/themes/<id>/theme.js` ausgeliefert (Anmeldung
erforderlich). Geladen wird das Skript in einem iframe mit
`sandbox="allow-scripts"` und einer verriegelten Content-Security-Policy - es
kommt nicht an die Host-Seite heran, kann keine Netzanfragen stellen und nichts
aus dem Speicher lesen.

Die einzige verfügbare Schnittstelle ist `window.evey.theme`; die Brücke des
Hosts stellt sie bereit, bevor `theme.js` läuft.

### `evey.theme.css(rules)`

Fügt zur Laufzeit eine CSS-Zeichenkette in die Stil-Schicht des Hosts ein.
Gedacht für dynamische Gestaltung, die von berechneten Werten oder von
Interaktionen innerhalb des Themes abhängt.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Schreibt eine Protokollmeldung in die Konsole des Browsers, mit dem Präfix
`[evey-theme]`. Endbenutzer bekommen sie nicht zu Gesicht.

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: eine Zeichenkette aus reinem Text

```js
evey.theme.log("info", "Aurora theme loaded.");
```

## V2-API (Roadmap)

Die folgenden Methoden sind geplant und in V1 noch nicht verfügbar:

### `evey.theme.registerComponent(slot, render)` (V2)

Registriert eine Render-Funktion für einen benannten Slot. Der Host ruft
`render(props, container)` auf, sobald der Slot eingehängt wird. `container` ist
ein DOM-Knoten im Shadow-Kontext des Themes - was Sie in `container` schreiben,
erscheint innerhalb einer ShadowRoot auf der Host-Seite.

```js
// V2 - noch nicht verfügbar
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)` (V2)

Abonniert Änderungen am Zustand des Hosts, nur lesend. Die Rückruffunktion wird
immer dann aufgerufen, wenn sich der beobachtete Schlüssel ändert.

```js
// V2 - noch nicht verfügbar
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)` (V2)

Wendet eine benannte Animation auf ein Element des Hosts an, das über
`selector` gefunden wird - vermittelt über die Brücke, ohne unmittelbaren
Zugriff auf das DOM.

## Beispiele

Das Verzeichnis `examples/themes/` gibt es noch nicht. Die Referenzbeispiele
(`dark-cozy`, `aurora`, `minimal-player`) sind für Phase 8.5 des Umbaus von
Themes und Erweiterungen vorgesehen. Nehmen Sie bis dahin die `theme.css` einer
beliebigen bestehenden Erweiterung mit `language: css` aus der Datenbank als
Ausgangspunkt - diese Dateien liegen im selben Format vor, mit dem die
Bereinigung umgehen kann.
