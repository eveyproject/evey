---
source_sha: 7876d37df60eb4fbee9051cf4489539dc2cf55ce751dd381208379a28a4a22e5
---

# ADR-0013 : moniteur de ressources des extensions en espace utilisateur (cgroups abandonnés)

- Statut : acceptée
- Date : 2026-05-23

## Contexte

Chaque extension tourne dans un processus fils et doit être plafonnée
individuellement en mémoire, en nombre de processus et de fils d'exécution, en
place disque et en processeur. Le mécanisme évident est cgroups v2.

Il ne fonctionne pas dans notre déploiement. Evey est livré sous forme d'image
préconstruite que des auto-hébergeurs lancent avec `docker compose` sur des
machines du type NAS, donc dans des **conteneurs non privilégiés**. Piloter
cgroup v2 exige soit `--privileged`, soit une délégation explicite au conteneur,
et nous ne pouvons compter ni sur l'un ni sur l'autre chez un utilisateur
quelconque. Les tentatives précédentes allaient dans le mauvais sens : un point
d'entrée qui démarrait en root pour redescendre ensuite, ou des limites de
mémoire et de processeur dans le compose, qui plafonnent le conteneur ENTIER et
non chaque extension. Aucune ne donne d'application par extension, et les deux
déportent de la complexité et du root sur l'administrateur.

## Décision

Abandonner cgroups et appliquer les plafonds depuis **l'espace utilisateur, dans
le processus du serveur** :

- Une goroutine de surveillance parcourt l'arbre de processus de chaque
  extension toutes les deux secondes environ, lit `/proc` pour la mémoire
  résidente, le nombre de fils et le temps processeur, et mesure le dossier de
  l'extension pour le disque. Au-delà d'un plafond, l'extension fautive est
  arrêtée.
- Le **processeur** est modelé au lancement par la politesse d'ordonnancement
  (`nice` à 10 par défaut) plutôt que par un quota strict : les extensions
  cèdent le pas au serveur en cas de contention.
- Le serveur tourne **non root par défaut**, et le compose par défaut ne porte
  aucune limite de mémoire ni de processeur.
- Les plafonds sont **modifiables depuis le panneau d'administration**, résolus
  au démarrage et appliqués à chaud lors d'un changement. Les valeurs armées
  sont journalisées au démarrage.

## Conséquences

- **Fonctionne dans un conteneur non privilégié, sous un utilisateur non root**,
  c'est-à-dire le seul environnement que nous puissions garantir. Aucune
  préparation, aucun `--privileged`.
- **La granularité est celle du balayage**, environ deux secondes : une
  extension peut brièvement dépasser un plafond avant d'être arrêtée. C'est
  acceptable, il s'agit d'une frontière de confiance vérifiée pour les
  extensions d'un auto-hébergeur, pas d'un ordonnanceur multi-locataire hostile.
- **Le calcul du disque parcourt le dossier à chaque balayage**, ce qui convient
  à des arbres de taille d'extension et serait à revoir si une extension y
  stockait de gros volumes.
- **Décision de référence : ne PAS retenter cgroups dans un Docker non
  privilégié.** Ce point couvre les plafonds de ressources seulement ; le
  confinement des fichiers et des appels système relève du bac à sable
  Landlock et seccomp.

## Références

- `internal/adapters/driven/pluginrunner/runner_resmon.go` - le balayeur, les
  lectures de `/proc`, la mesure du dossier
- `internal/app/plugin/resource_config.go` - résolution et application à chaud
  des plafonds
- `internal/adapters/driving/http/settings_handler.go` - les plafonds
  modifiables par l'administrateur
