---
source_sha: 6095a8b2b48d1c8ec2613e48fa7ace4603ea6bf231dc4c39b598eb130cb452bc
reviewed: true
---
# Thèmes

Un thème change l'apparence de l'interface d'Evey. C'est un paquet de
`kind: theme` **sans processus côté serveur**, qui contient :

- `theme.css`, une feuille de style injectée dans chaque page après
  assainissement ;
- en option, `theme.js`, un script exécuté dans un cadre isolé avec une petite
  API `evey.theme`.

Un thème ne peut pas déclarer `language`, `entrypoint` ni `permissions` :
l'installation est refusée s'il le fait. Les deux fichiers sont vérifiés à
l'installation. Les thèmes s'installent comme les plugins, depuis
**Admin → Plugins**.

## Schéma du manifeste

```yaml
name: aurora               # obligatoire ; nom lisible
version: 1.0.0             # obligatoire ; semver
kind: theme                # obligatoire ; doit valoir "theme"

description: "..."         # facultatif
author: "..."              # facultatif
min_evey_version: "1.0.0"  # facultatif

# PAS de language, entrypoint ni permissions : ils sont invalides pour un thème
```

Le ZIP doit contenir `theme.css` et/ou `theme.js` à sa racine. Un paquet qui
n'a ni l'un ni l'autre est accepté, mais le thème n'a aucun effet visible.

## theme.css

Servi à `/api/v1/themes/<id>/theme.css`. Le fichier passe par l'assainisseur CSS
à l'installation.

La feuille de style est injectée par un `<link rel="stylesheet">` dans le
`<head>`. Elle s'applique à tout le document hôte. Le CSS standard fonctionne :
propriétés personnalisées, `@keyframes`, `@media`, `@font-face` (avec des sources
`data:`), `@layer`, requêtes de conteneur.

L'assainisseur réécrit ou retire ce qu'un thème n'a pas le droit de faire et
enregistre un avertissement pour chaque modification ; il ne refuse pas le
fichier. Ce qui est stocké et servi est le résultat assaini, qui peut donc
différer de ce que vous avez écrit.

### Ce que l'assainisseur retire

**`@import`** : retiré entièrement.

Toute directive `@import` est supprimée. Raison : un thème doit être autonome ;
importer une feuille de style externe est un moyen de faire fuir des
informations sur le réseau.

**`url()` externe** : remplacé par `none`.

Les valeurs `url()` qui pointent vers des adresses `http://`, `https://` ou
relatives au protocole `//` sont remplacées. Les URI `data:` (`data:image/*`,
`data:font/*`) sont autorisées et passent telles quelles.

**`javascript:` dans `url()`** : remplacé par `none`.

