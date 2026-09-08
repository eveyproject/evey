---
source_sha: 2ec48afdaf5dd44ee661048d21aa84c7ac2b22e7b6454b4270a203bd473922b4
---

# Modèle de sécurité des thèmes

## Thème et extension : la distinction essentielle

Un **thème** ne vit que côté interface. Il n'a ni processus serveur, ni environnement d'exécution de langage, ni socket IPC, ni permission côté serveur. Il se compose de :

- `theme.css` - une feuille de style injectée dans le `<head>` de chaque page
- `theme.js` (facultatif) - du JavaScript exécuté dans un contexte isolé, doté d'une API `evey.theme.*` restreinte

Une **extension**, elle, a toujours un processus serveur (`kind: plugin`, avec `language` et `entrypoint` obligatoires). Un thème qui tente de déclarer `language`, `entrypoint` ou `permissions` est refusé à l'installation.

Le CSS comme le JS sont analysés statiquement à l'installation, avant que le thème soit accepté.

## Les règles de l'assainisseur CSS

`theme.css` traverse `SanitizeCSS` à l'installation. L'assainisseur réécrit ou supprime les constructions dangereuses et rend une liste d'avertissements. Il **ne rejette pas** le fichier dans son ensemble : un CSS propre en ressort inchangé.

### Constructions bloquées

**`@import`** - purement et simplement supprimé.

Toute directive `@import` est retirée. La raison : un thème doit se suffire à lui-même, et importer une feuille de style externe est un vecteur de fuite réseau.

**`url()` externe** - remplacé par `none`.

Les valeurs `url()` qui pointent vers une URL `http://`, `https://` ou relative au protocole (`//`) sont remplacées. Les URI `data:` (`data:image/*`, `data:font/*`) sont autorisées et passent inchangées.

**`javascript:` dans un `url()`** - remplacé par `none`.

**`expression()`** (l'héritage d'IE) - remplacé par `none`.

**`-moz-binding` et `behavior`** - supprimés.

Ce sont d'anciens vecteurs d'exécution XBL/HTC, retirés purement et simplement.

**Règles visant un sélecteur sensible** - le bloc de règles entier est supprimé.

Tout bloc dont le sélecteur correspond à l'un des motifs suivants est retiré intégralement :

- `[type="password"]`, `[type='password']` ou `[type=password]`
- `[type="hidden"]`
- `.admin-<quoi que ce soit>`
- `.token-<quoi que ce soit>`
- `.secret-<quoi que ce soit>`

La raison : un thème ne doit pas pouvoir mettre en forme les champs de mot de passe ni les éléments d'interface privilégiés d'une manière qui se prêterait à un détournement d'interface.

**Règles de surcouche plein écran** - le bloc de règles entier est supprimé.

Un bloc est retiré dès qu'il contient `position: fixed` ou `position: sticky` combiné à au moins deux de ces déclarations : `top: 0`, `left: 0`, `width: 100vw`, `height: 100vh`. C'est le motif canonique de la surcouche CSS d'hameçonnage.

**`pointer-events: none` sur un élément interactif** - la déclaration est retirée du bloc.

Quand le sélecteur d'une règle vise `button`, `[role="button"]`, `a[href*=logout]` ou `[data-critical]`, toute déclaration `pointer-events: none` de cette règle est retirée, le reste de la règle étant conservé. La raison : rendre incliquable un bouton de déconnexion ou d'action critique est une attaque par détournement d'interface.

**`z-index` au-delà de 999** - ramené à 999.

Toute valeur de `z-index` supérieure à 999 est réécrite en 999, et un avertissement est consigné. Un thème n'a pas à se placer au-dessus de la couche de fenêtres modales et de surcouches de l'application elle-même.

### Ce qu'un thème PEUT faire en CSS

