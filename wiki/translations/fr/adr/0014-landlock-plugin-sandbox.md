---
source_sha: 7d140ddcf22a3fb82b8567865310393f6a315c4e144c344f677e95ebc377a3b2
---

# ADR-0014 : bac à sable Landlock, appliqué par une amorce de lancement

- Statut : acceptée
- Date : 2026-05-24

## Contexte

Les extensions sont des processus tiers sur le serveur personnel de quelqu'un.
Les confiner doit fonctionner dans un conteneur ordinaire et non privilégié,
parce que c'est le seul environnement que nous puissions garantir : tout ce qui
réclamerait `--privileged`, un profil seccomp relâché ou une installation en
deux temps ne serait tout simplement pas appliqué.

## Décision

Une petite amorce est placée en tête de la ligne de commande de l'extension et
fait son travail avant de lui céder la main. Elle bascule vers un uid non
privilégié quand on le lui demande, applique une liste blanche de fichiers
Landlock - le dossier de l'extension et sa socket, les bibliothèques système en
lecture seule, tout le reste refusé, `/data` et les extensions voisines
comprises - puis installe une liste noire d'appels système seccomp, et exécute
seulement ensuite l'extension.

Landlock confine le fil d'exécution appelant depuis l'intérieur du processus. Il
ne réclame ni montage, ni espace de noms, ni privilège, et tourne donc sous les
seccomp et AppArmor par défaut de Docker sans rien à configurer.

Seccomp est installé **avant** l'exécution, et c'est ce qui le rend applicable
aux extensions compilées. Une bibliothèque préchargée ne le peut pas : un
binaire statique ne la charge jamais.

Le mode par défaut se résout une fois, du plus fort au plus faible : Landlock,
puis bubblewrap là où Landlock est absent et où les espaces de noms sont
disponibles, puis un mode basique. Basique ne veut pas dire « aucune isolation » :
la séparation d'uid, seccomp et le moniteur de ressources s'appliquent toujours ;
c'est le confinement du système de fichiers par chemin qui manque, et le panneau
d'administration le dit.

**Aucune extension n'a d'accès réseau direct, quoi que dise son manifeste.**
Celle qui a besoin du réseau le demande à l'hôte par sa socket locale, et c'est
l'hôte qui effectue la requête : liste blanche appliquée hôte par hôte, GET et
HEAD seulement, adresses privées et lien-local refusées après résolution DNS,
donc les redirections sont revérifiées. La conception précédente filtrait par
nom d'hôte et par port, ce qu'une adresse IP littérale traversait sans effort,
métadonnées cloud comprises.

## Conséquences

- Fonctionne sans aucune préparation de l'administrateur dans un conteneur non
  privilégié, et se dégrade visiblement plutôt qu'en silence sur les noyaux trop
  anciens pour Landlock.
- Fermé par défaut là où cela compte : un mode `landlock` explicite refuse de
  démarrer une extension qu'il ne peut pas confiner.
- Le relais de sortie réseau devient le point de passage critique pour la
  sécurité, raison pour laquelle un administrateur devrait tout de même bloquer
  le lien-local et les plages internes au niveau du réseau. Deux couches
  indépendantes échouent différemment, une seule non.
- C'est une frontière de confiance vérifiée pour du code communautaire sur votre
  propre machine. Ce n'est pas une prison contre un attaquant disposant d'une
  faille noyau, et ce n'est pas vendu comme telle.