**`expression()`** (héritage d'Internet Explorer) : remplacé par `none`.

**`-moz-binding` et `behavior`** : retirés.

D'anciens moyens d'exécution XBL/HTC, retirés entièrement.

**Règles sur des sélecteurs sensibles** : le bloc de règles entier est retiré.

Les blocs dont le sélecteur correspond à l'un des cas suivants sont supprimés en
entier :

- `[type="password"]`, `[type='password']` ou `[type=password]`
- `[type="hidden"]`
- `.admin-<n'importe quoi>`
- `.token-<n'importe quoi>`
- `.secret-<n'importe quoi>`

Raison : un thème ne doit pas pouvoir styliser les champs de mot de passe ni les
éléments d'interface privilégiés d'une façon qui servirait à maquiller
l'interface pour tromper l'utilisateur.

**Règles de superposition plein écran** : le bloc de règles entier est retiré.

Un bloc est retiré quand il associe `position: fixed` ou `position: sticky` à
une boîte qui couvre l'écran : `inset: 0`, les quatre bords à `0`, ou au moins
deux de `top: 0`, `right: 0`, `bottom: 0`, `left: 0`, `width: 100vw`,
`height: 100vh` dont l'une des deux tailles de l'écran. Un en-tête fixé par
`top: 0; left: 0` et dimensionné par son contenu est conservé. Les déclarations
d'un même sélecteur s'additionnent d'une règle à l'autre : répartir la forme sur
plusieurs blocs ne permet pas d'y échapper. C'est la forme type d'une
superposition CSS d'hameçonnage.

**`pointer-events: none` sur des éléments interactifs** : la déclaration est
retirée du bloc.

Quand le sélecteur d'une règle vise `button`, `[role="button"]`,
`a[href*=logout]` ou `[data-critical]`, toute déclaration `pointer-events: none`
de cette règle est retirée. Le reste de la règle est conservé. Raison : rendre
inactifs les boutons de déconnexion ou d'actions critiques est une attaque par
maquillage de l'interface.

**z-index au-dessus de 999** : ramené à 999.

Toute valeur de `z-index` supérieure à 999 est réécrite en 999, avec un
avertissement. Un thème ne peut pas se placer au-dessus de la couche de fenêtres
et de superpositions de l'application.

### Ce qu'un thème peut faire en CSS

- Personnaliser toute la mise en page de l'accueil, la zone de mise en avant, la
  grille de cartes, la barre latérale
- Restyler les contrôles du lecteur vidéo, la barre de progression, les
  superpositions
- Des polices personnalisées (via `data:` ou un `@font-face` en ligne avec
  `src: url(data:...)`)
- Des animations et transitions (`@keyframes`, `transition`, `animation`)
- Des propriétés personnalisées (`--ma-couleur: #...`), des surcharges de
  `:root`
- Des requêtes média, des requêtes de conteneur
- Tout sélecteur qui ne vise pas les champs de mot de passe, `.admin-*`,
  `.token-*`, `.secret-*`

### Points d'accroche stables (contrat de sélecteurs)

Evey marque un petit ensemble d'éléments cosmétiques et non sensibles avec des
**classes stables**, que vous pouvez viser sans dépendre des classes
utilitaires (qui, elles, changent). C'est un contrat : elles ne seront pas
renommées au fil des refontes.

| Sélecteur | Élément | Ce qu'on peut restyler sans risque |
|---|---|---|
| `.evey-app-header` | la barre du haut | couleurs, hauteur, espacements, flou ; **jamais** masquer ni déplacer les contrôles qu'elle contient |
| `.evey-media-grid` | les grilles de cartes de médias (bibliothèque, rangées) | colonnes, `gap`, alignement |
| `.evey-media-card` | une carte de média (affiche et informations) | largeur, espacements, survol |
| `.evey-poster` | l'affiche ou le visuel (aussi sur la page de détail) | `aspect-ratio`, `border-radius`, ombre, cadrage |
| `.evey-media-detail` | la racine de la page de détail d'un média | la mise en page du corps de la page |

Tout ce que l'assainisseur interdit s'applique toujours (pas de `@import`, pas
de superposition plein écran, pas de `pointer-events:none` sur des contrôles,
pas de `[type=password]` / `.admin-*` / `.token-*` / `.secret-*`, `z-index`
plafonné). Un thème peut restyler ces points d'accroche librement ; il ne peut
**pas** détourner la barre du haut pour masquer ou imiter les contrôles de
confiance (déconnexion, profil, vraies notifications).

### Exemple : affiches plus grandes, grille plus dense, coins plus nets

```css
/* Des affiches plus grandes sur une grille plus serrée */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Exemple : une autre couleur d'accent

L'interface tire ses boutons, ses liens, ses états actifs et la barre de
progression du lecteur de quelques propriétés personnalisées ;
`examples/plugins/theme-dark-blue/theme.css` les énumère. Les redéfinir
restyle tous les écrans d'un coup :

```css
:root {
  --accent: #e8a33d;
  --accent-hover: #f0b95c;
}
```

## theme.js

Servi à `/api/v1/themes/<id>/theme.js` (authentification requise). Chargé dans
une iframe avec `sandbox="allow-scripts"` et une Content Security Policy
verrouillée : le script n'a aucun accès à la page hôte, ne peut faire aucune
requête réseau et ne peut rien lire du stockage.

La seule surface disponible est `window.evey.theme`, injectée par le pont de
l'hôte avant l'exécution de `theme.js`.

Même un script passé à travers l'analyse d'installation ne peut atteindre ni la
page hôte, ni son stockage, ni le réseau : le cadre n'a aucun accès de même
origine et sa politique fixe `connect-src 'none'`.

### `evey.theme.css(rules)`

Injecte une chaîne CSS dans la surface de style de l'hôte, pendant l'exécution.
À utiliser pour un style dynamique qui dépend de valeurs calculées ou d'une
interaction dans le thème.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Écrit un message dans la console du navigateur, préfixé par `[evey-theme]`. Il
n'est pas montré aux utilisateurs.

- `level` : `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg` : une chaîne de texte brut

```js
evey.theme.log("info", "Aurora theme loaded.");
```

### Ce que l'analyse refuse

Une seule correspondance refuse l'installation, avec la raison. L'analyse est
volontairement prudente : un thème n'a aucune raison légitime d'utiliser ces
API.

#### Chaînes littérales

Si l'une des sous-chaînes suivantes apparaît n'importe où dans le code,
l'installation est refusée :

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

#### Motifs

| Motif | Ce qu'il repère |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Les appels directs à des globales interdites |
| `\bimport\s*\(` | L'import dynamique |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | L'accès par crochets aux globales de l'hôte (repère `window["fetch"]`) |
| `\.cookie\b` | Tout accès à `.cookie`, par descripteur de propriété ou calculé |
| `\.constructor\s*\.\s*constructor` | L'évasion par `[].constructor.constructor` |
| `new\s+Function\b` | La construction de code par `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | La lecture de propriétés qui sont des globales interdites |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | L'affectation de propriétés sur les globales de l'hôte |
| `Object\s*\.\s*prototype` | La pollution de prototype |
| `\.register\s*\(\s*['"]\S+\.js` | L'enregistrement d'un service worker |

## Exemples

Deux thèmes sont livrés avec les exemples de plugins :
`examples/plugins/theme-dark-blue` (un court `theme.css` qui ne redéfinit que
des variables de couleur) et `examples/plugins/open-workshop` (une refonte
complète). Partez de l'un ou de l'autre : tous deux passent déjà l'assainisseur.
