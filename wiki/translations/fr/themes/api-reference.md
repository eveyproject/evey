---
source_sha: 72324d88dd91ef30b9ee280c903c4879dad626512f8f23deee264b8281e0538f
---

# Référence de l'API des thèmes

Ce document s'adresse aux développeurs externes qui écrivent un thème pour Evey. Pour les contraintes de sécurité imposées aux thèmes, voir [`security-model.md`](security-model.md).

## Ce qu'est un thème

Un thème est un paquet `kind: theme`. Il n'a **aucun processus serveur**. Il peut contenir :

- `theme.css` - injecté dans le `<head>` de chaque page, après assainissement
- `theme.js` - du JavaScript facultatif, chargé dans un contexte isolé doté d'une API `evey.theme.*` restreinte

Un thème ne peut déclarer ni `language`, ni `entrypoint`, ni `permissions` : toute tentative fait refuser l'installation.

## Schéma du manifeste

```yaml
name: aurora               # obligatoire ; nom lisible par un humain
version: 1.0.0             # obligatoire ; semver
kind: theme                # obligatoire ; doit valoir "theme"

description: "..."         # facultatif
author: "..."              # facultatif
min_evey_version: "1.0.0"  # facultatif

# PAS de language, entrypoint ni permissions - invalides pour un thème
```

Le ZIP doit contenir `theme.css` et/ou `theme.js` à sa racine. Un paquet dépourvu des deux est accepté, mais le thème n'a alors aucun effet visible.

## theme.css

Servi sur `/api/v1/themes/<id>/theme.css`. Le fichier traverse l'assainisseur CSS au moment de l'installation (voir [`security-model.md`](security-model.md) pour la liste des constructions bloquées). C'est la sortie assainie qui est stockée puis servie : ce que vous écrivez et ce qui est injecté peuvent donc différer si des règles se sont déclenchées.

La feuille de style est injectée sous forme de `<link rel="stylesheet">` dans le `<head>` et s'applique globalement au document hôte. Le CSS standard y fonctionne : propriétés personnalisées, `@keyframes`, `@media`, `@font-face` (avec des sources `data:`), `@layer`, requêtes de conteneur.

### Points d'accroche stables (contrat de sélecteurs)

Evey marque d'un petit jeu de **classes stables** quelques éléments cosmétiques
et non sensibles, que vous pouvez viser sans dépendre des classes utilitaires,
volatiles par nature. C'est un contrat : ces classes ne seront pas renommées au
fil des refontes.

| Sélecteur | Élément | Ce qu'on peut restyler sans risque |
|---|---|---|
| `.evey-app-header` | la barre supérieure | couleurs, hauteur, espacements, flou - **jamais** masquer ni déplacer les contrôles qu'elle contient |
| `.evey-media-grid` | les grilles de fiches de médias (bibliothèque, rangées) | colonnes, `gap`, alignement |
| `.evey-media-card` | une fiche de média (affiche et informations) | largeur, espacements, survol |
| `.evey-poster` | le visuel d'affiche ou de jaquette (page de détail comprise) | `aspect-ratio`, `border-radius`, ombre, cadrage |
| `.evey-media-detail` | la racine de la page de détail d'un média | mise en page du corps de la fiche |

Tout ce que l'assainisseur interdit reste interdit ici (pas d'`@import`, pas de
surcouche plein écran, pas de `pointer-events:none` sur les contrôles, pas de
`[type=password]`, `.admin-*`, `.token-*` ni `.secret-*`, `z-index` borné). Un
thème peut restyler librement ces points d'accroche ; il ne peut en revanche
**pas** détourner l'en-tête pour masquer ou imiter les contrôles de confiance
(déconnexion, profil, vraies notifications).

### Exemple : des affiches plus grandes, une grille plus dense, des angles plus vifs

```css
/* Des affiches plus grandes sur une grille plus serrée */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### Exemple : un thème de lecteur sur mesure

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

Servi sur `/api/v1/themes/<id>/theme.js` (authentification requise). Il est chargé dans une iframe en `sandbox="allow-scripts"`, sous une Content Security Policy verrouillée : le script n'a aucun accès à la page hôte, ne peut émettre aucune requête réseau et ne peut lire aucun stockage.

La seule surface à sa disposition est `window.evey.theme`, injectée par le pont de l'hôte avant l'exécution de `theme.js`.

### `evey.theme.css(rules)`

Injecte une chaîne CSS dans la surface de style de l'hôte, à l'exécution. À employer pour une mise en forme dynamique, qui dépend de valeurs calculées ou d'une interaction de l'utilisateur au sein du thème.

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

Émet un message de journal vers la console du navigateur, préfixé par `[evey-theme]`. Il n'apparaît jamais aux utilisateurs finaux.

- `level` : `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg` : une chaîne de texte brut

```js
evey.theme.log("info", "Aurora theme loaded.");
```

## API V2 (feuille de route)

Les méthodes suivantes sont prévues et ne sont pas encore disponibles en V1 :

### `evey.theme.registerComponent(slot, render)` (V2)

Enregistre une fonction de rendu pour un emplacement d'insertion nommé. L'hôte appelle `render(props, container)` au montage de cet emplacement. `container` est un nœud DOM cantonné au contexte shadow du thème : ce qui y est écrit apparaît dans la page hôte, à l'intérieur d'un ShadowRoot.

```js
// V2 - pas encore disponible
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)` (V2)

S'abonne aux mises à jour, en lecture seule, de l'état de l'hôte. La fonction de rappel est invoquée chaque fois que la clé observée change.

```js
// V2 - pas encore disponible
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)` (V2)

Applique une animation nommée à un élément de l'hôte désigné par `selector`, en passant par le pont (aucun accès direct au DOM).

## Exemples

Deux thèmes accompagnent les exemples d'extensions : `examples/plugins/theme-dark-blue` (un `theme.css` court qui ne remplace que des variables de couleur) et `examples/plugins/open-workshop` (une refonte visuelle complète). Copiez l'un ou l'autre comme point de départ - tous deux passent déjà l'assainisseur.
