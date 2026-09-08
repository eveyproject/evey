---
source_sha: 83d351b09747d0704d8a0c15f44839008404764ccf1d3b95c95e03f938f490a0
---

# ADR 0021 : extensions sur TV, portage WebView maintenant, rendu natif SDUI plus tard

Date : 2026-07-09
Statut : acceptée
Portée : phase 1 implémentée, phase 2 prévue

## Contexte

Les extensions Evey livrent une partie FRONTALE isolée (iframe sur le web,
WebView sur mobile) qui parle à l'hôte par un petit pont postMessage
(`window.evey` : redimensionnement, navigation, notification éphémère,
préférences par profil, API de médias et lecture des ressources relayées par le
PARENT avec un jeton d'extension limité que l'extension ne voit jamais).
L'application mobile embarque déjà un portage durci et à l'identique du
protocole web (`apps/mobile/src/features/plugins/PluginSlot.tsx` : bundles mis
en cache sur disque, armement paresseux en trois étapes, couche matérielle pour
WebGL, blocage de la navigation, gardes contre la traversée de chemin) : « une
extension écrite pour le web tourne telle quelle sur mobile ».

L'application TV a elle aussi besoin de surfaces d'extension. Exigences du
propriétaire du produit : simple, facilement portable, performant sur du
matériel Android TV modeste.

Options envisagées :

1. **Portage WebView** (copier le composant mobile). Simplicité et portabilité
   maximales : les extensions existantes tournent sans modification, et le
   protocole est déjà éprouvé sur deux plateformes. Les atténuations de
   performance existent déjà (armement paresseux, cache de bundles, couche
   matérielle) ; les extensions vitrines ont été réécrites sans aucune
   dépendance (bundles WebGL de 50 à 90 Ko) précisément pour les clients
   modestes. Limites : pas de WebKit sur tvOS (les builds Apple TV n'auraient
   donc aucune extension par cette voie), et une WebView ne participe pas au
   focus de la croix directionnelle, si bien que les emplacements sont en
   affichage seul sur TV.
2. **Rendu natif SDUI.** Le schéma d'interface pilotée par le serveur, une fois
   assaini (`apps/web/src/features/sdui/schema.ts`), a été conçu explicitement
   pour être consommé par des moteurs de rendu React Native : données pures,
   types de nœuds sur liste blanche, budgets anti-déni de service. Focus parfait
   à la croix directionnelle, performances parfaites, fonctionne sur tvOS. Mais
   aucun moteur de rendu RN n'existe encore, aucune extension n'émet encore de
   document SDUI (ni surface de manifeste, ni point d'accès), et cela casse le
   « les extensions web tournent sans modification ».
3. **Hybride** : WebView pour les widgets visuels d'aujourd'hui, SDUI comme voie
   de croissance pour les surfaces interactives et pour tvOS.

## Décision

L'hybride, par phases :

- **Phase 1 (faite)** : porter le `PluginSlot` et le `pluginBridge` du mobile
  vers `apps/tv/src/features/plugins/`, montés sur `home.section` sous le
  bandeau d'accueil, avec `focusable={false}` sur la WebView (affichage seul,
  elle ne capture jamais la croix directionnelle), le même schéma d'armement
  paresseux et de mise en cache, et les mêmes gardes de sécurité
  (`pluginIdShape`, vérification de traversée de chemin sur les ressources).
- **Phase 2 (plus tard)** : déplacer le schéma SDUI dans un module partagé de
  `packages/`, écrire un moteur de rendu RN avec un vrai focus à la croix
  directionnelle, et ajouter une surface de manifeste pour les documents SDUI.
  C'est la voie des widgets TV interactifs et de tvOS, où la WebView n'existe
  pas. Les extensions pourront alors déclarer une indication de performance
  (`heavy: webgl` par exemple), pour que les profils TV modestes n'arment pas du
  tout les extensions 3D.

## Conséquences

- Toutes les extensions existantes (weather-v2, cosmos, marquee) apparaissent sur
  TV sans la moindre modification de leur côté.
- Les builds Apple TV n'affichent aucun emplacement d'extension jusqu'à la phase 2.
- Les emplacements d'extension sur TV sont volontairement d'ambiance et en
  affichage seul pour l'instant ; l'interaction reste sur le web et le mobile
  tant que le moteur de rendu SDUI n'est pas livré.
