---
source_sha: 935a77e1c7b524e4df14c0a67f9ad57f4571098b770e8c3245fda5189aefc0c3
reviewed: true
---
# Tâches de fond et charge du serveur

Pourquoi le serveur est occupé quand personne ne regarde, et pourquoi les
miniatures d'une nouvelle bibliothèque mettent des heures à arriver : en plus
de la lecture, le serveur mène son propre travail (scanner les bibliothèques,
récupérer les illustrations, générer les miniatures de la barre de lecture,
détecter intros et génériques, analyser les fichiers). Cette page dit comment
ce travail partage la machine avec les personnes qui regardent, et où régler
sa part.

## La lecture passe en premier

Les tâches de fond puisent dans une part du processeur, la **Part globale**
de **Administration → Configuration du serveur → Puissance allouée** : **25 %** par
défaut, **50 %** pour les accélérer quand personne n'utilise le serveur. Sous
**Tâches lourdes**, l'une d'elles (analyse des nouveaux médias, miniatures de
navigation, détection d'intro, classification de complexité) peut recevoir sa
propre part, qui remplace la part globale pour cette tâche ; les parts ne se
cumulent jamais. Un changement s'applique à l'élément suivant, sans
redémarrage.

Quelle que soit cette part, une tâche de fond se met en pause pendant qu'une
personne regarde, et reprend ensuite. Sur une nouvelle bibliothèque, les
miniatures et les repères d'intro arrivent donc en quelques heures plutôt
qu'en quelques minutes si le serveur est utilisé.

## Ce qui tourne en arrière-plan

| Tâche | Ce qu'elle apporte | Quand elle tourne |
| --- | --- | --- |
| Scan des bibliothèques | Les fichiers ajoutés, modifiés et supprimés | À chaque changement, selon la planification, au démarrage si activé. Voir [Bibliothèques et métadonnées](../administration/libraries.md). |
| Téléchargement des illustrations | Affiches et fonds conservés sur le serveur, en WebP | Pendant le scan ; les manquantes sont retentées toutes les heures. Désactivé, les images sont chargées depuis Internet par le navigateur, ce que certains navigateurs bloquent. |
| Miniatures de navigation | Les aperçus affichés en faisant glisser la barre de lecture (une image toutes les 7 secondes) | À l'ajout d'un titre, et au démarrage pour ceux qui n'en ont pas. **Activer les miniatures de navigation** dans la **Configuration du serveur** les désactive. |
| Détection des intros et génériques | Les boutons **Passer** | En continu, pour les titres que les bases communautaires ne couvrent pas |
| Index des images clés et niveaux de codec | La lecture directe et la copie sans réencodage, des couleurs correctes | En continu, sur les fichiers scannés avant que ces informations ne soient collectées |

## Mémoire

**Profil mémoire** (**Administration → Configuration du serveur**) : **Faible** met en
veille les plugins inactifs pour économiser la mémoire, et les réveille à la
demande. **Auto (recommandé)** choisit Faible sur les machines de moins de
3 Go de mémoire.

## Réglages lus au démarrage

Quelques réglages ne sont lus qu'une fois, au démarrage du serveur :
enregistrer conserve votre choix, redémarrer l'applique. Le panneau le signale
à côté d'eux. Il s'agit de la carte **Travaux en arrière-plan** (mise en pause
et rythme des passes d'images clés et de niveaux de codec), de l'interrupteur
**Sources de détection des intros**, et des **Limites avancées** du
transcodage.
