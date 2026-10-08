---
source_sha: 53779971ebd8b5f41b11b97d89f9f1102e0d283c08d999b9cc028f3b92f630ca
reviewed: true
---
# Thèmes

Un thème change l'apparence de l'interface d'Evey : un paquet de
`kind: theme` sans processus côté serveur, fait d'une feuille de style et, en
option, d'un script confiné dans un cadre caché. Cette page s'adresse aux
auteurs de thèmes ; elle dit ce que les deux fichiers peuvent contenir et ce
que l'installateur réécrit ou refuse. Les thèmes s'installent comme les
plugins, depuis **Admin → Plugins**, et chaque utilisateur choisit le sien
dans **Paramètres → Apparence → Thème**.

## Manifeste

```yaml title="plugin.yaml"
name: aurora               # obligatoire ; nom lisible
version: 1.0.0             # obligatoire ; semver
kind: theme                # obligatoire

description: "..."         # facultatif
author: "..."              # facultatif

# ni language, ni entrypoint, ni permissions, ni capabilities : l'installation est refusée
```

Le ZIP contient `theme.css` et/ou `theme.js` à sa racine, à côté du
manifeste. Un paquet qui n'a ni l'un ni l'autre est accepté et n'a aucun
effet visible. `examples/plugins/theme-dark-blue` (un court `theme.css` qui
redéfinit les variables de couleur) et `examples/plugins/open-workshop` (une
refonte complète) passent tous deux l'assainisseur et sont de bons points de
départ.

## theme.css

La feuille de style est injectée comme `<link rel="stylesheet">` dans
l'en-tête de chaque page et s'applique à tout le document : propriétés
personnalisées, `@keyframes`, `@media`, `@font-face` avec des sources
`data:`, `@layer` et requêtes de conteneur fonctionnent. Elle est servie à
`/api/v1/themes/<id>/theme.css` sans authentification, de sorte qu'un thème
dont le navigateur se souvient s'applique aussi avant la connexion.

### Partir des propriétés personnalisées

L'interface tire ses surfaces, son texte, son accent, ses bordures et ses
rayons d'un jeu de propriétés personnalisées sur `:root` ;
`examples/plugins/theme-dark-blue/theme.css` les liste avec un commentaire
chacune. Les redéfinir restyle tous les écrans d'un coup :

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

### Points d'accroche stables

Quelques éléments cosmétiques portent des classes stables que vous pouvez
cibler sans dépendre des classes utilitaires, qui changent d'une version à
l'autre. C'est un contrat : elles ne seront pas renommées.

| Sélecteur | Élément | Ce qu'on peut restyler |
|---|---|---|
| `.evey-app-header` | la barre du haut | couleurs, hauteur, espacement, flou ; jamais masquer ni déplacer les contrôles qu'elle contient |
| `.evey-media-grid` | les grilles de cartes média (bibliothèque, rangées) | colonnes, `gap`, alignement |
| `.evey-media-card` | une carte média (affiche et informations) | largeur, espacement, survol |
| `.evey-poster` | le visuel d'affiche ou d'illustration (aussi sur la fiche) | `aspect-ratio`, `border-radius`, ombre, ajustement |
| `.evey-media-detail` | la racine de la fiche média | mise en page du corps de la fiche |

