---
source_sha: 7e7ce6104368af3eb62d01d284ca729176dfea683c5a589a317463e96b77691a
---

# Système de design de l'interface

Les règles qui gardent les trois clients (web, mobile, TV) cohérents et
rapides, et qui empêchent les rustines ponctuelles de s'accumuler. Quand un
bug d'interface donne envie d'ajouter un cas particulier, commencez par
regarder ici : la plupart des familles de bugs ont déjà une réponse
canonique, et le correctif appartient à la convention (ou à son utilitaire),
pas à l'endroit qui l'appelle.

## Les jetons de style, source unique de vérité

`frontend/packages/theme/src/themeVars.ts` est la source unique de vérité pour
les couleurs, les rayons et les espacements. Les propriétés CSS personnalisées
du web en sont **générées** (`packages/theme/gen-theme-css.mts` →
`apps/web/public/themes/*.css`) ; le client React Native lit `tokens.ts`, lui
aussi **dérivé** du même `themeVars.ts` et exposé par `useTheme()`. On modifie
`themeVars.ts`, jamais le CSS généré ni les jetons dérivés côté natif.

- Ne codez jamais une couleur en dur. Web : `var(--accent)`, `var(--text-muted)`,
  etc. Natif : `theme.colors.accent`.
- N'inventez ni rayon ni pas d'espacement ; servez-vous de l'échelle de jetons.
- Une nouvelle couleur sémantique (la teinte d'un badge, par exemple) s'ajoute
  à `ThemeTokens` et à tous les thèmes, elle ne s'écrit pas en ligne là où on
  en a eu besoin la première fois.

## Cibles tactiles (posé le 2026-07-11)

Tout élément interactif doit offrir une zone de contact d'au moins 44x44 px
(WCAG 2.5.5, Apple HIG). Deux utilitaires de `apps/web/src/styles/main.css`
s'en chargent :

- `.tap-target` agrandit l'élément lui-même (`min-width/height: 44px`). À
  utiliser quand la mise en page permet à la boîte de grandir (boutons de
  barre d'outils, bouton d'avatar).
- `.tap-hit` conserve la taille affichée et étend la ZONE DE CONTACT par un
  pseudo-élément centré. À utiliser sur les petites commandes positionnées en
  absolu, là où agrandir la boîte déplacerait le design (badges de vignette,
  cases à cocher, points de débordement).

Tout nouvel élément interactif choisit l'un des deux. Une commande de moins de
44 px sans l'une de ces classes est un bug.

## Règles de performance sur mobile

Apprises à la dure (passage de perf mobile-web du 11-07) ; en enfreindre une
réintroduit des saccades très difficiles à bissecter ensuite :

1. **Pas de `backdrop-filter` sur les surfaces rendues en mobile.** Un flou sur
   une barre collante se repeint à chaque image défilée ; un flou par vignette
   multiplie les couches du compositeur. Gardez les effets de verre derrière
   `@media (min-width: 768px)` ou les variantes `[@media(hover:hover)]`.
   Repliez-vous sur une couleur de surface OPAQUE : une translucidité sans
   flou donne l'impression que c'est cassé.
2. **Ne remontez jamais une page pour obtenir une transition.**
   `key={pathname}` sur un nœud de mise en page démonte et remonte la page
   entière à chaque navigation. La remise à zéro du défilement et les
   animations d'entrée passent par un effet sur une ref (`scrollTo` +
   `el.animate` de WAAPI), avec détection de fonctionnalité pour jsdom.
3. **Bornez ce qu'une page empilée monte au départ.** Les rangées horizontales
   n'affichent qu'un nombre borné d'éléments (voir `INITIAL_CARDS` de
   `MediaSlider`) et révèlent le reste à la première interaction ; les grilles
   verticales sont virtualisées (`VirtuosoGrid`).
4. **Un fichier de route ne doit pas importer son composant.** Un fichier de
   route non paresseux est importé statiquement par `routeTree.gen.ts` : tout
   ce qu'il tire est livré à chaque page. Les fonctionnalités lourdes (hls.js !)
   vivent dans le fichier `.lazy.tsx` voisin. Vérification après ajout d'une
   route : le `modulepreload` de `dist/index.html` ne doit lister que vendor et
   i18n.
5. **Les abonnements par élément sélectionnent des primitives.** Un hook
   consommé par chaque vignette d'une liste doit `select` une primitive
   (booléen, chaîne) depuis sa requête, pour qu'un rechargement ne re-rende que
   les éléments dont la valeur a changé. Ne donnez jamais à chaque élément une
   Map ou un tableau fraîchement construits (comparez `useIsWatched` et
   `useWatchedMap`).
6. **Champs de saisie iOS à 16 px minimum** (`text-base`) sous `sm`, sinon
   Safari zoome à la mise au point.
7. **`dvh`, pas `vh`**, pour les mises en page pleine hauteur (barre d'URL
   mobile), et `env(safe-area-inset-bottom)` sur les barres fixes du bas.

## Primitives établies : servez-vous en, ne les dupliquez pas

| Sujet | Réponse canonique |
|---|---|
| Récupération de données | TanStack Query via les hooks générés de `@evey/api` ; `fetch` brut réservé aux corps en flux (fin de SSE, téléchargements) |
| Client HTTP | `eveyFetch` (authentification, rafraîchissement, CSRF) ; jamais de fetch brut pour une API JSON |
| Notifications éphémères | `toast` depuis `@evey/store` (un magasin, un conteneur) |
| État côté client | Magasins Zustand dans `@evey/store`, un sujet par magasin |
| Navigation (web) | `useNavigate` / `Link` de TanStack Router ; jamais `window.location.href` |
| Fenêtres modales | `components/ui/Modal` (plafond de défilement et zone de sécurité déjà gérés) |
| Erreurs dans les vues | branche `isError` avec `EmptyState` et les clés `common.errorBoundary.*` ; ne jamais retomber sur le texte d'état vide |
| Mutations | une mise à jour optimiste s'accompagne d'un retour arrière et d'un toast `onError` |
| Invalidation de l'état vu | `lib/invalidateWatchedState.ts` : les nouveaux flux sensibles à cet état s'ajoutent là, nulle part ailleurs |
| Jetons dans les URL | jamais le JWT du compte. La diffusion et le téléchargement de médias utilisent des jetons de flux limités au média (frappés via `useStreamToken` / `mintToken` sur `DownloadButton`) ; les flux d'administration envoient le Bearer par SSE en fetch-streaming |

## Internationalisation

Toute chaîne visible par l'utilisateur passe par `t()`, avec des clés
présentes dans les SIX langues (en, fr, de, es, hi, zh). Pas de littéral de
repli français ou anglais dans un composant. Réutilisez les clés existantes
avant d'en frapper de nouvelles (`common.*` est riche).

## Processus

- Modifications web : typecheck et `vitest run` dans le conteneur `evey-web`.
- Tout ce qui touche aux listes, à la navigation ou au lecteur : à vérifier sur
  une fenêtre de 390 px (Playwright) avant de considérer que c'est terminé.
- Le lecteur est VITAL : aucune modification de `features/playback/**` sans
  surveillance, et sans avoir réellement rejoué les parcours concernés.
