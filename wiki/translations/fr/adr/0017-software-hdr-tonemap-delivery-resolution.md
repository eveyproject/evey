---
source_sha: 5281ba850734622c576388dda092046cd158402067d07fb7d6fb3ff57f23e58f
---

# ADR-0017 : tonemap logiciel HDR→SDR à la résolution de diffusion, pas à celle de la source

- Statut : acceptée
- Date : 2026-06-04

## Contexte

Sur un NAS modeste, une machine de génération old iGPU sans décodeur HEVC
matériel et sans traitement VAAPI 10 bits, la lecture d'un titre 4K HEVC 10 bits
en HDR faisait s'effondrer l'échelle de qualité jusqu'à 360p. Un serveur open
source de référence, configuré **sans accélération matérielle** sur la *même*
machine, tenait environ 1080p à 8 Mbit/s avec des couleurs correctes et quelques
rares coupures. La machine était donc capable : c'est notre chaîne logicielle
qui était bien moins efficace.

Deux coûts dominaient, mesurés au banc d'essai sur un échantillon 4K HEVC HDR10.

1. **Le tonemap tournait à la résolution de la source.** La chaîne HDR vers SDR
   traitait l'image complète en 3840 par 2160 *avant* la réduction d'échelle,
   soit 8,3 millions de pixels par image de calcul flottant sur l'étape
   dominante. Replier le redimensionnement dans la première passe, pour que la
   chaîne flottante travaille en 1080p, a fait passer la mesure de 36 à 75-83
   images par seconde, soit un facteur d'environ 2,2, et cela correspond à
   l'ordre qu'emploie le serveur de référence.
2. **L'éventail multi-variantes.** Un seul processus effectuait le tonemap une
   fois en 4K puis se divisait en plusieurs encodages parallèles. Une seule
   variante 1080p contre la rafale de quatre paliers a mesuré 67 contre 26
   images par seconde, un facteur d'environ 2,6.

Le banc de calibration mesurait lui aussi l'ancienne géométrie, si bien que le
plafond de résolution enregistré ne reflétait pas la chaîne que le serveur
allait réellement exécuter après la correction.

C'est le pendant **logiciel** de la décision sur l'accélération matérielle :
quand le matériel est absent ou inutilisable pour un titre, le chemin logiciel
doit être efficace par lui-même, et non traité comme un repli rare.

## Décision

Faire le tonemap à la **résolution de diffusion**, en lumière linéaire, sur tous
les chemins logiciels.

La réduction d'échelle est repliée dans la première passe linéarisante, si bien
que le calcul flottant opère sur l'image à la résolution livrée. Le chemin
d'incrustation des sous-titres conserve l'ancienne chaîne pleine résolution, car
l'incrustation doit avoir lieu à taille fixe avant la mise à l'échelle.

En mode multi-variantes, le flux partagé est tonemappé à la largeur du palier le
plus haut, les paliers inférieurs ne faisant plus que réduire à partir de là.

Le banc de calibration a été aligné sur cette géométrie et sur le préréglage
d'encodage réellement utilisé à l'exécution. **Relancer la calibration sur la
machine cible est nécessaire** pour que le plafond monte depuis son ancienne
valeur, mesurée sur la chaîne lente.

Redimensionner en lumière linéaire est colorimétriquement correct, et sans doute
de meilleure qualité qu'une mise à l'échelle dans le domaine encodé.

## Conséquences

- **Environ deux fois plus de débit sur l'étape dominante**, ce qui suffit à
  sortir une machine old iGPU du plancher 360p et à l'approcher de la parité avec
  la référence. Le plancher qui demeure est le **décodage logiciel du 4K HEVC 10
  bits** lui-même, qui touche chaque pixel une fois quel que soit l'ordre des
  filtres : le serveur de référence a la même limite, d'où ses coupures
  occasionnelles.
- **Le plafond est mesuré, pas réglé à la main.** Tant que la calibration n'a pas
  été relancée sur la machine de déploiement, l'échelle reste bornée par
  l'ancienne valeur.
- **L'incrustation de sous-titres est inchangée** : le repli ne concerne que le
  chemin logiciel HDR.
- Le retour en arrière consiste à ne pas replier la réduction d'échelle.
- Le comportement est figé par des tests à chaque couche : la construction du
  filtre, la forme des arguments dans les deux modes, et le plafond appliqué à
  l'échelle.

## Alternatives envisagées

- **Décoder en logiciel puis confier l'encodage au matériel.** Envisagé, puisque
  la machine expose bien un encodeur matériel. Écarté pour cette machine et
  cette version : le matériel y est coupé, la calibration n'a trouvé aucun
  plafond d'encodage matériel utilisable, et le pipeline découplé qui manque
  représente un changement plus lourd sur le chemin vital du lecteur. Conservé
  comme travail futur.
- **Se contenter de relever le plafond de résolution.** Écarté, parce que ce
  serait malhonnête : cela annoncerait des paliers que la chaîne lente ne peut
  pas tenir, et affamerait le lecteur. La bonne réponse est de rendre la chaîne
  rapide *puis* de remesurer le plafond contre elle.
- **Des échelles par classe de contenu.** Orthogonal et reporté : c'est une
  optimisation de bande passante, pas un remède au coût du tonemap.

## Mise à jour (2026-07-12)

Le mode d'échelle par défaut a été **inversé** après retour du terrain.
Contrairement à ce qu'indiquait la section des conséquences, c'est le mode
multi-variantes - qui partage UN décodage entre tous les paliers - qui est
redevenu le défaut stable pour la cible NAS modeste, le mode à variante unique
devenant une option pour les machines assez puissantes pour redécoder par
palier. Sur une machine limitée par le décodage, la variante unique redécode la
source 4K pour chaque palier et retombe au plancher 360p : c'est exactement le
mauvais défaut à cet endroit. La décision centrale de cette page, le tonemap à
la résolution de diffusion, est inchangée ; seul le mode d'échelle par défaut a
basculé.