```css title="theme.css - affiches plus grandes, grille plus dense, coins plus nets"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Ce que l'assainisseur réécrit

À l'installation, `theme.css` passe par un assainisseur qui réécrit ou
retire ce qu'un thème n'a pas le droit de faire, consigne un avertissement
pour chaque changement, et réécrit le résultat : ce qui est servi est le
fichier assaini, qui peut différer de ce que vous avez écrit. Il ne refuse
jamais le paquet.

| Ce que vous avez écrit | Ce qui arrive | Pourquoi |
|---|---|---|
| `@import ...` | Retiré | Un thème se suffit à lui-même ; un import est une fuite réseau. |
| `url(http://...)`, `url(https://...)`, `url(//...)`, un candidat externe dans `image-set()`, une propriété personnalisée qui contient une telle URL | Remplacé par `none` ou neutralisé | Même raison. Les URI `data:` passent inchangées. |
| `url(javascript:...)`, `expression(...)`, `-moz-binding`, `behavior` | Retirés | Vecteurs d'exécution hérités. |
| Une règle dont le sélecteur correspond à `[type=password]`, `[type=hidden]`, `.admin-*`, `.token-*` ou `.secret-*` | Tout le bloc retiré | Un thème ne doit pas restyler les champs de mot de passe ni les éléments privilégiés. |
| Une règle avec `position: fixed` ou `sticky` qui couvre l'écran : `inset: 0`, les quatre côtés à `0`, ou au moins deux parmi `top`/`right`/`bottom`/`left: 0`, `width: 100vw`, `height: 100vh` dont une taille de fenêtre | Tout le bloc retiré | La surcouche d'hameçonnage classique. Un en-tête fixé par `top: 0; left: 0` et dimensionné par son contenu est conservé. Les déclarations d'un même sélecteur s'additionnent d'une règle à l'autre. |
| `pointer-events: none` dans une règle qui vise `button`, `[role=button]`, `a[href*=logout]` ou `[data-critical]` | Cette déclaration retirée, le reste conservé | Masquer la cliquabilité d'un contrôle critique est un détournement d'interface. |
| `z-index` au-dessus de 999 | Ramené à 999 | Un thème reste sous la couche modale de l'application. |

Les caractères échappés sont décodés avant ces vérifications : une séquence
d'échappement ne les contourne pas.

## theme.js

Le script est facultatif. Il est servi à `/api/v1/themes/<id>/theme.js` aux
utilisateurs connectés et s'exécute dans un `<iframe sandbox="allow-scripts">`
caché dont la politique fixe `connect-src 'none'` : aucun accès à la page
hôte, à son stockage ni au réseau, même pour un script qui a passé l'analyse
à l'installation. La seule surface est `window.evey.theme`, injectée avant
l'exécution du script.

### `evey.theme.css(declarations)`

Envoie des déclarations de propriétés personnalisées à la page hôte, qui les
applique sur `:root`. C'est la seule forme acceptée : une liste de
`--nom: valeur`, jusqu'à 200 déclarations, chaque valeur de 500 caractères au
plus, sans `url()`, `image-set`, `expression()`, `@import`, `javascript:`,
`data:`, `http(s):`, `//`, commentaire ni accolade. Tout le reste est
abandonné et signalé dans la console. Une propriété n'agit que là où la
feuille de style assainie y fait référence.

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

Transmet une ligne à la console du navigateur. `level` vaut `"debug"`,
`"info"`, `"warn"` ou `"error"` ; le message est coupé à 500 caractères.
Rien n'est montré à l'utilisateur.

`evey.theme.registerComponent` et `evey.theme.observe` existent sur l'objet
mais ne sont pas implémentés : l'hôte leur répond par une erreur.

### Ce que le scanner refuse

À l'installation, `theme.js` est analysé comme du texte, et une seule
correspondance refuse le paquet, avec la raison. Le scanner est
volontairement conservateur : un thème n'a aucune raison légitime de
mentionner ces noms, même dans un commentaire.

L'une de ces sous-chaînes, n'importe où dans la source :

`document.cookie`, `localStorage`, `sessionStorage`, `indexedDB`,
`XMLHttpRequest`, `navigator.sendBeacon`, `navigator.serviceWorker`,
`WebSocket`, `EventSource`, `importScripts`, `document.write`,
`Element.prototype`, `Object.defineProperty`, `globalThis`, `window.parent`,
`window.top`, `window.opener`, `Reflect.get`, `Reflect.apply`,
`Reflect.construct`, `window.frames`, `frameElement`, `postMessage`.

L'un de ces motifs :

| Motif | Ce qu'il attrape |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Appels directs de globales interdites |
| `\bimport\s*\(` | Import dynamique |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | Accès par crochets aux globales de l'hôte, comme `window["fetch"]` |
| `\.cookie\b` | Tout accès `.cookie` |
| `\.constructor\s*\.\s*constructor` | L'évasion `[].constructor.constructor` |
| `new\s+Function\b` | Construction de code |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | Lecture d'une globale interdite par propriété |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | Affectation sur une globale de l'hôte |
| `Object\s*\.\s*prototype` | Pollution de prototype |
| `\.register\s*\(\s*['"]\S+\.js` | Enregistrement d'un service worker |

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
