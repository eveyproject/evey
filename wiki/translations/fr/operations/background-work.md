---
source_sha: a10d4c679e7aed09a2fa56ed708a6beb299e8c731fcea4016e7cbf7b9f1e30e5
reviewed: true
---
# Tâches de fond et charge du serveur

En plus de la lecture, le serveur mène son propre travail : scanner les
bibliothèques, récupérer les illustrations, générer les miniatures de la barre
de navigation, détecter intros et génériques, analyser les fichiers (images
clés, niveaux de codec, santé des médias). Cette page explique comment ce
travail partage la machine avec les personnes qui regardent.

## La lecture passe en premier

Les tâches de fond puisent dans une part du processeur, **25 %** par défaut
(**Admin → Configuration du serveur → Puissance allouée**). **50 %** les
accélère, pour les moments où personne n'utilise le serveur ; une tâche
lourde (analyse des médias, miniatures, détection des intros, classification)
peut recevoir sa propre part. Un changement s'applique à l'élément suivant.

Quelle que soit cette part, une tâche de fond se met en pause pendant qu'une
personne regarde, et reprend ensuite. Sur une nouvelle bibliothèque, les
miniatures et les repères d'intro arrivent donc en quelques heures plutôt
qu'en quelques minutes si le serveur est utilisé.

## Ce qui tourne en arrière-plan

| Tâche | Ce qu'elle apporte | Quand elle tourne |
| --- | --- | --- |
| Scan des bibliothèques | Les fichiers ajoutés, modifiés et supprimés | À chaque changement, selon la planification, au démarrage si activé. Voir [Bibliothèques et métadonnées](../administration/libraries.md). |
| Téléchargement des illustrations | Affiches et fonds conservés sur le serveur, en WebP | Pendant le scan ; les manquantes sont retentées toutes les heures. Désactivé, les images sont chargées depuis Internet par le navigateur, ce que certains navigateurs bloquent. |
| Miniatures de navigation | Les aperçus affichés en faisant glisser la barre de lecture (une image toutes les 7 secondes) | À l'ajout d'un titre, et au démarrage pour ceux qui n'en ont pas. Peut être désactivé dans la **Configuration du serveur**. |
| Détection des intros et génériques | Les boutons **Passer** | En continu, pour les titres que les bases communautaires ne couvrent pas |
| Index des images clés et niveaux de codec | La lecture directe et la copie sans réencodage, des couleurs correctes | En continu, sur les fichiers scannés avant que ces informations ne soient collectées |

## Mémoire

**Profil mémoire** (**Admin → Configuration du serveur**) : **Faible** met en
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