- Refondre toute la mise en page de l'accueil, la zone de mise en avant, la grille de fiches, la barre latérale
- Restyler les contrôles du lecteur vidéo, la barre de progression, les surcouches
- Employer des polices sur mesure (par `data:`, ou par un `@font-face` en ligne avec `src: url(data:...)`)
- Poser des animations et des transitions (`@keyframes`, `transition`, `animation`)
- Définir des propriétés personnalisées (`--my-color: #...`), redéfinir des valeurs sur `:root`
- Écrire des requêtes média et des requêtes de conteneur
- Employer tout sélecteur qui ne vise ni les champs de mot de passe, ni `.admin-*`, `.token-*`, `.secret-*`

## Les règles de l'analyseur du JS de thème

`theme.js` est analysé à l'installation par `internal/pkg/themejs/scan.go`. Une seule correspondance suffit à faire refuser l'installation, avec une erreur explicite. Il n'y a pas d'avertissement : l'analyseur est délibérément conservateur, les faux positifs étant acceptables puisqu'un thème n'a aucune raison légitime de mentionner ces API.

### Chaînes littérales bloquées

Si l'une des sous-chaînes suivantes apparaît où que ce soit dans le source, l'installation est refusée :

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

### Motifs d'expressions régulières bloqués

| Motif | Ce qu'il attrape |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | Les appels directs aux objets globaux interdits |
| `\bimport\s*\(` | L'import dynamique |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | L'accès par crochets aux globales de l'hôte (attrape `window["fetch"]`) |
| `\.cookie\b` | tout accès à `.cookie`, par descripteur de propriété comme par accès calculé |
| `\.constructor\s*\.\s*constructor` | L'évasion par `[].constructor.constructor` |
| `new\s+Function\b` | La construction de code par `new Function(...)` |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | La lecture d'une propriété des globales interdites |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | L'affectation d'une propriété sur les globales de l'hôte |
| `Object\s*\.\s*prototype` | La pollution de prototype |
| `\.register\s*\(\s*['"]\S+\.js` | L'enregistrement d'un service worker |

### Isolation du JS de thème à l'exécution

À l'exécution, `theme.js` tourne dans une iframe en `sandbox="allow-scripts"`, avec un `srcdoc` sous CSP verrouillée. Même si l'analyseur d'installation venait à être contourné, cette iframe n'a aucun accès de même origine au document hôte, aucun accès au `window`, au `localStorage` ni aux cookies de l'hôte, et aucune sortie réseau (`connect-src 'none'` dans la CSP).

Le seul canal de communication est l'API `evey.theme.*` injectée par le pont de l'hôte. Les méthodes disponibles sont décrites dans [`api-reference.md`](api-reference.md).

## Ce que vous POUVEZ faire dans theme.js

- Appeler `evey.theme.css(rules)` pour injecter du CSS dans la surface de style de l'hôte
- Appeler `evey.theme.log(level, msg)` pour émettre des messages de débogage vers la console du navigateur
- Enregistrer des fonctions de rendu de composants par `evey.theme.registerComponent(slot, render)` (V2 - voir plus bas)
- Observer en lecture seule l'état du lecteur ou de l'application par `evey.theme.observe(stateKey, cb)` (V2)

## Ce que vous ne POUVEZ PAS faire

- Récupérer des données depuis une URL, quelle qu'elle soit (`fetch`, `XMLHttpRequest`, `WebSocket`, `navigator.sendBeacon`)
- Lire les identifiants ou les jetons de l'utilisateur (`document.cookie`, `localStorage`, `sessionStorage`)
- Sortir du contexte de l'iframe (`window.parent`, `window.top`, `frameElement`)
- Afficher une surcouche plein écran qui recouvre toute la fenêtre
- Viser en CSS les champs de mot de passe ou les sélecteurs `.admin-*`, `.token-*`, `.secret-*`
- Rendre incliquables les boutons de déconnexion ou d'action critique (`pointer-events: none`)
- Se placer au-dessus des fenêtres modales de l'application (`z-index > 999`)
